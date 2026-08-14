// Command quickstart demonstrates the recommended kit path from zero to a
// running HTTP service. kit.New, kit.HandleJSON, and Service.Run own HTTP
// assembly, middleware, lifecycle, and graceful shutdown.
//
// Concepts shown:
//   - kit.New validates configuration and creates a Service with /health
//   - kit.HandleJSON wraps a typed handler with endpoint middleware and JSON transport
//   - svc.Run follows the caller-owned context lifecycle
//
// Run:
//
//	go run ./examples/quickstart
//
// Test:
//
//	curl -X POST http://localhost:8080/greet \
//	     -H "Content-Type: application/json" \
//	     -d '{"name":"Alice"}'
//
//	curl http://localhost:8080/health
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/kit"
)

// ── Domain types (no framework dependency) ────────────────────────────────────

type GreetRequest struct {
	Name string `json:"name"`
}

type GreetResponse struct {
	Message string `json:"message"`
}

// ── Transport-neutral business logic ─────────────────────────────────────────

func greet(_ context.Context, req GreetRequest) (any, error) {
	if req.Name == "" {
		return nil, apperror.New(
			apperror.KindInvalidArgument,
			"greet.name_required",
			"name is required",
		)
	}
	return GreetResponse{Message: "Hello, " + req.Name + "!"}, nil
}

// ── Wire-up ───────────────────────────────────────────────────────────────────

func main() {
	httpAddr := flag.String("http.addr", ":8080", "HTTP listen address")
	flag.Parse()

	// kit.New creates a Service with sensible defaults. Options add
	// cross-cutting concerns (request IDs and timeouts) as
	// service-level middleware applied to every registered handler.
	svc, err := kit.New(*httpAddr,
		kit.WithRequestID(),
		kit.WithTimeout(5*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}

	// HandleJSON preserves the normal service -> endpoint -> transport path, so
	// configured endpoint middleware and strict JSON decoding both apply.
	kit.HandleJSON[GreetRequest](svc, "/greet", greet)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := svc.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
