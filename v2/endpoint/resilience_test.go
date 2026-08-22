package endpoint_test

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestRateLimitMiddleware_RejectsOverLimit(t *testing.T) {
	calls := 0
	limiter := endpoint.RateLimiterFunc{
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
	limiter := endpoint.RateLimiterFunc{
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
