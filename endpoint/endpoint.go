// Package endpoint defines the core Endpoint type and related helpers.
//
// An Endpoint is the fundamental building block of the framework: a function
// that accepts a context and an arbitrary request value, and returns an
// arbitrary response value or an error.  Business logic, middleware, and
// transport layers all communicate through this single interface.
package endpoint

import (
	"context"
	"time"
)

// Endpoint is a function that handles a single RPC-style request.
// It is the primary abstraction in the framework — every service method,
// middleware, and transport adapter is expressed in terms of Endpoint.
type Endpoint func(ctx context.Context, request any) (response any, err error)

// Nop is a no-op Endpoint that always succeeds and returns an empty struct.
// Useful as a placeholder in tests or when an endpoint is not yet implemented.
func Nop(context.Context, any) (any, error) { return struct{}{}, nil }

// EndpointerOptions configures the behaviour of an EndpointCache when a
// service-discovery error is received.
type EndpointerOptions struct {
	// InvalidateOnError, when true, causes the cache to be cleared after
	// InvalidateTimeout has elapsed following a discovery error.
	InvalidateOnError bool

	// InvalidateTimeout is the grace period before the cache is cleared.
	// Only meaningful when InvalidateOnError is true.
	InvalidateTimeout time.Duration
}

// EndpointerOption is a functional option for EndpointerOptions.
type EndpointerOption func(*EndpointerOptions)

// InvalidateOnError returns an EndpointerOption that enables cache
// invalidation after a service-discovery error.  The cache is cleared once
// timeout has elapsed, causing subsequent Endpoints() calls to return an
// error until healthy instances are re-discovered.
func InvalidateOnError(timeout time.Duration) EndpointerOption {
	return func(opts *EndpointerOptions) {
		opts.InvalidateOnError = true
		opts.InvalidateTimeout = timeout
	}
}

// Failer may be implemented by a response type to signal a business-logic
// error without using the Go error return value.  If the response implements
// Failer and Failed() returns non-nil, the transport layer treats the call
// as failed.
//
// # When to use Failer
//
// Most services should return errors normally:
//
//	func (s *svc) CreateUser(ctx context.Context, req CreateUserRequest) (CreateUserResponse, error) {
//	    if req.Name == "" {
//	        return CreateUserResponse{}, errors.New("name required") // normal Go error
//	    }
//	    ...
//	}
//
// Use Failer only when the transport protocol requires a successful wire-level
// response even on business failure — for example, when a gRPC method must
// return a proto message (not a gRPC status error) to carry structured error
// details:
//
//	type CreateUserResponse struct {
//	    User  *User
//	    Error string `json:"error,omitempty"`
//	    err   error  // unexported, set by business logic
//	}
//
//	func (r CreateUserResponse) Failed() error { return r.err }
type Failer interface {
	Failed() error
}
