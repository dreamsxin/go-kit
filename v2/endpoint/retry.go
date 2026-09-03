package endpoint

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/dreamsxin/go-kit/v2/apperror"
)

// Retry defaults. RetryMiddleware falls back to these for any non-positive
// setting.
const (
	// DefaultRetryBaseBackoff is the first backoff step; each further attempt
	// doubles it before jitter.
	DefaultRetryBaseBackoff = 50 * time.Millisecond
	// DefaultRetryMaxBackoff caps the backoff step.
	DefaultRetryMaxBackoff = time.Second
	// MaxRetryAfterHint caps how long a server-reported retry hint may delay
	// the next attempt, so a hostile or misconfigured peer cannot park a
	// caller that has no deadline of its own.
	MaxRetryAfterHint = 30 * time.Second
)

// RetryOption configures RetryMiddleware.
type RetryOption func(*retrySettings)

type retrySettings struct {
	backoff   func(attempt int) time.Duration
	retryable func(error) bool
}

// WithRetryBackoff replaces the backoff schedule. attempt is 1 for the wait
// after the first failure. A non-positive result retries immediately.
func WithRetryBackoff(backoff func(attempt int) time.Duration) RetryOption {
	return func(s *retrySettings) {
		if backoff != nil {
			s.backoff = backoff
		}
	}
}

// WithRetryable replaces the retry classifier. Return true only for failures
// that a repeat call can plausibly fix.
func WithRetryable(retryable func(error) bool) RetryOption {
	return func(s *retrySettings) {
		if retryable != nil {
			s.retryable = retryable
		}
	}
}

// DefaultRetryable is the conservative classifier RetryMiddleware uses by
// default. Precedence:
//
//  1. context cancellation and deadlines are never retried, because the caller
//     already gave up
//  2. local admission-control rejections (ErrCircuitOpen, ErrBulkheadFull,
//     ErrBackpressure, ErrRateLimited) are never retried, because an immediate
//     repeat would be rejected again and only add load
//  3. an error that implements the optional contract interface{ Retryable() bool }
//     decides for itself; transport client errors such as
//     *client.HTTPStatusError use it to expose protocol knowledge the endpoint
//     layer does not have
//  4. otherwise only apperror.KindUnavailable is retried
//
// Unclassified errors are not retried, because a business failure is not
// transient.
func DefaultRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, ErrCircuitOpen) || errors.Is(err, ErrBulkheadFull) ||
		errors.Is(err, ErrBackpressure) || errors.Is(err, ErrRateLimited) {
		return false
	}
	var classified interface{ Retryable() bool }
	if errors.As(err, &classified) {
		return classified.Retryable()
	}
	// Both classification contracts are read, so an error is retryable under
	// the same rule whether it implements apperror.Kinder or only the minimal
	// apperror.KindNamer that optional transports use.
	kind, ok := errorKind(err)
	if !ok {
		return false
	}
	return kind == apperror.KindUnavailable
}

// errorKind reads the classification from either the typed apperror.Kinder
// contract or the minimal apperror.KindNamer contract.
func errorKind(err error) (apperror.Kind, bool) {
	var kinder apperror.Kinder
	if errors.As(err, &kinder) {
		return kinder.ErrorKind(), true
	}
	var namer apperror.KindNamer
	if errors.As(err, &namer) {
		return apperror.Kind(namer.ErrorKindName()), true
	}
	return "", false
}

// defaultRetryBackoff is exponential backoff with full jitter, the schedule
// that avoids retry storms from synchronized clients.
func defaultRetryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	step := DefaultRetryBaseBackoff << (attempt - 1)
	if step <= 0 || step > DefaultRetryMaxBackoff {
		step = DefaultRetryMaxBackoff
	}
	return rand.N(step)
}

// RetryMiddleware retries the wrapped endpoint while the error is retryable,
// up to maxAttempts total calls. It waits between attempts and abandons the
// retry loop as soon as the context is done, returning the last error.
//
// The caller owns idempotency: only wrap endpoints where a repeated call is
// safe. maxAttempts below 1 disables retrying.
//
// The default classifier is DefaultRetryable and the default schedule is
// exponential backoff with full jitter. Use WithRetryable and
// WithRetryBackoff to replace either one. When the error reports a retry hint
// through RetryAfterReporter, that hint wins over the schedule, capped by
// MaxRetryAfterHint.
//
// Place it inside a circuit breaker so repeated failures still trip the
// breaker once, not once per attempt:
//
//	ep := endpoint.NewBuilder(callDependency).
//	    WithCircuitBreaker(breaker).
//	    WithRetry(3).
//	    Build()
func RetryMiddleware(maxAttempts int, options ...RetryOption) Middleware {
	settings := retrySettings{
		backoff:   defaultRetryBackoff,
		retryable: DefaultRetryable,
	}
	for _, option := range options {
		if option != nil {
			option(&settings)
		}
	}
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	return func(next Endpoint) Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			var (
				response any
				err      error
			)
			for attempt := 1; ; attempt++ {
				response, err = next(ctx, request)
				if err == nil || attempt >= maxAttempts || !settings.retryable(err) {
					return response, err
				}
				if waitErr := waitBeforeRetry(ctx, retryDelay(err, settings.backoff, attempt)); waitErr != nil {
					return response, err
				}
			}
		}
	}
}

// retryDelay prefers a retry hint carried by the error (a Retry-After header
// parsed by an HTTP client, google.rpc.RetryInfo from gRPC, or a local
// RetryAfterError) over the local schedule, so the peer that knows when it will
// be ready decides the wait. Hints are capped by MaxRetryAfterHint.
func retryDelay(err error, backoff func(attempt int) time.Duration, attempt int) time.Duration {
	var reporter RetryAfterReporter
	if errors.As(err, &reporter) {
		if after := reporter.RetryAfter(); after > 0 {
			if after > MaxRetryAfterHint {
				return MaxRetryAfterHint
			}
			return after
		}
	}
	return backoff(attempt)
}

// waitBeforeRetry sleeps for delay unless the context finishes first, in which
// case it reports the context error so the caller stops retrying.
func waitBeforeRetry(ctx context.Context, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// WithRetry appends a RetryMiddleware to the Builder.
func (b *Builder) WithRetry(maxAttempts int, options ...RetryOption) *Builder {
	return b.UseNamed("retry", RetryMiddleware(maxAttempts, options...))
}
