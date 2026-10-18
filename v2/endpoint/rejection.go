package endpoint

import (
	"time"
)

// rejectionError is a framework-owned admission-control failure. It carries a
// transport-neutral classification so every transport maps it through the
// normal classification rules instead of special-casing the sentinel values,
// which kept HTTP and gRPC from agreeing on a status.
type rejectionError struct {
	message string
	kind    string
}

func (e *rejectionError) Error() string { return e.message }

// ErrorKindName classifies the rejection through the structural contract every
// transport reads.
func (e *rejectionError) ErrorKindName() string { return e.kind }

// Rejection errors reported by the admission-control middleware. Each one
// classifies itself, so HTTP and gRPC derive the same meaning from it:
//
//   - ErrRateLimited is resource_exhausted (HTTP 429 / gRPC ResourceExhausted),
//     because the caller exceeded its own quota and may retry later.
//   - ErrBackpressure, ErrBulkheadFull, and ErrCircuitOpen are unavailable
//     (HTTP 503 / gRPC Unavailable), because the service or its dependency is
//     shedding load rather than blaming the caller. This matches how Envoy
//     reports circuit-breaker overflow and how Resilience4j surfaces a
//     rejected call.
//
// Compare with errors.Is; the values stay comparable.
var (
	ErrBackpressure error = &rejectionError{
		message: "too many concurrent requests",
		kind:    kindUnavailable,
	}
	ErrBulkheadFull error = &rejectionError{
		message: "bulkhead full",
		kind:    kindUnavailable,
	}
	ErrCircuitOpen error = &rejectionError{
		message: "circuit breaker open",
		kind:    kindUnavailable,
	}
	ErrRateLimited error = &rejectionError{
		message: "rate limit exceeded",
		kind:    kindResourceExhausted,
	}
)

// RetryAfterReporter is the optional contract an error implements to tell a
// transport how long the client should wait before retrying. The HTTP error
// encoders translate it into a Retry-After header.
//
// A RateLimiter may implement it too; RateLimitMiddleware then attaches the
// reported delay to ErrRateLimited.
type RetryAfterReporter interface {
	// RetryAfter reports the wait before a retry may succeed. A non-positive
	// value means the delay is unknown and no hint is emitted.
	RetryAfter() time.Duration
}

// RetryAfterError annotates an error with a retry delay without changing its
// identity: errors.Is and errors.As still see the wrapped error, including its
// classification.
type RetryAfterError struct {
	// Err is the wrapped error.
	Err error
	// After is the wait before a retry may succeed.
	After time.Duration
}

// NewRetryAfterError wraps err with a retry delay. It returns err unchanged
// when there is nothing to add, so callers can use it unconditionally.
func NewRetryAfterError(err error, after time.Duration) error {
	if err == nil || after <= 0 {
		return err
	}
	return &RetryAfterError{Err: err, After: after}
}

func (e *RetryAfterError) Error() string { return e.Err.Error() }

// Unwrap exposes the wrapped error to errors.Is and errors.As.
func (e *RetryAfterError) Unwrap() error { return e.Err }

// RetryAfter implements RetryAfterReporter.
func (e *RetryAfterError) RetryAfter() time.Duration { return e.After }
