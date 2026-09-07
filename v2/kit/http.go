package kit

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/health"
	transporthttp "github.com/dreamsxin/go-kit/v2/transport/http"
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
	httpRecorders      []httpserver.Recorder
	httpConfig         HTTPServerConfig
	requestID          bool
	requestIDValidator RequestIDValidator
	jsonMaxBodyBytes   int64
	jsonServerOptions  []httpserver.ServerOption
	healthTimeout      time.Duration
	timeout            time.Duration

	probePaths       health.Paths
	pendingLiveness  []pendingProbe
	pendingReadiness []pendingProbe
	probes           *health.Registry

	lifecycleMu   sync.Mutex
	srv           *http.Server
	serveErrors   chan error
	lifecycleDone chan struct{}
	started       bool
	stopped       bool
	listenerAddr  string

	// stopping closes when the process announces that it is going away, so a
	// long-lived handler can end its own response instead of being cut.
	stopping    chan struct{}
	stopOnce    sync.Once
	serveCtx    context.Context
	cancelServe context.CancelFunc
	inFlight    atomic.Int64
	tlsConfig   *tls.Config
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
		probePaths:       DefaultProbePaths(),
		serveErrors:      make(chan error, 1),
		stopping:         make(chan struct{}),
	}
	for i, option := range opts {
		if option == nil {
			return nil, fmt.Errorf("kit: option %d is nil", i)
		}
		if err := option(h); err != nil {
			return nil, fmt.Errorf("kit: apply option %d: %w", i, err)
		}
	}
	if err := h.buildProbes(); err != nil {
		return nil, fmt.Errorf("kit: %w", err)
	}
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

	// Every request context descends from serveCtx, which is what lets Shutdown
	// tell in-flight handlers to stop once the grace period is spent. Without
	// it, a handler that never watches for the process going away can only be
	// ended by closing its connection underneath it.
	h.serveCtx, h.cancelServe = context.WithCancel(context.Background())
	h.serveCtx = WithStopping(h.serveCtx, h.stopping)
	serveCtx := h.serveCtx

	h.srv = &http.Server{
		Addr:              h.addr,
		Handler:           h.httpHandler,
		BaseContext:       func(net.Listener) context.Context { return serveCtx },
		ReadHeaderTimeout: h.httpConfig.ReadHeaderTimeout,
		ReadTimeout:       h.httpConfig.ReadTimeout,
		WriteTimeout:      h.httpConfig.WriteTimeout,
		IdleTimeout:       h.httpConfig.IdleTimeout,
		MaxHeaderBytes:    h.httpConfig.MaxHeaderBytes,
	}
	h.listenerAddr = httpLis.Addr().String()
	h.lifecycleDone = make(chan struct{})
	h.started = true
	tlsConfig := h.tlsConfig
	go func() {
		var err error
		if tlsConfig != nil {
			// The certificates are already loaded and in TLSConfig, so the file
			// arguments are empty: ServeTLS only reads them when TLSConfig has no
			// certificate of its own.
			h.srv.TLSConfig = tlsConfig
			err = h.srv.ServeTLS(httpLis, "", "")
		} else {
			err = h.srv.Serve(httpLis)
		}
		if err != nil && err != http.ErrServerClosed {
			h.reportServeError(fmt.Errorf("http serve: %w", err))
		}
	}()
	return nil
}

// Addr reports the address the component is serving on: the resolved listener
// address once Start has bound it, which is the only way to learn the port when
// the configured address ends in ":0", and the configured address before that.
func (h *HTTP) Addr() string {
	h.lifecycleMu.Lock()
	defer h.lifecycleMu.Unlock()
	if h.listenerAddr != "" {
		return h.listenerAddr
	}
	return h.addr
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

// Shutdown gracefully stops the HTTP server, and then stops it.
//
// The graceful attempt is http.Server.Shutdown: idle connections close, and
// in-flight handlers are given until ctx expires. What happens after that is the
// part worth stating: a handler that never watches for cancellation — a stream,
// a long poll — would otherwise leave this call returning a deadline error with
// the connection still open and the goroutine still running. So the request
// contexts are cancelled, handlers get a moment to unwind, and whatever is left
// is closed. The error says how many requests that was.
//
// Stable: kit.shutdown-ends — when the graceful attempt runs out of budget, Shutdown cancels in-flight requests and closes the rest rather than returning while they are open, and reports how many it interrupted.
// Covered by: TestShutdownClosesWhatTheGracePeriodLeftOpen, TestShutdownStaysGracefulWhenHandlersFinish
//
// One kind of connection is outside all of that: a hijacked one. Once a handler
// takes the socket — a WebSocket, or any other upgrade — the server stops tracking
// it, so the graceful wait does not include it, closing the listener does not close
// it, and the hard close cannot reach it. Ending it is the upgraded handler's job,
// which is what Stopping is for.
//
// Stable: kit.hijacked-connections-are-not-drained — a hijacked connection is neither waited for nor closed by Shutdown; the upgraded handler ends it, using Stopping as the signal.
// Covered by: TestHijackedConnectionOutlivesShutdown, TestUpgradedHandlerIsToldTheProcessIsStopping
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
	cancelServe := h.cancelServe
	h.started = false
	h.stopped = true
	h.lifecycleMu.Unlock()

	h.announceStopping()
	if srv == nil {
		return nil
	}
	gracefulErr := srv.Shutdown(ctx)
	if gracefulErr == nil {
		return nil
	}
	return h.closeWhatIsLeft(srv, cancelServe, gracefulErr)
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
// What a raw route still gets from the component: the request context (so
// kit.RequestFromContext works), WithRequestID, and the WithTimeout deadline.
// What it does not get: WithMetrics, WithRecorder, WithRateLimit,
// WithCircuitBreaker, and anything added through WithEndpointMiddleware, since
// those observe or gate an endpoint call rather than an http.Handler. Use
// HandleJSONTyped, HandleJSON, or HandleJSONEndpoint for application endpoints
// that should run the service -> endpoint -> transport chain.
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
	routed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.inFlight.Add(1)
		defer h.inFlight.Add(-1)
		ctx := h.prepareHTTPContext(r.Context(), r, w)
		if h.timeout > 0 {
			// The deadline is applied here, not only in the endpoint chain, so
			// a raw handler registered with Handle is bounded too. Without it,
			// WithTimeout looked like a service-wide option while silently
			// leaving every raw route unbounded.
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, h.timeout)
			defer cancel()
		}
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
	if len(h.httpRecorders) == 0 {
		return routed
	}
	// Recording wraps the route rather than the mux: the matched pattern only
	// exists on the request a ServeMux dispatched, so this is the outermost
	// place that can report http.route. It also wraps the response writer,
	// which is why it sits outside the context preparation — the writer the
	// handler and the context see must be the one that counts the status.
	return httpserver.RecordingMiddleware(h.httpRecorders...)(routed)
}

func (h *HTTP) prepareHTTPContext(ctx context.Context, r *http.Request, w http.ResponseWriter) context.Context {
	ctx = withHTTPContext(ctx, r, w)
	// The stopping signal is added here as well as through the server's base
	// context, so a component mounted on someone else's server — httptest, an
	// outer mux — still tells its handlers when the process is going away.
	ctx = WithStopping(ctx, h.stopping)
	// Trace context is extracted unconditionally: a service that had to opt in
	// would break every trace that reaches it until somebody noticed. An
	// absent or malformed traceparent leaves the context untouched, and
	// endpoint.TracingMiddleware then mints a new trace.
	ctx = transporthttp.ExtractTraceparent(ctx, r)
	if !h.requestID {
		return ctx
	}
	ctx = transporthttp.RequestIDExtractor(h.requestIDValidator)(ctx, r)
	return transporthttp.EchoRequestID(ctx, w)
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
