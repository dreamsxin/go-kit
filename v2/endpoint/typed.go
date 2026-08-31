package endpoint

import (
	"context"
	"fmt"
	"reflect"
)

// TypedEndpoint is a type-safe variant of Endpoint that eliminates runtime
// type assertions.  Req and Resp are the concrete request and response types.
//
// Use Wrap to convert a TypedEndpoint into a plain Endpoint for use with
// middleware and transport layers.  Use Unwrap to go the other direction.
//
// Example:
//
//	type HelloReq  struct { Name string }
//	type HelloResp struct { Message string }
//
//	var ep endpoint.TypedEndpoint[HelloReq, HelloResp] =
//	    func(ctx context.Context, req HelloReq) (HelloResp, error) {
//	        return HelloResp{Message: "Hello, " + req.Name}, nil
//	    }
//
//	// Convert to plain Endpoint for middleware / transport
//	plain := ep.Wrap()
//
//	// Build with middleware, then call type-safely
//	typed := endpoint.Unwrap[HelloReq, HelloResp](
//	    endpoint.NewBuilder(plain).
//	        WithTimeout(5 * time.Second).
//	        Build(),
//	)
//	resp, err := typed(ctx, HelloReq{Name: "world"})
type TypedEndpoint[Req, Resp any] func(ctx context.Context, req Req) (Resp, error)

// Wrap converts a TypedEndpoint into a plain Endpoint.
// The returned Endpoint performs a type assertion on the request value;
// it returns a TypeAssertError if the request is not of type Req.
func (te TypedEndpoint[Req, Resp]) Wrap() Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		typed, ok := request.(Req)
		if !ok {
			return nil, NewTypeAssertError[Req](request)
		}
		return te(ctx, typed)
	}
}

// Unwrap wraps a plain Endpoint in a TypedEndpoint[Req, Resp].
// The returned function type-asserts the response; it returns an error if
// the response cannot be asserted to Resp.
//
// Use this after applying middleware to recover type safety:
//
//	typed := endpoint.Unwrap[HelloReq, HelloResp](
//	    endpoint.NewBuilder(base).Use(myMiddleware).Build(),
//	)
//	resp, err := typed(ctx, HelloReq{Name: "world"})
func Unwrap[Req, Resp any](ep Endpoint) TypedEndpoint[Req, Resp] {
	return func(ctx context.Context, req Req) (Resp, error) {
		resp, err := ep(ctx, req)
		if err != nil {
			var zero Resp
			return zero, err
		}
		typed, ok := resp.(Resp)
		if !ok {
			var zero Resp
			return zero, NewTypeAssertError[Resp](resp)
		}
		return typed, nil
	}
}

// NewTypedBuilder creates a Builder from a TypedEndpoint.
// This is a convenience shorthand for endpoint.NewBuilder(te.Wrap()).
//
// Example:
//
//	ep := endpoint.NewTypedBuilder(myTypedEndpoint).
//	    WithTimeout(5 * time.Second).
//	    Use(endpoint.NewCircuitBreaker().Middleware()).
//	    Build()
func NewTypedBuilder[Req, Resp any](te TypedEndpoint[Req, Resp]) *Builder {
	return NewBuilder(te.Wrap())
}

// TypeAssertError is returned when a wrapped endpoint request or unwrapped
// endpoint response cannot be asserted to the expected type.
type TypeAssertError struct {
	// Got is the value that failed the assertion.
	Got any
	// Want is the type the endpoint expected. It records a type rather than a
	// zero value so interface and pointer targets still report a usable name.
	Want reflect.Type
}

// NewTypeAssertError reports that got could not be asserted to Want.
func NewTypeAssertError[Want any](got any) *TypeAssertError {
	return &TypeAssertError{Got: got, Want: reflect.TypeFor[Want]()}
}

func (e *TypeAssertError) Error() string {
	return "endpoint: type assertion failed: " +
		typeName(e.Got) + " is not " + wantTypeName(e.Want)
}

func typeName(v any) string {
	if v == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%T", v)
}

func wantTypeName(t reflect.Type) string {
	if t == nil {
		return "<unknown>"
	}
	return t.String()
}
