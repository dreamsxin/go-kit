package kit

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// HTTP is a lifecycle component that serves HTTP routes. Attach it to a
// Host to run it alongside other components, or start it directly for
// single-component services.
//
//	component, err := kit.NewHTTP(":8080", kit.WithRequestID())
//	if err != nil {
//	    return err
//	}
//	kit.HandleJSONTyped(component, "POST /hello", handler)
//	host, err := kit.NewHost(kit.WithLifecycle(component))
type HTTP struct {
	addr               string
	mux                *http.ServeMux
	httpHandler        http.Handler
	httpMiddleware     []func(http.Handler) http.Handler
	middleware         []endpoint.Middleware
	metrics            *endpoint.Metrics
	recorders          []endpoint.Recorder
	httpConfig         HTTPServerConfig
	requestID          bool
	requestIDValidator RequestIDValidator
	jsonMaxBodyBytes   int64
	jsonServerOptions  []httpserver.ServerOption
	healthTimeout      time.Duration

	checksMu        sync.Mutex
	livenessChecks  []namedHealthCheck
	readinessChecks []namedHealthCheck

	lifecycleMu   sync.Mutex
	srv           *http.Server
	serveErrors   chan error
	lifecycleDone chan struct{}
	started       bool
	stopped       bool
}

// Option configures an HTTP component.
type Option func(*HTTP) error

// NewHTTP creates an HTTP component listening on addr (for example ":8080").
func NewHTTP(addr string, opts ...Option) (*HTTP, error) {
	if strings.TrimSpace(addr) == "" {
		return nil, fmt.Errorf("kit: HTTP address cannot be empty")
	}
	h := &HTTP{
		addr:             addr,
		mux:              http.NewServeMux(),
		httpConfig:       DefaultHTTPServerConfig(),
		jsonMaxBodyBytes: DefaultJSONMaxBodyBytes,
		healthTimeout:    DefaultHealthCheckTimeout,
		serveErrors:      make(chan error, 1),
	}
	for i, option := range opts {
		if option == nil {
			return nil, fmt.Errorf("kit: option %d is nil", i)
		}
		if err := option(h); err != nil {
			return nil, fmt.Errorf("kit: apply option %d: %w", i, err)
		}
	}
	h.registerHealthEndpoints()
	h.httpHandler = h.applyHTTPMiddleware(h.mux)
	return h, nil
}

// MustNewHTTP creates an HTTP component and panics if its configuration is
// invalid. It is intended for tests and small examples; production startup
// should use NewHTTP.
func MustNewHTTP(addr string, opts ...Option) *HTTP {
	h, err := NewHTTP(addr, opts...)
	if err != nil {
		panic(err)
	}
	return h
}

// Name identifies the component in Host diagnostics.
func (h *HTTP) Name() string { return "http" }

// registerLifecycleReadiness bridges a lifecycle component's readiness probe
// into the /readyz and /health checks. Host calls it during assembly.
func (h *HTTP) registerLifecycleReadiness(name string, check HealthCheck) {
	h.checksMu.Lock()
	defer h.checksMu.Unlock()
	h.readinessChecks = append(h.readinessChecks, newNamedHealthCheck(name, check))
}

// Start binds the listener and serves HTTP in the background. Listener
// failures are returned directly.
func (h *HTTP) Start() error {
	h.lifecycleMu.Lock()
	defer h.lifecycleMu.Unlock()
	if h.started {
		return fmt.Errorf("kit: HTTP component already started")
	}
	if h.stopped {
		return fmt.Errorf("kit: HTTP component cannot be restarted after shutdown")
	}

	httpLis, err := net.Listen("tcp", h.addr)
	if err != nil {
		return fmt.Errorf("http listen: %w", err)
	}

	h.srv = &http.Server{
		Addr:              h.addr,
		Handler:           h.httpHandler,
		ReadHeaderTimeout: h.httpConfig.ReadHeaderTimeout,
		ReadTimeout:       h.httpConfig.ReadTimeout,
		WriteTimeout:      h.httpConfig.WriteTimeout,
		IdleTimeout:       h.httpConfig.IdleTimeout,
		MaxHeaderBytes:    h.httpConfig.MaxHeaderBytes,
	}
	h.lifecycleDone = make(chan struct{})
	h.started = true
	go func() {
		if err := h.srv.Serve(httpLis); err != nil && err != http.ErrServerClosed {
			h.reportServeError(fmt.Errorf("http serve: %w", err))
		}
	}()
	return nil
}

// Errors reports asynchronous serve failures after Start.
func (h *HTTP) Errors() <-chan error {
	return h.serveErrors
}

func (h *HTTP) reportServeError(err error) {
	select {
	case h.serveErrors <- err:
	default:
	}
}

// Shutdown gracefully stops the HTTP server.
func (h *HTTP) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("kit: nil shutdown context")
	}
	h.lifecycleMu.Lock()
	if !h.started {
		h.lifecycleMu.Unlock()
		return nil
	}
	srv := h.srv
	h.started = false
	h.stopped = true
	h.lifecycleMu.Unlock()

	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}

// ServeHTTP implements http.Handler, allowing the component to be used
// directly with httptest.NewServer or another HTTP server.
func (h *HTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.httpHandler.ServeHTTP(w, r)
}

// Handle registers a raw http.Handler for the given pattern.
//
// This is an escape hatch for HTTP integrations that do not model naturally
// as framework endpoints, such as static files, third-party handlers, or
// custom protocol endpoints.
//
// Endpoint middleware is intentionally not applied to plain HTTP handlers.
// Use HandleJSONTyped, HandleJSON, or HandleJSONEndpoint for application
// endpoints that should use the service -> endpoint -> transport chain and
// endpoint middleware such as timeout, logging, metrics, rate limiting, or
// circuit breaking.
func (h *HTTP) Handle(pattern string, handler http.Handler) {
	h.mux.Handle(pattern, h.withHTTPContext(handler))
}

// HandleFunc registers a raw http.HandlerFunc.
func (h *HTTP) HandleFunc(pattern string, fn http.HandlerFunc) {
	h.Handle(pattern, fn)
}

// applyEndpointMiddleware wraps base with the component-level endpoint
// middleware. Recording is applied outermost and labeled with operation, so
// each route reports its own numbers and the measurement covers the whole
// chain, including rejections from rate limiting or a circuit breaker.
func (h *HTTP) applyEndpointMiddleware(operation string, base endpoint.Endpoint) endpoint.Endpoint {
	if len(h.middleware) == 0 && len(h.recorders) == 0 {
		return base
	}
	b := endpoint.NewBuilder(base)
	if len(h.recorders) > 0 {
		b = b.WithRecording(operation, h.recorders...)
	}
	for _, mw := range h.middleware {
		b = b.Use(mw)
	}
	return b.Build()
}

func (h *HTTP) withHTTPContext(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := h.prepareHTTPContext(r.Context(), r, w)
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *HTTP) prepareHTTPContext(ctx context.Context, r *http.Request, w http.ResponseWriter) context.Context {
	ctx = withHTTPContext(ctx, r, w)
	if !h.requestID {
		return ctx
	}
	requestID := requestIDFromContextOrHeader(ctx, h.requestIDValidator)
	w.Header().Set(requestIDHeader, requestID)
	return endpoint.WithRequestID(ctx, requestID)
}

func (h *HTTP) applyHTTPMiddleware(handler http.Handler) http.Handler {
	for i := len(h.httpMiddleware) - 1; i >= 0; i-- {
		handler = h.httpMiddleware[i](handler)
	}
	return handler
}

type httpRequestKey struct{}
type httpResponseWriterKey struct{}

func requestFromContext(ctx context.Context) *http.Request {
	r, _ := ctx.Value(httpRequestKey{}).(*http.Request)
	return r
}

func responseWriterFromContext(ctx context.Context) http.ResponseWriter {
	w, _ := ctx.Value(httpResponseWriterKey{}).(http.ResponseWriter)
	return w
}

func withHTTPContext(ctx context.Context, r *http.Request, w http.ResponseWriter) context.Context {
	ctx = context.WithValue(ctx, httpRequestKey{}, r)
	return context.WithValue(ctx, httpResponseWriterKey{}, w)
}

var _ Lifecycle = (*HTTP)(nil)
var _ NamedLifecycle = (*HTTP)(nil)
var _ http.Handler = (*HTTP)(nil)
