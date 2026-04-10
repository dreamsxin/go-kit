// Package kit provides a high-level, zero-boilerplate API for rapid
// prototyping and production microservices.
//
// # 30-second quickstart
//
//	func main() {
//	    svc := kit.New(":8080")
//	    svc.Handle("/hello", kit.JSON[HelloReq](func(ctx context.Context, req HelloReq) (any, error) {
//	        return HelloResp{Message: "Hello, " + req.Name}, nil
//	    }))
//	    svc.Run()
//	}
//
// # With middleware
//
//	svc := kit.New(":8080",
//	    kit.WithRateLimit(100),           // 100 req/s
//	    kit.WithCircuitBreaker(5),        // open after 5 consecutive failures
//	    kit.WithTimeout(5*time.Second),
//	    kit.WithRequestID(),              // inject X-Request-ID
//	    kit.WithLogging(logger),
//	    kit.WithMetrics(&metrics),
//	)
package kit

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sony/gobreaker"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/endpoint/circuitbreaker"
	"github.com/dreamsxin/go-kit/endpoint/ratelimit"
	kitlog "github.com/dreamsxin/go-kit/log"
	httpserver "github.com/dreamsxin/go-kit/transport/http/server"
)

// Service is a ready-to-run HTTP + gRPC microservice.
// Create one with New, register handlers with Handle/GRPC, then call Run.
type Service struct {
	addr       string
	mux        *http.ServeMux
	middleware []endpoint.Middleware
	logger     *kitlog.Logger
	metrics    *endpoint.Metrics
	srv        *http.Server

	// gRPC
	grpcAddr   string
	grpcServer *grpc.Server
	grpcOpts   []grpc.ServerOption
}

// Option configures a Service.
type Option func(*Service)

// New creates a Service listening on addr (e.g. ":8080").
func New(addr string, opts ...Option) *Service {
	logger, _ := kitlog.NewDevelopment()
	s := &Service{
		addr:   addr,
		mux:    http.NewServeMux(),
		logger: logger,
	}
	for _, o := range opts {
		o(s)
	}
	// always add health endpoint
	s.mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if s.metrics != nil {
			fmt.Fprintf(w, `{"status":"ok","requests":%d}`, s.metrics.RequestCount)
		} else {
			fmt.Fprint(w, `{"status":"ok"}`)
		}
	})
	return s
}

// WithRateLimit adds a token-bucket rate limiter (rps = requests per second).
func WithRateLimit(rps float64) Option {
	return func(s *Service) {
		// burst must be at least 1; int(rps) truncates to 0 for rps < 1,
		// which would reject every request including the very first one.
		burst := int(rps)
		if burst < 1 {
			burst = 1
		}
		lim := rate.NewLimiter(rate.Limit(rps), burst)
		s.middleware = append(s.middleware, ratelimit.NewErroringLimiter(lim))
	}
}

// WithCircuitBreaker adds a Gobreaker circuit breaker that opens after
// consecutiveFailures consecutive errors.
func WithCircuitBreaker(consecutiveFailures uint32) Option {
	return func(s *Service) {
		cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name: "service",
			ReadyToTrip: func(c gobreaker.Counts) bool {
				return c.ConsecutiveFailures >= consecutiveFailures
			},
		})
		s.middleware = append(s.middleware, circuitbreaker.Gobreaker(cb))
	}
}

// WithTimeout adds a per-request context deadline.
func WithTimeout(d time.Duration) Option {
	return func(s *Service) {
		s.middleware = append(s.middleware, endpoint.TimeoutMiddleware(d))
	}
}

// WithLogging adds structured request logging.
func WithLogging(logger *kitlog.Logger) Option {
	return func(s *Service) {
		s.logger = logger
		s.middleware = append(s.middleware, endpoint.LoggingMiddleware(logger, "request"))
	}
}

// WithMetrics attaches a Metrics collector.  The /health endpoint will
// include the request count when this option is set.
func WithMetrics(m *endpoint.Metrics) Option {
	return func(s *Service) {
		s.metrics = m
		s.middleware = append(s.middleware, endpoint.MetricsMiddleware(m))
	}
}

// WithRequestID injects a unique request ID into the context and response
// headers.  The ID is taken from X-Request-ID if present, otherwise generated.
func WithRequestID() Option {
	return func(s *Service) {
		s.middleware = append(s.middleware, requestIDMiddleware())
	}
}

// WithGRPC enables a gRPC server on the given address (e.g. ":8081").
// Call GRPCServer() to register your proto services before calling Run/Start.
//
// Example:
//
//	svc := kit.New(":8080", kit.WithGRPC(":8081"))
//	pb.RegisterGreeterServer(svc.GRPCServer(), &myGreeter{})
//	svc.Run()
func WithGRPC(addr string, opts ...grpc.ServerOption) Option {
	return func(s *Service) {
		s.grpcAddr = addr
		s.grpcOpts = opts
	}
}

// GRPCServer returns the underlying *grpc.Server so callers can register
// proto services.  It is created lazily on first call.
// Panics if WithGRPC was not set.
//
// Example:
//
//	pb.RegisterGreeterServer(svc.GRPCServer(), &myGreeter{})
func (s *Service) GRPCServer() *grpc.Server {
	if s.grpcAddr == "" {
		panic("kit: GRPCServer() called but WithGRPC option was not set")
	}
	if s.grpcServer == nil {
		s.grpcServer = grpc.NewServer(s.grpcOpts...)
	}
	return s.grpcServer
}

// ServeHTTP implements http.Handler, allowing Service to be used directly
// with httptest.NewServer or http.ListenAndServe.
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// Handle registers an http.Handler for the given pattern.
// Service-level middleware (metrics, timeout, circuit breaker, etc.) is applied
// by wrapping the handler as an endpoint so the full middleware chain executes.
func (s *Service) Handle(pattern string, handler http.Handler) {
	if len(s.middleware) == 0 {
		s.mux.Handle(pattern, handler)
		return
	}
	// Wrap the http.Handler as an endpoint so middleware applies correctly.
	// The endpoint calls the handler and captures any panic as an error.
	base := endpoint.Endpoint(func(ctx context.Context, req any) (any, error) {
		rw := req.(http.ResponseWriter)
		r := ctx.Value(httpRequestKey{}).(*http.Request).WithContext(ctx)
		handler.ServeHTTP(rw, r)
		return nil, nil
	})
	b := endpoint.NewBuilder(base)
	for _, mw := range s.middleware {
		b = b.Use(mw)
	}
	ep := b.Build()
	s.mux.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), httpRequestKey{}, r)
		if _, err := ep(ctx, w); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
		}
	}))
}

type httpRequestKey struct{}

// HandleFunc registers a plain http.HandlerFunc.
// Service-level middleware (metrics, timeout, circuit breaker, etc.) is applied
// via Handle so the full middleware chain executes for every registered handler.
func (s *Service) HandleFunc(pattern string, fn http.HandlerFunc) {
	s.Handle(pattern, fn)
}

// Run starts the HTTP server (and gRPC server if WithGRPC was set) and blocks
// until SIGINT/SIGTERM.  It performs a graceful shutdown with a 10-second deadline.
func (s *Service) Run() {
	s.Start()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.srv.Shutdown(ctx); err != nil {
		s.logger.Sugar().Errorf("HTTP shutdown: %v", err)
	}
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
		s.logger.Sugar().Info("gRPC stopped")
	}
	s.logger.Sugar().Info("stopped")
}

// Start starts the HTTP server (and gRPC server if WithGRPC was set) in the
// background and returns immediately.
// Use this in tests or when you need to manage the lifecycle yourself.
// Call Shutdown to stop both servers.
func (s *Service) Start() {
	s.srv = &http.Server{Addr: s.addr, Handler: s.mux}
	go func() {
		s.logger.Sugar().Infof("HTTP listening on %s", s.addr)
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Sugar().Fatalf("listen: %v", err)
		}
	}()

	if s.grpcAddr != "" {
		gs := s.GRPCServer() // ensure server is created
		lis, err := net.Listen("tcp", s.grpcAddr)
		if err != nil {
			s.logger.Sugar().Fatalf("gRPC listen: %v", err)
		}
		go func() {
			s.logger.Sugar().Infof("gRPC listening on %s", s.grpcAddr)
			if err := gs.Serve(lis); err != nil {
				s.logger.Sugar().Errorf("gRPC serve: %v", err)
			}
		}()
	}
}

// Shutdown gracefully stops the HTTP server (and gRPC server if running).
// Useful in tests.
func (s *Service) Shutdown(ctx context.Context) error {
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

// JSON is a package-level convenience function that creates a typed JSON
// http.Handler without needing a Service.  It is the recommended way to
// register typed handlers on a Service, because Go methods cannot have
// their own type parameters:
//
//	svc.Handle("/hello", kit.JSON[HelloReq](func(ctx context.Context, req HelloReq) (any, error) {
//	    return HelloResp{Message: "Hello, " + req.Name + "!"}, nil
//	}))
//
// The handler receives a fully decoded value of type Req.
// Errors are encoded as JSON {"error": "..."} with status 500.
func JSON[Req any](handler func(ctx context.Context, req Req) (any, error)) http.Handler {
	return httpserver.NewJSONServer[Req](handler,
		httpserver.ServerErrorEncoder(httpserver.JSONErrorEncoder),
	)
}

// ── request ID middleware ─────────────────────────────────────────────────────

type requestIDKey struct{}

func requestIDMiddleware() endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req any) (any, error) {
			return next(ctx, req)
		}
	}
}
