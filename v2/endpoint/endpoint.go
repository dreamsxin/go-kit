// Package endpoint defines the core Endpoint type and related helpers.
//
// An Endpoint is the fundamental building block of the framework: a function
// that accepts a context and an arbitrary request value, and returns an
// arbitrary response value or an error.  Business logic, middleware, and
// transport layers all communicate through this single interface.
package endpoint

import "context"

// Endpoint is a function that handles a single RPC-style request.
// It is the primary abstraction in the framework — every service method,
// middleware, and transport adapter is expressed in terms of Endpoint.
type Endpoint func(ctx context.Context, request any) (response any, err error)

// Nop is a no-op Endpoint that always succeeds and returns an empty struct.
// Useful as a placeholder in tests or when an endpoint is not yet implemented.
func Nop(context.Context, any) (any, error) { return struct{}{}, nil }

// Failer may be implemented by a response type that carries its own
// business-logic error instead of returning it. When the response implements
// Failer and Failed() returns non-nil, the HTTP and gRPC servers discard the
// response and encode that error through their error encoder — the same status,
// stable code, and error handler a returned error would get.
//
// # When to use Failer
//
// Almost never. Return errors normally:
//
//	func (s *svc) CreateUser(ctx context.Context, req CreateUserRequest) (CreateUserResponse, error) {
//	    if req.Name == "" {
//	        return CreateUserResponse{}, apperror.InvalidArgument("user.name", "name required")
//	    }
//	    ...
//	}
//
// Failer exists for a service whose method signature cannot return an error —
// a generated batch handler that must fill one response slot per item, an
// adapter over a callback API — and which therefore has to smuggle the failure
// out inside the response value:
//
//	type CreateUserResponse struct {
//	    User *User
//	    err  error // unexported, set by business logic
//	}
//
//	func (r CreateUserResponse) Failed() error { return r.err }
//
// It is not a way to return a business error inside a 200 response: the servers
// turn it into an error response. To answer 200 with an error field in the
// body, put the field in the response struct and do not implement Failer.
//
// # Failer is invisible to middleware
//
// A Failer response reaches the transport with a nil error, so every middleware
// in this package — metrics, circuit breaker, retry, logging — records it as a
// success. Only the server that encodes the response sees the failure. Install
// FailerMiddleware innermost to turn it into a returned error first, and the
// rest of the chain observes what the client will.
type Failer interface {
	Failed() error
}

// ResponseError returns the error a response carries through Failer, or nil when
// the response carries none. Use it in a custom middleware that must judge the
// outcome of a call the way the transport will.
func ResponseError(response any) error {
	failer, ok := response.(Failer)
	if !ok {
		return nil
	}
	return failer.Failed()
}

// FailerMiddleware converts a Failer response into a returned error, so the
// middleware around it sees the failure instead of a nil error. What reaches the
// client is unchanged at the encoder: the servers discard a failed response and
// encode its error exactly as they encode a returned one.
//
// What does change is every error-sensitive middleware outside it, which is the
// point. A breaker now counts the failure and can open; retry now retries and
// re-executes the endpoint; metrics stop reporting the call as a success. Choose
// the placement accordingly — a business 404 is a failure worth measuring but
// usually not worth retrying, so put retry inside it and metrics outside.
//
// Install it innermost, closest to the endpoint:
//
//	ep = endpoint.NewBuilder(ep).
//	    WithMetrics(recorder).
//	    WithFailer(). // added last, so it ends up innermost
//	    Build()
func FailerMiddleware() Middleware {
	return func(next Endpoint) Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			response, err := next(ctx, request)
			if err != nil {
				return response, err
			}
			if failed := ResponseError(response); failed != nil {
				return nil, failed
			}
			return response, nil
		}
	}
}
