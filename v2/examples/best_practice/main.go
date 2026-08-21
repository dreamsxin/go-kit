// Package main demonstrates go-kit best practices:
//   - Pure business logic separated from transport
//   - Fluent endpoint.Builder for middleware assembly
//   - NewTypedJSONServer for type-safe HTTP handling
//   - MetricsMiddleware for built-in request counters
//   - Graceful shutdown
//
// Run:
//
//	go run ./examples/best_practice
//
// Test:
//
//	curl -X POST http://localhost:8080/hello \
//	     -H "Content-Type: application/json" \
//	     -d '{"name":"Alice"}'
//
//	curl http://localhost:8080/metrics
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// ── Domain types ──────────────────────────────────────────────────────────────

type helloRequest struct {
	Name string `json:"name"`
}

type helloResponse struct {
	Message string `json:"message"`
}

// errNameRequired is a sentinel error used by both helloLogic and the
// error encoder so that errors.Is can match reliably.
var errNameRequired = errors.New("name is required")

// ── Business logic (no framework dependency) ──────────────────────────────────

func helloLogic(_ context.Context, req helloRequest) (helloResponse, error) {
	if req.Name == "" {
		return helloResponse{}, errNameRequired
	}
	return helloResponse{Message: fmt.Sprintf("Hello, %s!", req.Name)}, nil
}

// ── Wire-up ───────────────────────────────────────────────────────────────────

func main() {
	httpAddr := flag.String("http.addr", ":8080", "HTTP listen address")
	flag.Parse()

	logger, err := zap.NewDevelopment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger init: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() //nolint:errcheck

	// ── Middleware components ─────────────────────────────────────────────────
	breaker := endpoint.NewCircuitBreaker(
		endpoint.WithBreakerFailureThreshold(3),
		endpoint.WithBreakerOpenTimeout(5*time.Second),
	)
	limiter := newFixedRateLimiter(100) // 100 requests per second, burst of 100

	// ── Endpoint assembly via Builder ─────────────────────────────────────────
	var metrics endpoint.Metrics
	ep := endpoint.NewTypedBuilder(endpoint.TypedEndpoint[helloRequest, helloResponse](helloLogic)).
		WithMetrics(&metrics).
		WithErrorHandling("hello").
		Use(endpoint.TimeoutMiddleware(5 * time.Second)).
		Use(breaker.Middleware()).
		Use(endpoint.RateLimitMiddleware(limiter)).
		Build()

	// ── HTTP handlers ─────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	// /hello — automatic JSON decode/encode via NewTypedJSONServer
	mux.Handle("/hello", server.NewTypedJSONServer(
		endpoint.Unwrap[helloRequest, helloResponse](ep),
		server.ServerErrorEncoder(jsonErrorEncoder(logger)),
	))

	// /metrics — expose request counters
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		snapshot := metrics.Snapshot()
		fmt.Fprintf(w,
			`{"requests":%d,"success":%d,"errors":%d,"avg_ms":%.2f}`,
			snapshot.RequestCount,
			snapshot.SuccessCount,
			snapshot.ErrorCount,
			float64(snapshot.AverageDuration())/float64(time.Millisecond),
		)
	})

	// /health
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	srv := &http.Server{Addr: *httpAddr, Handler: mux}

	serveErr := make(chan error, 1)
	go func() {
		logger.Sugar().Infof("listening on %s", *httpAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-signalCtx.Done():
	case err := <-serveErr:
		logger.Sugar().Errorf("listen: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Sugar().Errorf("shutdown: %v", err)
	}
	logger.Sugar().Infof("stopped — total requests: %d", metrics.Snapshot().RequestCount)
}

// fixedRateLimiter is a per-second token bucket for the example.
type fixedRateLimiter struct {
	tokensPerSecond int64
	burst           int64
	current         int64
	lastRefill      int64 // unix nanoseconds
}

func newFixedRateLimiter(perSecond int64) *fixedRateLimiter {
	return &fixedRateLimiter{
		tokensPerSecond: perSecond,
		burst:           perSecond,
		current:         perSecond,
		lastRefill:      time.Now().UnixNano(),
	}
}

func (l *fixedRateLimiter) Allow() bool {
	now := time.Now().UnixNano()
	elapsed := now - atomic.LoadInt64(&l.lastRefill)
	if elapsed >= int64(time.Second) {
		atomic.StoreInt64(&l.current, l.tokensPerSecond)
		atomic.StoreInt64(&l.lastRefill, now)
	}
	for {
		cur := atomic.LoadInt64(&l.current)
		if cur <= 0 {
			return false
		}
		if atomic.CompareAndSwapInt64(&l.current, cur, cur-1) {
			return true
		}
	}
}

func (l *fixedRateLimiter) Wait(ctx context.Context) error {
	for {
		if l.Allow() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// jsonErrorEncoder maps known errors to appropriate HTTP status codes and
// writes a JSON error body.
func jsonErrorEncoder(logger *zap.Logger) func(context.Context, error, http.ResponseWriter) {
	return func(_ context.Context, err error, w http.ResponseWriter) {
		code := http.StatusInternalServerError
		switch {
		case errors.Is(err, endpoint.ErrRateLimited):
			code = http.StatusTooManyRequests
		case errors.Is(err, errNameRequired):
			code = http.StatusBadRequest
		}
		logger.Sugar().Warnw("request error", "status", code, "err", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		fmt.Fprintf(w, `{"error":%q}`, err.Error())
	}
}
