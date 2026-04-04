// Package main demonstrates go-kit best practices:
//   - Pure business logic separated from transport
//   - Fluent endpoint.Builder for middleware assembly
//   - NewJSONServer for zero-boilerplate HTTP handling
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
	"syscall"
	"time"

	"github.com/sony/gobreaker"
	"go.uber.org/zap"
	"golang.org/x/time/rate"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/endpoint/circuitbreaker"
	"github.com/dreamsxin/go-kit/endpoint/ratelimit"
	kitlog "github.com/dreamsxin/go-kit/log"
	"github.com/dreamsxin/go-kit/transport/http/server"
)

// ── Domain types ──────────────────────────────────────────────────────────────

type helloRequest struct {
	Name string `json:"name"`
}

type helloResponse struct {
	Message string `json:"message"`
}

// ── Business logic (no framework dependency) ──────────────────────────────────

func helloLogic(_ context.Context, req helloRequest) (helloResponse, error) {
	if req.Name == "" {
		return helloResponse{}, errors.New("name is required")
	}
	return helloResponse{Message: fmt.Sprintf("Hello, %s!", req.Name)}, nil
}

// ── Wire-up ───────────────────────────────────────────────────────────────────

func main() {
	httpAddr := flag.String("http.addr", ":8080", "HTTP listen address")
	flag.Parse()

	logger, err := kitlog.NewDevelopment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger init: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() //nolint:errcheck

	// ── Middleware components ─────────────────────────────────────────────────
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "hello",
		MaxRequests: 5,
		Interval:    10 * time.Second,
		Timeout:     5 * time.Second,
		ReadyToTrip: func(c gobreaker.Counts) bool { return c.ConsecutiveFailures > 3 },
	})
	limiter := rate.NewLimiter(rate.Every(time.Second), 100)

	// ── Endpoint assembly via Builder ─────────────────────────────────────────
	var metrics endpoint.Metrics
	base := endpoint.Endpoint(func(ctx context.Context, req any) (any, error) {
		return helloLogic(ctx, req.(helloRequest))
	})
	ep := endpoint.NewBuilder(base).
		WithMetrics(&metrics).
		WithErrorHandling("hello").
		Use(endpoint.TimeoutMiddleware(5 * time.Second)).
		Use(circuitbreaker.Gobreaker(cb)).
		Use(ratelimit.NewErroringLimiter(limiter)).
		Build()

	// ── HTTP handlers ─────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	// /hello — automatic JSON decode/encode via NewJSONServer
	mux.Handle("/hello", server.NewJSONServer[helloRequest](
		func(ctx context.Context, req helloRequest) (any, error) {
			return ep(ctx, req)
		},
		server.ServerErrorEncoder(jsonErrorEncoder(logger)),
	))

	// /metrics — expose request counters
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w,
			`{"requests":%d,"success":%d,"errors":%d,"avg_ms":%.2f}`,
			metrics.RequestCount,
			metrics.SuccessCount,
			metrics.ErrorCount,
			avgMs(&metrics),
		)
	})

	// /health
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	srv := &http.Server{Addr: *httpAddr, Handler: mux}

	go func() {
		logger.Sugar().Infof("listening on %s", *httpAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Sugar().Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Sugar().Errorf("shutdown: %v", err)
	}
	logger.Sugar().Infof("stopped — total requests: %d", metrics.RequestCount)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func avgMs(m *endpoint.Metrics) float64 {
	if m.RequestCount == 0 {
		return 0
	}
	return float64(m.TotalDuration.Milliseconds()) / float64(m.RequestCount)
}

// jsonErrorEncoder maps known errors to appropriate HTTP status codes and
// writes a JSON error body.
func jsonErrorEncoder(logger *zap.Logger) func(context.Context, error, http.ResponseWriter) {
	return func(_ context.Context, err error, w http.ResponseWriter) {
		code := http.StatusInternalServerError
		switch {
		case errors.Is(err, ratelimit.ErrLimited):
			code = http.StatusTooManyRequests
		case errors.Is(err, errors.New("name is required")):
			code = http.StatusBadRequest
		}
		logger.Sugar().Warnw("request error", "status", code, "err", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		fmt.Fprintf(w, `{"error":%q}`, err.Error())
	}
}
