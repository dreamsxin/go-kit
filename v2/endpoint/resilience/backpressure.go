package resilience

import (
	"github.com/dreamsxin/go-kit/v2/endpoint"

	"context"
	"sync/atomic"
)

// BackpressureMiddleware returns a endpoint.Middleware that limits the number of
// concurrent in-flight requests to max.  When the limit is reached, new
// requests are rejected immediately with endpoint.ErrBackpressure.
//
// This is essential for large-scale systems to prevent cascading failures
// when a downstream service slows down.
//
// A max below 1 is clamped to 1, as in BulkheadMiddleware. An unset limit would
// otherwise reject every request, which no caller writing 0 intends.
//
// Example:
//
//	// Allow at most 100 concurrent requests
//	ep = BackpressureMiddleware(100)(ep)
func BackpressureMiddleware(max int64) endpoint.Middleware {
	if max < 1 {
		max = 1
	}
	var inflight int64
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			cur := atomic.AddInt64(&inflight, 1)
			defer atomic.AddInt64(&inflight, -1)
			if cur > max {
				return nil, endpoint.ErrBackpressure
			}
			return next(ctx, request)
		}
	}
}

// InFlightMiddleware is like BackpressureMiddleware but also exposes the
// current in-flight count via the provided pointer.  Useful for metrics.
//
// It panics when counter is nil so misassembly fails at startup rather than on
// the first request. A max below 1 is clamped to 1, as in
// BackpressureMiddleware.
//
// Example:
//
//	var inflight int64
//	ep = endpoint.InFlightMiddleware(100, &inflight)(ep)
//	// inflight is updated atomically on every call
func InFlightMiddleware(max int64, counter *int64) endpoint.Middleware {
	if counter == nil {
		panic("endpoint: in-flight counter cannot be nil")
	}
	if max < 1 {
		max = 1
	}
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			cur := atomic.AddInt64(counter, 1)
			defer atomic.AddInt64(counter, -1)
			if cur > max {
				return nil, endpoint.ErrBackpressure
			}
			return next(ctx, request)
		}
	}
}
