package endpoint

import (
	"context"
	"errors"
	"sync"
)

// ErrBulkheadFull is returned when a bulkhead's per-key concurrency limit is
// reached and the request context gives up waiting.
var ErrBulkheadFull = errors.New("bulkhead full")

// ErrCircuitOpen is returned by CircuitBreaker while the breaker rejects
// calls.
var ErrCircuitOpen = errors.New("circuit breaker open")

// Fallback returns a Middleware that answers with the fallback endpoint when
// the wrapped endpoint fails. Successful calls never touch the fallback.
//
// Use it to degrade gracefully instead of propagating a dependency failure:
// serve cached or default data while the primary dependency recovers. The
// fallback receives the same context and request, so it can distinguish
// callers if needed.
//
// Example:
//
//	ep := endpoint.NewBuilder(recommend).
//	    WithFallback(defaultRecommendations).
//	    Build()
func Fallback(fallback Endpoint) Middleware {
	return func(next Endpoint) Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			resp, err := next(ctx, request)
			if err == nil {
				return resp, nil
			}
			return fallback(ctx, request)
		}
	}
}

// WithFallback appends a Fallback middleware to the Builder.
func (b *Builder) WithFallback(fallback Endpoint) *Builder {
	return b.Use(Fallback(fallback))
}

// BulkheadMiddleware limits concurrent requests per resource key, so one
// slow tenant or dependency cannot consume the concurrency budget of the
// others. Unlike BackpressureMiddleware, which counts globally, each key
// gets an isolated slot pool of maxPerKey.
//
// key extracts the isolation key from the request. A nil key function puts
// every request into one shared bulkhead. The key space must stay bounded
// (dependency names, tenant buckets); the middleware keeps one slot pool per
// distinct key for the lifetime of the endpoint.
//
// Requests that arrive while their key's pool is full wait until a slot
// frees or the context is cancelled; cancellation returns ErrBulkheadFull.
//
// Example:
//
//	ep := endpoint.NewBuilder(callDependency).
//	    WithBulkhead(20, func(req any) string {
//	        return req.(Request).TenantID
//	    }).
//	    Build()
func BulkheadMiddleware(maxPerKey int, key func(request any) string) Middleware {
	if maxPerKey < 1 {
		maxPerKey = 1
	}
	var (
		mu     sync.Mutex
		pools  = make(map[string]chan struct{})
		shared chan struct{}
	)
	if key == nil {
		shared = make(chan struct{}, maxPerKey)
	}
	poolFor := func(request any) chan struct{} {
		if shared != nil {
			return shared
		}
		k := key(request)
		mu.Lock()
		defer mu.Unlock()
		pool, ok := pools[k]
		if !ok {
			pool = make(chan struct{}, maxPerKey)
			pools[k] = pool
		}
		return pool
	}

	return func(next Endpoint) Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			pool := poolFor(request)
			select {
			case pool <- struct{}{}:
				defer func() { <-pool }()
			case <-ctx.Done():
				return nil, errors.Join(ErrBulkheadFull, ctx.Err())
			}
			return next(ctx, request)
		}
	}
}

// WithBulkhead appends a BulkheadMiddleware to the Builder.
func (b *Builder) WithBulkhead(maxPerKey int, key func(request any) string) *Builder {
	return b.Use(BulkheadMiddleware(maxPerKey, key))
}
