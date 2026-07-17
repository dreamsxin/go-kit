package endpoint

import (
	"context"
	"errors"
	"sync/atomic"
)

// ErrBackpressure is returned when the concurrency limit is exceeded.
var ErrBackpressure = errors.New("too many concurrent requests")

// BackpressureMiddleware returns a Middleware that limits the number of
// concurrent in-flight requests to max.  When the limit is reached, new
// requests are rejected immediately with ErrBackpressure.
//
// This is essential for large-scale systems to prevent cascading failures
// when a downstream service slows down.
//
// Example:
//
//	// Allow at most 100 concurrent requests
//	ep = endpoint.BackpressureMiddleware(100)(ep)
func BackpressureMiddleware(max int64) Middleware {
	var inflight int64
	return func(next Endpoint) Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			cur := atomic.AddInt64(&inflight, 1)
			defer atomic.AddInt64(&inflight, -1)
			if cur > max {
				return nil, ErrBackpressure
			}
			return next(ctx, request)
		}
	}
}

// InFlightMiddleware is like BackpressureMiddleware but also exposes the
// current in-flight count via the provided pointer.  Useful for metrics.
//
// Example:
//
//	var inflight int64
//	ep = endpoint.InFlightMiddleware(100, &inflight)(ep)
//	// inflight is updated atomically on every call
func InFlightMiddleware(max int64, counter *int64) Middleware {
	return func(next Endpoint) Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			cur := atomic.AddInt64(counter, 1)
			defer atomic.AddInt64(counter, -1)
			if cur > max {
				return nil, ErrBackpressure
			}
			return next(ctx, request)
		}
	}
}
