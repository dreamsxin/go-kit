// Command manual_composition demonstrates explicit endpoint and HTTP transport
// assembly for applications that need lower-level control.
//
// It demonstrates the component path after users understand the kit quickstart:
//  1. Define plain request/response types.
//  2. Write pure business logic (no framework imports).
//  3. Wire everything with NewJSONEndpoint over an explicitly built endpoint.
//
// Run:
//
//	go run ./examples/manual_composition
//
// Test:
//
//	curl -X POST http://localhost:8080/hello \
//	     -H "Content-Type: application/json" \
//	     -d '{"name":"world"}'
//
//	curl http://localhost:8080/health
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/endpoint/resilience"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// ── 1. Domain types (no framework dependency) ────────────────────────────────

type HelloRequest struct {
	Name string `json:"name"`
}

type HelloResponse struct {
	Message string `json:"message"`
}

// ── 2. Transport-neutral business logic ──────────────────────────────────────

func hello(_ context.Context, req HelloRequest) (HelloResponse, error) {
	if req.Name == "" {
		return HelloResponse{}, apperror.New(
			apperror.KindInvalidArgument,
			"hello.name_required",
			"name is required",
		)
	}
	return HelloResponse{Message: "Hello, " + req.Name + "!"}, nil
}

// ── 3. Wire-up ────────────────────────────────────────────────────────────────

func main() {
	httpAddr := flag.String("http.addr", ":8080", "HTTP listen address")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	var metrics endpoint.Metrics

	// NewJSONEndpoint wires business logic + middleware + HTTP: the endpoint
	// carries its chain, the JSON server only owns the wire.
	breaker := resilience.NewCircuitBreaker(resilience.WithBreakerFailureThreshold(3))
	limiter := newFixedRateLimiter(100)
	ep := endpoint.NewBuilder(endpoint.TypedEndpoint[HelloRequest, HelloResponse](hello).Wrap()).
		WithMetrics(&metrics).
		WithErrorHandling("hello").
		WithTimeout(5 * time.Second).
		Use(breaker.Middleware()).
		Use(resilience.RateLimitMiddleware(limiter)).
		Build()
	handler := httpserver.NewJSONEndpoint[HelloRequest](
		ep,
		httpserver.DefaultJSONDecodeOptions(),
		httpserver.ServerErrorEncoder(httpserver.JSONErrorEncoder),
	)

	mux := http.NewServeMux()
	mux.Handle("/hello", handler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"status":"ok","requests":%d}`, metrics.Snapshot().RequestCount)
	})

	srv := &http.Server{Addr: *httpAddr, Handler: mux}
	serveErr := make(chan error, 1)
	go func() {
		logger.Info("listening", "address", *httpAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-signalCtx.Done():
	case err := <-serveErr:
		logger.Error("listen failed", "err", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("shutdown failed", "err", err)
	}
	logger.Info("stopped", "total_requests", metrics.Snapshot().RequestCount)
}

// fixedRateLimiter is a per-second token bucket for the example.
type fixedRateLimiter struct {
	tokensPerSecond int64
	current         int64
	lastRefill      int64
}

func newFixedRateLimiter(perSecond int64) *fixedRateLimiter {
	return &fixedRateLimiter{
		tokensPerSecond: perSecond,
		current:         perSecond,
		lastRefill:      time.Now().UnixNano(),
	}
}

func (l *fixedRateLimiter) Allow() bool {
	if time.Now().UnixNano()-l.lastRefill >= int64(time.Second) {
		l.current = l.tokensPerSecond
		l.lastRefill = time.Now().UnixNano()
	}
	if l.current <= 0 {
		return false
	}
	l.current--
	return true
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
