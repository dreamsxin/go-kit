package endpoint

import (
	"context"
)

// RateLimiter admits or delays requests. Implementations such as a token
// bucket are application owned; the endpoint package defines only the
// contract and the two middleware adapters.
//
// A RateLimiter may also implement RetryAfterReporter. RateLimitMiddleware
// then attaches the reported delay to ErrRateLimited so transports can emit a
// Retry-After hint.
type RateLimiter interface {
	// Allow reports whether a request may proceed immediately.
	Allow() bool
	// Wait blocks until a request may proceed or the context is cancelled.
	Wait(ctx context.Context) error
}

// RateLimiterFuncs adapts a pair of functions to RateLimiter. A nil field
// behaves as an unlimited limiter for that operation.
type RateLimiterFuncs struct {
	AllowFn func() bool
	WaitFn  func(ctx context.Context) error
}

// Allow implements RateLimiter.
func (f RateLimiterFuncs) Allow() bool {
	if f.AllowFn == nil {
		return true
	}
	return f.AllowFn()
}

// Wait implements RateLimiter.
func (f RateLimiterFuncs) Wait(ctx context.Context) error {
	if f.WaitFn == nil {
		return nil
	}
	return f.WaitFn(ctx)
}

// RateLimitMiddleware rejects requests over the limit with ErrRateLimited.
// Use DelayRateLimitMiddleware when over-limit requests should wait for a
// token instead of failing.
//
// It panics when limit is nil so misassembly fails at startup rather than on
// the first request.
//
// Example:
//
//	limiter := ratelimit.New(20) // application-owned bucket
//	ep = endpoint.NewBuilder(createUser).Use(endpoint.RateLimitMiddleware(limiter)).Build()
func RateLimitMiddleware(limit RateLimiter) Middleware {
	if limit == nil {
		panic("endpoint: rate limiter cannot be nil")
	}
	return func(next Endpoint) Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			if !limit.Allow() {
				return nil, withReportedRetryAfter(ErrRateLimited, limit)
			}
			return next(ctx, request)
		}
	}
}

// DelayRateLimitMiddleware throttles requests over the limit by waiting for
// the limiter; context cancellation aborts the wait. It panics when limit is
// nil.
func DelayRateLimitMiddleware(limit RateLimiter) Middleware {
	if limit == nil {
		panic("endpoint: rate limiter cannot be nil")
	}
	return func(next Endpoint) Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			if err := limit.Wait(ctx); err != nil {
				return nil, err
			}
			return next(ctx, request)
		}
	}
}

// WithRateLimit appends a RateLimitMiddleware to the Builder.
func (b *Builder) WithRateLimit(limit RateLimiter) *Builder {
	return b.UseNamed("rate_limit", RateLimitMiddleware(limit))
}

// WithDelayRateLimit appends a DelayRateLimitMiddleware to the Builder.
func (b *Builder) WithDelayRateLimit(limit RateLimiter) *Builder {
	return b.UseNamed("delay_rate_limit", DelayRateLimitMiddleware(limit))
}
