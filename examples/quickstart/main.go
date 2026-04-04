// Package quickstart is the minimal go-kit HTTP service.
//
// It demonstrates the recommended "happy path" for new users:
//   1. Define plain request/response types.
//   2. Write pure business logic (no framework imports).
//   3. Wire everything with NewJSONServerWithMiddleware.
//
// Run:
//
//	go run ./examples/quickstart
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
	"flag"
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

// ── 1. Domain types (no framework dependency) ────────────────────────────────

type HelloRequest struct {
	Name string `json:"name"`
}

type HelloResponse struct {
	Message string `json:"message"`
}

// ── 2. Pure business logic ────────────────────────────────────────────────────

func hello(_ context.Context, req HelloRequest) (any, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	return HelloResponse{Message: "Hello, " + req.Name + "!"}, nil
}

// ── 3. Wire-up ────────────────────────────────────────────────────────────────

func main() {
	httpAddr := flag.String("http.addr", ":8080", "HTTP listen address")
	flag.Parse()

	logger, _ := kitlog.NewDevelopment()
	defer logger.Sync() //nolint:errcheck

	var metrics endpoint.Metrics

	// NewJSONServerWithMiddleware wires business logic + middleware + HTTP in one call.
	handler := httpserver.NewJSONServerWithMiddleware[HelloRequest](
		hello,
		func(b *endpoint.Builder) *endpoint.Builder {
			return b.
				WithMetrics(&metrics).
				WithErrorHandling("hello").
				WithTimeout(5*time.Second).
				Use(circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(
					gobreaker.Settings{Name: "hello"},
				))).
				Use(ratelimit.NewErroringLimiter(
					rate.NewLimiter(rate.Every(time.Second), 100),
				))
		},
		httpserver.ServerErrorEncoder(httpserver.JSONErrorEncoder),
	)

	mux := http.NewServeMux()
	mux.Handle("/hello", handler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"status":"ok","requests":%d}`, metrics.RequestCount)
	})

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
	srv.Shutdown(ctx) //nolint:errcheck
	logger.Sugar().Infof("stopped. total requests: %d", metrics.RequestCount)
}
