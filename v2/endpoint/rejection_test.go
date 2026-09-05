package endpoint_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// The endpoint package does not import apperror, so its errors implement the
// structural apperror.KindNamer contract rather than the typed apperror.Kinder.
// Comparing against the apperror constants here is the point: it pins endpoint's
// own kind strings to apperror's vocabulary without a code dependency, so the
// two cannot drift apart unnoticed.
func TestRejectionErrorsClassifyThemselves(t *testing.T) {
	cases := []struct {
		err  error
		want apperror.Kind
	}{
		{endpoint.ErrRateLimited, apperror.KindResourceExhausted},
		{endpoint.ErrBackpressure, apperror.KindUnavailable},
		{endpoint.ErrBulkheadFull, apperror.KindUnavailable},
		{endpoint.ErrCircuitOpen, apperror.KindUnavailable},
	}
	for _, tc := range cases {
		var namer apperror.KindNamer
		if !errors.As(tc.err, &namer) {
			t.Fatalf("%v does not implement apperror.KindNamer", tc.err)
		}
		if got := namer.ErrorKindName(); got != string(tc.want) {
			t.Errorf("%v: kind name %q, want %q", tc.err, got, string(tc.want))
		}
	}
}

func TestRetryAfterErrorPreservesIdentityAndKind(t *testing.T) {
	err := endpoint.NewRetryAfterError(endpoint.ErrCircuitOpen, 5*time.Second)

	if !errors.Is(err, endpoint.ErrCircuitOpen) {
		t.Fatal("errors.Is must still match the wrapped sentinel")
	}
	var namer apperror.KindNamer
	if !errors.As(err, &namer) || namer.ErrorKindName() != string(apperror.KindUnavailable) {
		t.Fatal("classification must survive the wrapper")
	}
	var reporter endpoint.RetryAfterReporter
	if !errors.As(err, &reporter) {
		t.Fatal("wrapper must report a retry delay")
	}
	if got := reporter.RetryAfter(); got != 5*time.Second {
		t.Errorf("RetryAfter: got %v, want 5s", got)
	}
	if err.Error() != endpoint.ErrCircuitOpen.Error() {
		t.Errorf("message: got %q, want %q", err.Error(), endpoint.ErrCircuitOpen.Error())
	}
}

func TestNewRetryAfterErrorSkipsUselessWrapping(t *testing.T) {
	if got := endpoint.NewRetryAfterError(nil, time.Second); got != nil {
		t.Errorf("nil error: got %v, want nil", got)
	}
	if got := endpoint.NewRetryAfterError(endpoint.ErrRateLimited, 0); got != endpoint.ErrRateLimited {
		t.Errorf("zero delay should return the error unchanged, got %v", got)
	}
}

func TestOpenBreakerReportsRetryAfter(t *testing.T) {
	breaker := endpoint.NewCircuitBreaker(
		endpoint.WithBreakerFailureThreshold(1),
		endpoint.WithBreakerOpenTimeout(time.Minute),
	)
	failing := func(context.Context, any) (any, error) { return nil, errors.New("boom") }
	ep := endpoint.NewBuilder(failing).WithCircuitBreaker(breaker).Build()

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
	ep := endpoint.RateLimitMiddleware(retryAfterLimiter{after: 2 * time.Second})(
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
	limiter := endpoint.RateLimiterFuncs{AllowFn: func() bool { return false }}
	ep := endpoint.RateLimitMiddleware(limiter)(
		func(context.Context, any) (any, error) { return "ok", nil },
	)

	_, err := ep(context.Background(), nil)
	if err != endpoint.ErrRateLimited {
		t.Fatalf("got %v, want the bare sentinel", err)
	}
}
