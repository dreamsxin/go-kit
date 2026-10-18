package resilience_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/endpoint/resilience"
)

func TestOpenBreakerReportsRetryAfter(t *testing.T) {
	breaker := resilience.NewCircuitBreaker(
		resilience.WithBreakerFailureThreshold(1),
		resilience.WithBreakerOpenTimeout(time.Minute),
	)
	failing := func(context.Context, any) (any, error) { return nil, errors.New("boom") }
	ep := endpoint.NewBuilder(failing).Use(breaker.Middleware()).Build()

	if _, err := ep(context.Background(), nil); err == nil {
		t.Fatal("first call should fail and trip the breaker")
	}

	_, err := ep(context.Background(), nil)
	if !errors.Is(err, endpoint.ErrCircuitOpen) {
		t.Fatalf("second call: got %v, want ErrCircuitOpen", err)
	}
	var reporter endpoint.RetryAfterReporter
	if !errors.As(err, &reporter) {
		t.Fatal("open breaker must report a retry delay")
	}
	if after := reporter.RetryAfter(); after <= 0 || after > time.Minute {
		t.Errorf("RetryAfter: got %v, want a value within the open window", after)
	}
}

// retryAfterLimiter is a RateLimiter that also reports a retry delay.
type retryAfterLimiter struct{ after time.Duration }

func (retryAfterLimiter) Allow() bool                 { return false }
func (retryAfterLimiter) Wait(context.Context) error  { return nil }
func (l retryAfterLimiter) RetryAfter() time.Duration { return l.after }

func TestRateLimitMiddlewareForwardsLimiterRetryAfter(t *testing.T) {
	ep := resilience.RateLimitMiddleware(retryAfterLimiter{after: 2 * time.Second})(
		func(context.Context, any) (any, error) { return "ok", nil },
	)

	_, err := ep(context.Background(), nil)
	if !errors.Is(err, endpoint.ErrRateLimited) {
		t.Fatalf("got %v, want ErrRateLimited", err)
	}
	var reporter endpoint.RetryAfterReporter
	if !errors.As(err, &reporter) {
		t.Fatal("rejection must carry the limiter's retry delay")
	}
	if got := reporter.RetryAfter(); got != 2*time.Second {
		t.Errorf("RetryAfter: got %v, want 2s", got)
	}
}

func TestRateLimitMiddlewareWithoutRetryAfterStaysBare(t *testing.T) {
	limiter := resilience.RateLimiterFuncs{AllowFn: func() bool { return false }}
	ep := resilience.RateLimitMiddleware(limiter)(
		func(context.Context, any) (any, error) { return "ok", nil },
	)

	_, err := ep(context.Background(), nil)
	if err != endpoint.ErrRateLimited {
		t.Fatalf("got %v, want the bare sentinel", err)
	}
}
