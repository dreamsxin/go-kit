package resilience

import (
	"github.com/dreamsxin/go-kit/v2/endpoint"

	"context"
	"time"
)

// RateLimiter admits or delays requests. Implementations such as a token
// bucket are application owned; the endpoint package defines only the
// contract and the two middleware adapters.
//
// It limits the process as a whole: Allow takes no key, so every caller draws
// on the same allowance and a single noisy tenant can reject everybody. Use
// KeyedRateLimiter when the limit belongs to a caller rather than to the
// service.
//
// A RateLimiter may also implement endpoint.RetryAfterReporter. RateLimitMiddleware
// then attaches the reported delay to endpoint.ErrRateLimited so transports can emit a
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

// RateLimitMiddleware rejects requests over the limit with endpoint.ErrRateLimited.
// Use DelayRateLimitMiddleware when over-limit requests should wait for a
// token instead of failing.
//
// It panics when limit is nil so misassembly fails at startup rather than on
// the first request.
//
// Example:
//
//	limiter := ratelimit.New(20) // application-owned bucket
//	ep = endpoint.NewBuilder(createUser).Use(RateLimitMiddleware(limiter)).Build()
func RateLimitMiddleware(limit RateLimiter) endpoint.Middleware {
	if limit == nil {
		panic("endpoint: rate limiter cannot be nil")
	}
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			if !limit.Allow() {
				return nil, withReportedRetryAfter(endpoint.ErrRateLimited, limit)
			}
			return next(ctx, request)
		}
	}
}

// DelayRateLimitMiddleware throttles requests over the limit by waiting for
// the limiter; context cancellation aborts the wait. It panics when limit is
// nil.
func DelayRateLimitMiddleware(limit RateLimiter) endpoint.Middleware {
	if limit == nil {
		panic("endpoint: rate limiter cannot be nil")
	}
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			if err := limit.Wait(ctx); err != nil {
				return nil, err
			}
			return next(ctx, request)
		}
	}
}

// KeyedRateLimiter admits or delays requests per key, so one caller cannot
// spend another caller's allowance. Implementations — a bucket per tenant, a
// Redis counter per API key — are application owned; this package defines only
// the contract and the middleware adapters.
//
// RateLimiter and KeyedRateLimiter are separate contracts because they are
// separate capabilities: an unkeyed limiter has no per-key state to consult and
// must not be able to claim otherwise by ignoring an argument.
//
// A KeyedRateLimiter may also implement KeyedRetryAfterReporter, or the unkeyed
// endpoint.RetryAfterReporter when every key shares a refill rate.
type KeyedRateLimiter interface {
	// AllowKey reports whether a request under key may proceed immediately.
	AllowKey(ctx context.Context, key string) bool
	// WaitKey blocks until a request under key may proceed or the context is
	// cancelled.
	WaitKey(ctx context.Context, key string) error
}

// KeyedRetryAfterReporter is the optional contract a KeyedRateLimiter
// implements to report how long the caller behind one key should wait. The
// keyed middleware prefers it over the unkeyed endpoint.RetryAfterReporter.
type KeyedRetryAfterReporter interface {
	// RetryAfterForKey reports the wait for key. A non-positive value means
	// the delay is unknown and no hint is emitted.
	RetryAfterForKey(key string) time.Duration
}

// RateLimitKeyFunc derives the key a request is limited under: a tenant id, an
// API key, a client IP. What identifies a caller is the deployment's to decide,
// so the framework asks rather than guesses — read it off the context that
// authentication populated, or off the request.
//
// An empty key is not "no limit". Every request that cannot be identified is
// limited under the empty key, which means unidentified callers share one
// bucket rather than each getting their own or bypassing the limit entirely. A
// missing header must not be a way out.
type RateLimitKeyFunc func(ctx context.Context, request any) string

// KeyedRateLimiterFuncs adapts a pair of functions to KeyedRateLimiter. A nil
// field behaves as an unlimited limiter for that operation.
type KeyedRateLimiterFuncs struct {
	AllowKeyFn func(ctx context.Context, key string) bool
	WaitKeyFn  func(ctx context.Context, key string) error
}

// AllowKey implements KeyedRateLimiter.
func (f KeyedRateLimiterFuncs) AllowKey(ctx context.Context, key string) bool {
	if f.AllowKeyFn == nil {
		return true
	}
	return f.AllowKeyFn(ctx, key)
}

// WaitKey implements KeyedRateLimiter.
func (f KeyedRateLimiterFuncs) WaitKey(ctx context.Context, key string) error {
	if f.WaitKeyFn == nil {
		return nil
	}
	return f.WaitKeyFn(ctx, key)
}

// KeyedRateLimitMiddleware rejects requests over the limit for their own key
// with endpoint.ErrRateLimited. Use DelayKeyedRateLimitMiddleware when over-limit
// requests should wait for a token instead of failing.
//
// It panics when limit or key is nil, so misassembly fails at startup rather
// than on the first request. A nil key function is the dangerous case: it would
// silently limit every caller under one key, which is the process-wide limit
// this exists to replace.
//
//	ep = endpoint.NewBuilder(createUser).
//	    WithKeyedRateLimit(buckets, func(ctx context.Context, _ any) string {
//	        subject, _ := security.SubjectFromContext(ctx)
//	        return subject.Tenant
//	    }).
//	    Build()
//
// Stable: resilience.keyed-rate-limit — a keyed limiter is consulted per request key, so one caller's excess cannot reject another's request.
// Covered by: TestKeyedRateLimitMiddlewareLimitsEachKeySeparately
//
// Stable: resilience.keyed-rate-limit-shares-one-bucket — a request whose key is empty is limited under the empty key, never exempted.
// Covered by: TestKeyedRateLimitMiddlewareLimitsAnUnidentifiedCaller
func KeyedRateLimitMiddleware(limit KeyedRateLimiter, key RateLimitKeyFunc) endpoint.Middleware {
	if limit == nil {
		panic("endpoint: keyed rate limiter cannot be nil")
	}
	if key == nil {
		panic("endpoint: keyed rate limit key function cannot be nil")
	}
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			k := key(ctx, request)
			if !limit.AllowKey(ctx, k) {
				return nil, withKeyedRetryAfter(endpoint.ErrRateLimited, limit, k)
			}
			return next(ctx, request)
		}
	}
}

// DelayKeyedRateLimitMiddleware throttles requests over the limit for their own
// key by waiting for the limiter; context cancellation aborts the wait. It
// panics when limit or key is nil.
func DelayKeyedRateLimitMiddleware(limit KeyedRateLimiter, key RateLimitKeyFunc) endpoint.Middleware {
	if limit == nil {
		panic("endpoint: keyed rate limiter cannot be nil")
	}
	if key == nil {
		panic("endpoint: keyed rate limit key function cannot be nil")
	}
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			if err := limit.WaitKey(ctx, key(ctx, request)); err != nil {
				return nil, err
			}
			return next(ctx, request)
		}
	}
}

// withKeyedRetryAfter prefers the per-key hint and falls back to the limiter's
// unkeyed one, so a limiter with one refill rate for every key does not have to
// implement both.
func withKeyedRetryAfter(err error, limit KeyedRateLimiter, key string) error {
	if reporter, ok := limit.(KeyedRetryAfterReporter); ok {
		if after := reporter.RetryAfterForKey(key); after > 0 {
			return endpoint.NewRetryAfterError(err, after)
		}
	}
	return withReportedRetryAfter(err, limit)
}
