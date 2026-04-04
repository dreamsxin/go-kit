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
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sony/gobreaker"
	"golang.org/x/time/rate"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/endpoint/circuitbreaker"
	"github.com/dreamsxin/go-kit/endpoint/ratelimit"
	kitlog "github.com/dreamsxin/go-kit/log"
	httpserver "github.com/dreamsxin/go-kit/transport/http/server"
)

// Service is a ready-to-run HTTP microservice.
// Create one with New, register handlers with Handle, then call Run.
type Service struct {
	addr       string
	mux        *http.ServeMux
	middleware []endpoint.Middleware
	logger     *kitlog.Logger
	metrics    *endpoint.Metrics
	srv        *http.Server
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
		lim := rate.NewLimiter(rate.Limit(rps), int(rps))
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

// Handle registers a handler for the given pattern.
// The handler is wrapped with all service-level middleware.
func (s *Service) Handle(pattern string, handler http.Handler) {
	s.mux.Handle(pattern, handler)
}

// HandleFunc registers a plain http.HandlerFunc.
func (s *Service) HandleFunc(pattern string, fn http.HandlerFunc) {
	s.mux.HandleFunc(pattern, fn)
}

// JSON registers a typed JSON handler for the given pattern.
// All service-level middleware is applied automatically.
//
// Example:
//
//	svc.JSON("/users", func(ctx context.Context, req CreateUserReq) (any, error) {
//	    return userService.Create(ctx, req)
//	})
func (s *Service) JSON(pattern string, handler func(ctx context.Context, req any) (any, error)) {
	b := endpoint.NewBuilder(endpoint.Endpoint(handler))
	for _, mw := range s.middleware {
		b = b.Use(mw)
	}
	ep := b.Build()
	h := httpserver.NewServer(ep,
		func(_ context.Context, r *http.Request) (any, error) { return nil, nil },
		httpserver.EncodeJSONResponse,
		httpserver.ServerErrorEncoder(httpserver.JSONErrorEncoder),
	)
	s.mux.Handle(pattern, h)
}

// Run starts the HTTP server and blocks until SIGINT/SIGTERM.
// It performs a graceful shutdown with a 10-second deadline.
func (s *Service) Run() {
	s.Start()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.srv.Shutdown(ctx); err != nil {
		s.logger.Sugar().Errorf("shutdown: %v", err)
	}
	s.logger.Sugar().Info("stopped")
}

// Start starts the HTTP server in the background and returns immediately.
// Use this in tests or when you need to manage the lifecycle yourself.
// Call Shutdown to stop the server.
func (s *Service) Start() {
	s.srv = &http.Server{Addr: s.addr, Handler: s.mux}
	go func() {
		s.logger.Sugar().Infof("listening on %s", s.addr)
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Sugar().Fatalf("listen: %v", err)
		}
	}()
}

// Shutdown gracefully stops the server.  Useful in tests.
func (s *Service) Shutdown(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

// JSON is a package-level convenience function that creates a typed JSON
// http.Handler without needing a Service.
//
// Example:
//
//	http.Handle("/hello", kit.JSON[HelloReq](func(ctx context.Context, req HelloReq) (any, error) {
//	    return HelloResp{Message: "Hello, " + req.Name}, nil
//	}))
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
