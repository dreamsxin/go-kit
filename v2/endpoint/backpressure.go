package endpoint

import (
	"context"
	"sync/atomic"
)

// BackpressureMiddleware returns a Middleware that limits the number of
// concurrent in-flight requests to max.  When the limit is reached, new
// requests are rejected immediately with ErrBackpressure.
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
//	ep = endpoint.BackpressureMiddleware(100)(ep)
func BackpressureMiddleware(max int64) Middleware {
	if max < 1 {
		max = 1
	}
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

// WithBackpressure appends a BackpressureMiddleware to the Builder.
func (b *Builder) WithBackpressure(max int64) *Builder {
	return b.UseNamed("backpressure", BackpressureMiddleware(max))
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
func InFlightMiddleware(max int64, counter *int64) Middleware {
	if counter == nil {
		panic("endpoint: in-flight counter cannot be nil")
	}
	if max < 1 {
		max = 1
	}
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
