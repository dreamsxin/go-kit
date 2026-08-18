package endpoint_test

import (
	"context"
	"fmt"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// ExampleChain composes middleware into a single Middleware. The first
// argument is outermost: requests pass through the given middleware first to
// last, and responses return last to first.
func ExampleChain() {
	annotate := func(label string) endpoint.Middleware {
		return func(next endpoint.Endpoint) endpoint.Endpoint {
			return func(ctx context.Context, req any) (any, error) {
				fmt.Println("before", label)
				resp, err := next(ctx, req)
				fmt.Println("after", label)
				return resp, err
			}
		}
	}

	base := func(context.Context, any) (any, error) {
		fmt.Println("business logic")
		return "ok", nil
	}

	composed := endpoint.Chain(annotate("first"), annotate("second"))
	resp, err := composed(base)(context.Background(), nil)
	fmt.Println(resp, err)

	// Output:
	// before first
	// before second
	// business logic
	// after second
	// after first
	// ok <nil>
}

// ExampleUnwrap restores the concrete request and response types of an
// Endpoint that was built from typed business logic, so callers keep compile
// time type safety on both sides of a middleware chain.
func ExampleUnwrap() {
	logic := endpoint.TypedEndpoint[string, string](
		func(_ context.Context, name string) (string, error) {
			return "Hello, " + name + "!", nil
		})

	// Wrap into an untyped Endpoint, for example to pass it through
	// middleware that only knows endpoint.Endpoint.
	var ep endpoint.Endpoint = logic.Wrap()

	greet := endpoint.Unwrap[string, string](ep)
	greeting, err := greet(context.Background(), "go-kit")
	fmt.Println(greeting, err)

	// Output:
	// Hello, go-kit! <nil>
}
