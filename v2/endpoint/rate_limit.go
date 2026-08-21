package endpoint

import (
	"context"
	"errors"
)

// ErrRateLimited is returned when the rate limiter rejects the request.
var ErrRateLimited = errors.New("rate limit exceeded")

// RateLimiter admits or delays requests. Implementations such as a token
// bucket are application owned; the endpoint package defines only the
// contract and the two middleware adapters.
type RateLimiter interface {
	// Allow reports whether a request may proceed immediately.
	Allow() bool
	// Wait blocks until a request may proceed or the context is cancelled.
	Wait(ctx context.Context) error
}

// RateLimiterFunc adapts a function pair to RateLimiter.
type RateLimiterFunc struct {
	AllowFn func() bool
	WaitFn  func(ctx context.Context) error
}

// Allow implements RateLimiter.
func (f RateLimiterFunc) Allow() bool {
	if f.AllowFn == nil {
		return true
	}
	return f.AllowFn()
}

// Wait implements RateLimiter.
func (f RateLimiterFunc) Wait(ctx context.Context) error {
	if f.WaitFn == nil {
		return nil
	}
	return f.WaitFn(ctx)
}

// RateLimitMiddleware rejects requests over the limit with ErrRateLimited.
// Use DelayRateLimitMiddleware when over-limit requests should wait for a
// token instead of failing.
//
// Example:
//
//	limiter := ratelimit.New(20) // application-owned bucket
//	ep = endpoint.NewBuilder(createUser).Use(endpoint.RateLimitMiddleware(limiter)).Build()
func RateLimitMiddleware(limit RateLimiter) Middleware {
	return func(next Endpoint) Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			if !limit.Allow() {
				return nil, ErrRateLimited
			}
			return next(ctx, request)
		}
	}
}

// DelayRateLimitMiddleware throttles requests over the limit by waiting for
// the limiter; context cancellation aborts the wait.
func DelayRateLimitMiddleware(limit RateLimiter) Middleware {
	return func(next Endpoint) Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			if err := limit.Wait(ctx); err != nil {
				return nil, err
			}
			return next(ctx, request)
		}
	}
}
