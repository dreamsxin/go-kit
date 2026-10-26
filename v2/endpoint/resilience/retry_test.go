package resilience_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/endpoint/resilience"
)

// noBackoff removes the wait so retry tests stay fast and deterministic.
var noBackoff = resilience.WithRetryBackoff(func(int) time.Duration { return 0 })

func TestRetryMiddlewareRetriesUnavailableUntilSuccess(t *testing.T) {
	calls := 0
	flaky := func(context.Context, any) (any, error) {
		calls++
		if calls < 3 {
			return nil, apperror.Unavailable("dep.down", "dependency unavailable")
		}
		return "ok", nil
	}
	ep := endpoint.NewBuilder(flaky).Use(resilience.RetryMiddleware(3, noBackoff)).Build()

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
	ep := resilience.RetryMiddleware(2, noBackoff)(always)

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
			ep := resilience.RetryMiddleware(3, noBackoff)(
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
	ep := resilience.RetryMiddleware(3, noBackoff, resilience.WithRetryable(func(err error) bool {
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
	ep := resilience.RetryMiddleware(5, resilience.WithRetryBackoff(func(int) time.Duration {
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
	ep := resilience.RetryMiddleware(0)(
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
	if resilience.DefaultRetryable(nil) {
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
			if got := resilience.DefaultRetryable(tc.err); got != tc.want {
				t.Errorf("DefaultRetryable = %v, want %v", got, tc.want)
			}
		})
	}
}

// A local rejection still wins over self-classification, so a tripped breaker
// is never hammered by a client error that calls itself retryable.
func TestDefaultRetryableRejectionsOutrankSelfClassification(t *testing.T) {
	if resilience.DefaultRetryable(endpoint.NewRetryAfterError(endpoint.ErrCircuitOpen, time.Second)) {
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
	ep := resilience.RetryMiddleware(2,
		resilience.WithRetryBackoff(func(int) time.Duration {
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
	ep := resilience.RetryMiddleware(2, noBackoff)(func(context.Context, any) (any, error) {
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
	if elapsed := time.Since(start); elapsed > resilience.MaxRetryAfterHint {
		t.Errorf("waited %v, want at most %v", elapsed, resilience.MaxRetryAfterHint)
	}
}

// TestRetryAdvancesTheSpanPerAttempt pins the correlation property: every
// attempt the retry makes runs on its own span ID, so downstream records
// attribute to attempt N; the trace and the request ID stay one operation's.
func TestRetryAdvancesTheSpanPerAttempt(t *testing.T) {
	type spanRecord struct {
		span    endpoint.SpanID
		attempt int
	}
	var seen []spanRecord
	fail := errors.New("flaky")
	ep := resilience.RetryMiddleware(3, noBackoff, resilience.WithRetryable(func(error) bool { return true }))(func(ctx context.Context, _ any) (any, error) {
		seen = append(seen, spanRecord{
			span:    endpoint.SpanIDFromContext(ctx),
			attempt: len(seen) + 1,
		})
		if len(seen) < 3 {
			return nil, fail
		}
		return "ok", nil
	})

	response, err := ep(endpoint.ContinueIncomingTrace(context.Background()), nil)
	if err != nil || response != "ok" {
		t.Fatalf("response=%v err=%v", response, err)
	}
	if len(seen) != 3 {
		t.Fatalf("attempts = %d, want 3", len(seen))
	}
	ids := map[endpoint.SpanID]bool{seen[0].span: true, seen[1].span: true, seen[2].span: true}
	if len(ids) != 3 {
		t.Fatalf("attempts shared span IDs: %v", seen)
	}
}
