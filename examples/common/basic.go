// Package common provides shared helpers and illustrative patterns used
// across the examples/ directory.
//
// It demonstrates:
//   - Wrapping an existing service interface as an endpoint.Endpoint
//   - Implementing transport/http/interfaces.Headerer on a response type
package common

import (
	"context"
	"fmt"
	"net/http"

	"github.com/dreamsxin/go-kit/endpoint"
)

// ── Service interface ─────────────────────────────────────────────────────────

// Greeter is a minimal service interface used in examples.
type Greeter interface {
	Greet(ctx context.Context, name string) (string, error)
}

// ── In-memory implementation ──────────────────────────────────────────────────

type inmemGreeter struct{}

// NewGreeter returns a simple in-memory Greeter implementation.
func NewGreeter() Greeter { return &inmemGreeter{} }

func (g *inmemGreeter) Greet(_ context.Context, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("name must not be empty")
	}
	return "Hello, " + name + "!", nil
}

// ── Endpoint factory ──────────────────────────────────────────────────────────

// MakeGreetEndpoint wraps a Greeter as an endpoint.Endpoint.
// The request value must be a string (the name to greet).
func MakeGreetEndpoint(svc Greeter) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		name, ok := request.(string)
		if !ok {
			return nil, fmt.Errorf("expected string request, got %T", request)
		}
		return svc.Greet(ctx, name)
	}
}

// ── Response with custom headers ──────────────────────────────────────────────

// GreetResponse is a response type that adds a custom HTTP header.
// It implements transport/http/interfaces.Headerer so the HTTP server
// automatically merges the headers into the response.
type GreetResponse struct {
	Message string `json:"message"`
}

// Headers implements interfaces.Headerer.
func (r GreetResponse) Headers() http.Header {
	return http.Header{"X-Greeted-By": []string{"go-kit-example"}}
}
