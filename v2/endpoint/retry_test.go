package endpoint_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// noBackoff removes the wait so retry tests stay fast and deterministic.
var noBackoff = endpoint.WithRetryBackoff(func(int) time.Duration { return 0 })

func TestRetryMiddlewareRetriesUnavailableUntilSuccess(t *testing.T) {
	calls := 0
	flaky := func(context.Context, any) (any, error) {
		calls++
		if calls < 3 {
			return nil, apperror.Unavailable("dep.down", "dependency unavailable")
		}
		return "ok", nil
	}
	ep := endpoint.NewBuilder(flaky).WithRetry(3, noBackoff).Build()

	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "ok" {
		t.Errorf("response: got %v, want ok", resp)
	}
	if calls != 3 {
		t.Errorf("calls: got %d, want 3", calls)
	}
}

func TestRetryMiddlewareStopsAtMaxAttempts(t *testing.T) {
	calls := 0
	always := func(context.Context, any) (any, error) {
		calls++
		return nil, apperror.Unavailable("dep.down", "dependency unavailable")
	}
	ep := endpoint.RetryMiddleware(2, noBackoff)(always)

	if _, err := ep(context.Background(), nil); err == nil {
		t.Fatal("want the last error, got nil")
	}
	if calls != 2 {
		t.Errorf("calls: got %d, want 2", calls)
	}
}

func TestRetryMiddlewareDoesNotRetryNonTransientErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"not found", apperror.NotFound("user.missing", "no such user")},
		{"unclassified", errors.New("boom")},
		{"circuit open", endpoint.ErrCircuitOpen},
		{"rate limited", endpoint.ErrRateLimited},
		{"bulkhead full", endpoint.ErrBulkheadFull},
		{"backpressure", endpoint.ErrBackpressure},
		{"deadline", context.DeadlineExceeded},
		{"canceled", context.Canceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			ep := endpoint.RetryMiddleware(3, noBackoff)(
				func(context.Context, any) (any, error) { calls++; return nil, tc.err },
			)
			if _, err := ep(context.Background(), nil); !errors.Is(err, tc.err) {
				t.Fatalf("error: got %v, want %v", err, tc.err)
			}
			if calls != 1 {
				t.Errorf("calls: got %d, want 1", calls)
			}
		})
	}
}

func TestRetryMiddlewareHonorsCustomClassifier(t *testing.T) {
	sentinel := errors.New("retry me")
	calls := 0
	ep := endpoint.RetryMiddleware(3, noBackoff, endpoint.WithRetryable(func(err error) bool {
		return errors.Is(err, sentinel)
	}))(func(context.Context, any) (any, error) { calls++; return nil, sentinel })

	if _, err := ep(context.Background(), nil); !errors.Is(err, sentinel) {
		t.Fatalf("error: got %v", err)
	}
	if calls != 3 {
		t.Errorf("calls: got %d, want 3", calls)
	}
}

func TestRetryMiddlewareStopsWhenContextIsDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	ep := endpoint.RetryMiddleware(5, endpoint.WithRetryBackoff(func(int) time.Duration {
		return time.Hour
	}))(func(context.Context, any) (any, error) {
		calls++
		cancel()
		return nil, apperror.Unavailable("dep.down", "dependency unavailable")
	})

	if _, err := ep(ctx, nil); err == nil {
		t.Fatal("want the endpoint error, got nil")
	}
	if calls != 1 {
		t.Errorf("calls: got %d, want 1", calls)
	}
}

func TestRetryMiddlewareSingleAttemptDisablesRetrying(t *testing.T) {
	calls := 0
	ep := endpoint.RetryMiddleware(0)(
		func(context.Context, any) (any, error) {
			calls++
			return nil, apperror.Unavailable("dep.down", "dependency unavailable")
		},
	)

	if _, err := ep(context.Background(), nil); err == nil {
		t.Fatal("want an error, got nil")
	}
	if calls != 1 {
		t.Errorf("calls: got %d, want 1", calls)
	}
}

func TestDefaultRetryableIgnoresNil(t *testing.T) {
	if endpoint.DefaultRetryable(nil) {
		t.Error("nil error must not be retryable")
	}
}

// selfClassifiedError is the optional contract transport clients use to expose
// protocol knowledge the endpoint layer does not have.
type selfClassifiedError struct {
	retryable bool
	kind      apperror.Kind
}

func (e selfClassifiedError) Error() string            { return "self classified" }
func (e selfClassifiedError) Retryable() bool          { return e.retryable }
func (e selfClassifiedError) ErrorKind() apperror.Kind { return e.kind }

func TestDefaultRetryableHonorsSelfClassification(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"retryable internal", selfClassifiedError{retryable: true, kind: apperror.KindInternal}, true},
		{"non-retryable unavailable", selfClassifiedError{retryable: false, kind: apperror.KindUnavailable}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := endpoint.DefaultRetryable(tc.err); got != tc.want {
				t.Errorf("DefaultRetryable = %v, want %v", got, tc.want)
			}
		})
	}
}

// A local rejection still wins over self-classification, so a tripped breaker
// is never hammered by a client error that calls itself retryable.
func TestDefaultRetryableRejectionsOutrankSelfClassification(t *testing.T) {
	if endpoint.DefaultRetryable(endpoint.NewRetryAfterError(endpoint.ErrCircuitOpen, time.Second)) {
		t.Error("an open circuit must not be retryable")
	}
}

type retryAfterError struct {
	after time.Duration
}

func (e retryAfterError) Error() string             { return "slow down" }
func (e retryAfterError) RetryAfter() time.Duration { return e.after }
func (e retryAfterError) Retryable() bool           { return true }

func TestRetryMiddlewarePrefersReportedRetryAfterOverBackoff(t *testing.T) {
	var waits []time.Duration
	calls := 0
	ep := endpoint.RetryMiddleware(2,
		endpoint.WithRetryBackoff(func(int) time.Duration {
			waits = append(waits, time.Hour)
			return time.Hour
		}),
	)(func(context.Context, any) (any, error) {
		calls++
		return nil, retryAfterError{after: time.Millisecond}
	})

	if _, err := ep(context.Background(), nil); err == nil {
		t.Fatal("want the last error, got nil")
	}
	if calls != 2 {
		t.Errorf("calls: got %d, want 2", calls)
	}
	if len(waits) != 0 {
		t.Errorf("backoff schedule was consulted despite a retry hint: %v", waits)
	}
}

func TestRetryMiddlewareCapsReportedRetryAfter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	calls := 0
	start := time.Now()
	ep := endpoint.RetryMiddleware(2, noBackoff)(func(context.Context, any) (any, error) {
		calls++
		return nil, retryAfterError{after: 24 * time.Hour}
	})

	if _, err := ep(ctx, nil); err == nil {
		t.Fatal("want the endpoint error, got nil")
	}
	if calls != 1 {
		t.Errorf("calls: got %d, want 1", calls)
	}
	// The context ends the wait; without the cap a hostile hint could also park
	// a caller that has no deadline at all.
	if elapsed := time.Since(start); elapsed > endpoint.MaxRetryAfterHint {
		t.Errorf("waited %v, want at most %v", elapsed, endpoint.MaxRetryAfterHint)
	}
}
