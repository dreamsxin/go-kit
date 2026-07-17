// Package main demonstrates the kit high-level API — the fastest path from
// zero to a running HTTP microservice.
//
// Unlike the quickstart example (which constructs the HTTP server manually),
// this example uses kit.New + kit.JSON + svc.Run to handle server lifecycle,
// middleware, and graceful shutdown automatically.
//
// Concepts shown:
//   - kit.New creates a Service with built-in /health endpoint
//   - kit.JSON wraps a typed handler with automatic JSON decode/encode
//   - svc.Handle registers the handler with service-level middleware
//   - svc.Run starts the server and blocks until SIGINT/SIGTERM
//
// Run:
//
//	go run ./examples/kit_basic
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
	"fmt"
	"time"

	"github.com/dreamsxin/go-kit/v2/kit"
)

// ── Domain types (no framework dependency) ────────────────────────────────────

type GreetRequest struct {
	Name string `json:"name"`
}

type GreetResponse struct {
	Message string `json:"message"`
}

// ── Pure business logic ───────────────────────────────────────────────────────

func greet(_ context.Context, req GreetRequest) (any, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	return GreetResponse{Message: "Hello, " + req.Name + "!"}, nil
}

// ── Wire-up ───────────────────────────────────────────────────────────────────

func main() {
	httpAddr := flag.String("http.addr", ":8080", "HTTP listen address")
	flag.Parse()

	// kit.New creates a Service with sensible defaults. Options add
	// cross-cutting concerns (rate limiting, request IDs, timeouts) as
	// service-level middleware applied to every registered handler.
	svc := kit.New(*httpAddr,
		kit.WithRequestID(),
		kit.WithTimeout(5*time.Second),
	)

	// kit.JSON produces an http.Handler that decodes the JSON request body
	// into GreetRequest, calls the business logic, and encodes the response
	// as JSON — no manual decoding or content-type handling needed.
	svc.Handle("/greet", kit.JSON(greet))

	// svc.Run starts the HTTP server and blocks until SIGINT or SIGTERM,
	// then performs a graceful shutdown.
	svc.Run()
}
