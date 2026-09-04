package endpoint_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/endpoint"
)

func TestCircuitBreaker_OpensAfterThresholdAndRejects(t *testing.T) {
	breaker := endpoint.NewCircuitBreaker(
		endpoint.WithBreakerFailureThreshold(2),
		endpoint.WithBreakerOpenTimeout(time.Hour),
	)
	down := func(context.Context, any) (any, error) { return nil, errors.New("down") }
	ep := breaker.Middleware()(down)

	for i := 0; i < 2; i++ {
		if _, err := ep(context.Background(), nil); err == nil {
			t.Fatalf("call %d should fail", i)
		}
	}
	if got := breaker.State(); got != endpoint.BreakerOpen {
		t.Fatalf("state: got %v, want open", got)
	}

	_, err := ep(context.Background(), nil)
	if !errors.Is(err, endpoint.ErrCircuitOpen) {
		t.Fatalf("open breaker should reject with ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreaker_ProbeHalfOpenAndClose(t *testing.T) {
	breaker := endpoint.NewCircuitBreaker(
		endpoint.WithBreakerFailureThreshold(1),
		endpoint.WithBreakerOpenTimeout(10*time.Millisecond),
	)
	callCount := 0
	ep := breaker.Middleware()(func(context.Context, any) (any, error) {
		callCount++
		if callCount == 1 {
			return nil, errors.New("first failure trips it")
		}
		return "ok", nil
	})

	_, _ = ep(context.Background(), nil)
	if breaker.State() != endpoint.BreakerOpen {
		t.Fatal("breaker should open after one failure")
	}

	// Rejected while the window is still open.
	if _, err := ep(context.Background(), nil); !errors.Is(err, endpoint.ErrCircuitOpen) {
		t.Fatalf("open window: got %v", err)
	}

	time.Sleep(15 * time.Millisecond)

	// First call after the window is the probe; a success closes the breaker.
	if _, err := ep(context.Background(), nil); err != nil {
		t.Fatalf("probe should pass and succeed: %v", err)
	}
	if breaker.State() != endpoint.BreakerClosed {
		t.Fatalf("state: got %v, want closed", breaker.State())
	}
}

func TestCircuitBreaker_HonorsSuccessThreshold(t *testing.T) {
	breaker := endpoint.NewCircuitBreaker(
		endpoint.WithBreakerFailureThreshold(1),
		endpoint.WithBreakerSuccessThreshold(3),
		endpoint.WithBreakerOpenTimeout(10*time.Millisecond),
	)
	fail := true
	ep := breaker.Middleware()(func(context.Context, any) (any, error) {
		if fail {
			return nil, errors.New("down")
		}
		return "ok", nil
	})

	_, _ = ep(context.Background(), nil)
	if breaker.State() != endpoint.BreakerOpen {
		t.Fatal("breaker should open after one failure")
	}
	fail = false
	time.Sleep(15 * time.Millisecond)

	// The first two probe successes must not close the breaker yet.
	for probe := 1; probe <= 2; probe++ {
		if _, err := ep(context.Background(), nil); err != nil {
			t.Fatalf("probe %d should pass: %v", probe, err)
		}
		if got := breaker.State(); got != endpoint.BreakerHalfOpen {
			t.Fatalf("after %d of 3 successes: got %v, want half-open", probe, got)
		}
	}

	if _, err := ep(context.Background(), nil); err != nil {
		t.Fatalf("third probe should pass: %v", err)
	}
	if got := breaker.State(); got != endpoint.BreakerClosed {
		t.Fatalf("after 3 successes: got %v, want closed", got)
	}
}

func TestCircuitBreaker_DoesNotCountCallerCancellation(t *testing.T) {
	breaker := endpoint.NewCircuitBreaker(endpoint.WithBreakerFailureThreshold(1))
	base := func(ctx context.Context, _ any) (any, error) { return nil, ctx.Err() }
	ep := breaker.Middleware()(base)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ep(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if got := breaker.State(); got != endpoint.BreakerClosed {
		t.Fatalf("state after caller cancellation: got %v, want closed", got)
	}
}

func TestRecoveryMiddlewareConvertsPanicAndKeepsRecoveredValueOutOfError(t *testing.T) {
	var recovered any
	ep := endpoint.RecoveryMiddleware(func(_ context.Context, _ any, value any) error {
		recovered = value
		return apperror.Internal("panic", "request failed")
	})(func(context.Context, any) (any, error) {
		panic("secret stack detail")
	})

	_, err := ep(context.Background(), nil)
	var appErr *apperror.Error
	if err == nil || !errors.As(err, &appErr) {
		t.Fatalf("error = %v, want classified internal error", err)
	}
	if recovered != "secret stack detail" {
		t.Fatalf("recovered = %#v, want panic value", recovered)
	}
	if strings.Contains(err.Error(), "secret stack detail") {
		t.Fatalf("panic value leaked into returned error: %v", err)
	}
}

func TestRecoveryMiddlewareProtectsAgainstPanickingPanicHandler(t *testing.T) {
	ep := endpoint.RecoveryMiddleware(func(context.Context, any, any) error {
		panic("handler panic")
	})(func(context.Context, any) (any, error) {
		panic("endpoint panic")
	})
	_, err := ep(context.Background(), nil)
	if err == nil {
		t.Fatal("recovery handler panic should still return an error")
	}
	if strings.Contains(err.Error(), "handler panic") {
		t.Fatalf("panic detail leaked: %v", err)
	}
}

func TestCircuitBreaker_ProbeFailureReopensAndResetsSuccesses(t *testing.T) {
	breaker := endpoint.NewCircuitBreaker(
		endpoint.WithBreakerFailureThreshold(1),
		endpoint.WithBreakerSuccessThreshold(2),
		endpoint.WithBreakerOpenTimeout(10*time.Millisecond),
	)
	fail := true
	ep := breaker.Middleware()(func(context.Context, any) (any, error) {
		if fail {
			return nil, errors.New("down")
		}
		return "ok", nil
	})

	_, _ = ep(context.Background(), nil)
	fail = false
	time.Sleep(15 * time.Millisecond)

	if _, err := ep(context.Background(), nil); err != nil {
		t.Fatalf("first probe should pass: %v", err)
	}

	// A failing probe reopens the breaker and discards the earlier success.
	fail = true
	if _, err := ep(context.Background(), nil); err == nil {
		t.Fatal("second probe should fail")
	}
	if got := breaker.State(); got != endpoint.BreakerOpen {
		t.Fatalf("state: got %v, want open", got)
	}

	fail = false
	time.Sleep(15 * time.Millisecond)
	if _, err := ep(context.Background(), nil); err != nil {
		t.Fatalf("probe after reopen should pass: %v", err)
	}
	if got := breaker.State(); got != endpoint.BreakerHalfOpen {
		t.Fatalf("success count must restart from zero, got %v", got)
	}
}

func TestNewCircuitBreaker_NonPositiveSettingsUseDefaults(t *testing.T) {
	breaker := endpoint.NewCircuitBreaker(
		endpoint.WithBreakerFailureThreshold(0),
		endpoint.WithBreakerSuccessThreshold(-1),
		endpoint.WithBreakerOpenTimeout(0),
	)
	down := func(context.Context, any) (any, error) { return nil, errors.New("down") }
	ep := breaker.Middleware()(down)

	// The default threshold is 5, so four failures must not trip the breaker.
	for i := 0; i < endpoint.DefaultBreakerFailureThreshold-1; i++ {
		_, _ = ep(context.Background(), nil)
	}
	if got := breaker.State(); got != endpoint.BreakerClosed {
		t.Fatalf("state after %d failures: got %v, want closed", endpoint.DefaultBreakerFailureThreshold-1, got)
	}
	_, _ = ep(context.Background(), nil)
	if got := breaker.State(); got != endpoint.BreakerOpen {
		t.Fatalf("state after %d failures: got %v, want open", endpoint.DefaultBreakerFailureThreshold, got)
	}
}

func TestRateLimitMiddleware_RejectsOverLimit(t *testing.T) {
	calls := 0
	limiter := endpoint.RateLimiterFuncs{
		AllowFn: func() bool { return calls == 0 },
	}
	ep := endpoint.RateLimitMiddleware(limiter)(
		func(context.Context, any) (any, error) { calls++; return "ok", nil },
	)

	if _, err := ep(context.Background(), nil); err != nil {
		t.Fatalf("first call should pass: %v", err)
	}
	if _, err := ep(context.Background(), nil); !errors.Is(err, endpoint.ErrRateLimited) {
		t.Fatalf("over-limit call: got %v", err)
	}
	if calls != 1 {
		t.Fatalf("handler ran %d times, want 1", calls)
	}
}

func TestDelayRateLimitMiddleware_WaitsAndAbortsOnCancel(t *testing.T) {
	limiter := endpoint.RateLimiterFuncs{
		WaitFn: func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Hour):
				return nil
			}
		},
	}
	ep := endpoint.DelayRateLimitMiddleware(limiter)(
		func(context.Context, any) (any, error) { return "ok", nil },
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := ep(ctx, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait should honor cancellation, got %v", err)
	}
}

func TestBuilder_Describe(t *testing.T) {
	b := endpoint.NewBuilder(endpoint.Nop).
		WithValidation().
		UseNamed("auth", func(next endpoint.Endpoint) endpoint.Endpoint { return next }).
		Use(func(next endpoint.Endpoint) endpoint.Endpoint { return next }).
		WithTimeout(5)

	got := b.Describe()
	want := []string{"validation", "auth", "?", "timeout"}
	if len(got) != len(want) {
		t.Fatalf("labels: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("label %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBulkheadMiddleware_ReportsTheCallerContextThatEndedTheWait(t *testing.T) {
	release := make(chan struct{})
	occupied := make(chan struct{})
	base := func(ctx context.Context, request any) (any, error) {
		close(occupied)
		<-release
		return "ok", nil
	}
	ep := endpoint.NewBuilder(base).WithBulkhead(1, nil).Build()

	go func() { _, _ = ep(context.Background(), "first") }()
	<-occupied

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := ep(ctx, "second")
	close(release)

	// The saturation stays visible, but the failure is classified as the
	// caller's timeout so a transport reports 504 rather than 503.
	if !errors.Is(err, endpoint.ErrBulkheadFull) {
		t.Fatalf("error = %v, want it to wrap ErrBulkheadFull", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want it to wrap context.DeadlineExceeded", err)
	}
	var kinder apperror.Kinder
	if !errors.As(err, &kinder) {
		t.Fatalf("error %v does not classify itself", err)
	}
	if got := kinder.ErrorKind(); got != apperror.KindDeadlineExceeded {
		t.Fatalf("kind = %q, want %q", got, apperror.KindDeadlineExceeded)
	}
}

func TestBulkheadMiddleware_ClassifiesCancellationAsCanceled(t *testing.T) {
	release := make(chan struct{})
	occupied := make(chan struct{})
	base := func(ctx context.Context, request any) (any, error) {
		close(occupied)
		<-release
		return "ok", nil
	}
	ep := endpoint.NewBuilder(base).WithBulkhead(1, nil).Build()

	go func() { _, _ = ep(context.Background(), "first") }()
	<-occupied

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ep(ctx, "second")
	close(release)

	var kinder apperror.Kinder
	if !errors.As(err, &kinder) {
		t.Fatalf("error %v does not classify itself", err)
	}
	if got := kinder.ErrorKind(); got != apperror.KindCanceled {
		t.Fatalf("kind = %q, want %q", got, apperror.KindCanceled)
	}
}

func TestFallback_AnswersWhenPrimaryFails(t *testing.T) {
	primary := func(context.Context, any) (any, error) { return nil, errors.New("dependency down") }
	ep := endpoint.NewBuilder(primary).
		WithFallback(func(context.Context, any) (any, error) { return "cached", nil }).
		Build()

	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("fallback should absorb the failure: %v", err)
	}
	if resp != "cached" {
		t.Errorf("response: got %v, want cached", resp)
	}
}

func TestFallback_SkippedWhenContextIsDone(t *testing.T) {
	primaryErr := errors.New("dependency down")
	fallbackCalls := 0
	ep := endpoint.NewBuilder(
		func(context.Context, any) (any, error) { return nil, primaryErr },
	).WithFallback(func(context.Context, any) (any, error) {
		fallbackCalls++
		return "cached", nil
	}).Build()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ep(ctx, nil)
	if !errors.Is(err, primaryErr) {
		t.Fatalf("cancelled caller should see the primary error, got %v", err)
	}
	if fallbackCalls != 0 {
		t.Errorf("fallback calls: got %d, want 0", fallbackCalls)
	}
}

func TestFallback_JoinsBothErrorsWhenFallbackFails(t *testing.T) {
	primaryErr := errors.New("dependency down")
	fallbackErr := errors.New("cache miss")
	ep := endpoint.NewBuilder(
		func(context.Context, any) (any, error) { return nil, primaryErr },
	).WithFallback(func(context.Context, any) (any, error) {
		return nil, fallbackErr
	}).Build()

	_, err := ep(context.Background(), nil)
	if !errors.Is(err, primaryErr) {
		t.Errorf("primary cause must stay diagnosable, got %v", err)
	}
	if !errors.Is(err, fallbackErr) {
		t.Errorf("fallback cause must stay diagnosable, got %v", err)
	}
}

func TestMiddlewareConstructorsRejectNilDependencies(t *testing.T) {
	cases := map[string]func(){
		"metrics":          func() { endpoint.MetricsMiddleware(nil) },
		"rate limit":       func() { endpoint.RateLimitMiddleware(nil) },
		"delay rate limit": func() { endpoint.DelayRateLimitMiddleware(nil) },
		"fallback":         func() { endpoint.Fallback(nil) },
		"in-flight":        func() { endpoint.InFlightMiddleware(1, nil) },
	}
	for name, construct := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("assembling with a nil dependency must panic")
				}
			}()
			construct()
		})
	}
}
