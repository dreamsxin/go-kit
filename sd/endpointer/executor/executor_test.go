package executor_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/endpoint"
	kitlog "github.com/dreamsxin/go-kit/log"
	"github.com/dreamsxin/go-kit/sd/endpointer"
	"github.com/dreamsxin/go-kit/sd/endpointer/balancer"
	"github.com/dreamsxin/go-kit/sd/endpointer/executor"
	"github.com/dreamsxin/go-kit/sd/events"
	"github.com/dreamsxin/go-kit/sd/instance"
	"github.com/dreamsxin/go-kit/sd/interfaces"
)

var nopLogger = kitlog.NewNopLogger()

func newBalancer(t *testing.T, factory endpoint.Factory) interfaces.Balancer {
	t.Helper()
	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"svc:80"}})
	time.Sleep(20 * time.Millisecond)
	ep := endpointer.NewEndpointer(cache, factory, nopLogger)
	return balancer.NewRoundRobin(ep)
}

// ── Retry ─────────────────────────────────────────────────────────────────────

func TestRetry_SucceedsOnFirstAttempt(t *testing.T) {
	f := endpoint.Factory(func(_ string) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) { return "ok", nil })
		return ep, io.NopCloser(nil), nil
	})
	lb := newBalancer(t, f)
	ep := executor.Retry(3, time.Second, lb)

	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "ok" {
		t.Errorf("got %v, want ok", resp)
	}
}

func TestRetry_SucceedsAfterFailures(t *testing.T) {
	attempts := 0
	f := endpoint.Factory(func(_ string) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			attempts++
			if attempts < 3 {
				return nil, fmt.Errorf("attempt %d failed", attempts)
			}
			return "success", nil
		})
		return ep, io.NopCloser(nil), nil
	})
	lb := newBalancer(t, f)
	ep := executor.Retry(5, time.Second, lb)

	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "success" {
		t.Errorf("got %v, want success", resp)
	}
}

func TestRetry_ExceedsMax(t *testing.T) {
	f := endpoint.Factory(func(_ string) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			return nil, errors.New("always fails")
		})
		return ep, io.NopCloser(nil), nil
	})
	lb := newBalancer(t, f)
	ep := executor.Retry(3, time.Second, lb)

	_, err := ep(context.Background(), nil)
	if err == nil {
		t.Error("expected error after max retries")
	}
}

func TestRetry_ContextCancelled(t *testing.T) {
	f := endpoint.Factory(func(_ string) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(ctx context.Context, _ any) (any, error) {
			time.Sleep(50 * time.Millisecond)
			return nil, errors.New("slow fail")
		})
		return ep, io.NopCloser(nil), nil
	})
	lb := newBalancer(t, f)
	ep := executor.Retry(10, time.Second, lb)

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	_, err := ep(ctx, nil)
	if err == nil {
		t.Error("expected error from context cancellation")
	}
}

// ── RetryAlways ───────────────────────────────────────────────────────────────

func TestRetryAlways_StopsOnContextCancel(t *testing.T) {
	f := endpoint.Factory(func(_ string) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			return nil, errors.New("fail")
		})
		return ep, io.NopCloser(nil), nil
	})
	lb := newBalancer(t, f)
	ep := executor.RetryAlways(time.Second, lb)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := ep(ctx, nil)
	if err == nil {
		t.Error("expected error when context expires")
	}
}

// ── RetryWithCallback ─────────────────────────────────────────────────────────

func TestRetryWithCallback_StopsOnFalse(t *testing.T) {
	calls := 0
	f := endpoint.Factory(func(_ string) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			calls++
			return nil, errors.New("fail")
		})
		return ep, io.NopCloser(nil), nil
	})
	lb := newBalancer(t, f)
	ep := executor.RetryWithCallback(time.Second, lb,
		func(n int, _ error) (bool, error) {
			return n < 2, nil // retry only once
		},
	)

	_, err := ep(context.Background(), nil)
	if err == nil {
		t.Error("expected error")
	}
	if calls > 2 {
		t.Errorf("expected at most 2 calls, got %d", calls)
	}
}

func TestRetryWithCallback_ReplacesError(t *testing.T) {
	replacement := errors.New("replaced")
	f := endpoint.Factory(func(_ string) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			return nil, errors.New("original")
		})
		return ep, io.NopCloser(nil), nil
	})
	lb := newBalancer(t, f)
	ep := executor.RetryWithCallback(time.Second, lb,
		func(n int, _ error) (bool, error) {
			return false, replacement
		},
	)

	_, err := ep(context.Background(), nil)
	var retryErr executor.RetryError
	if errors.As(err, &retryErr) {
		if !errors.Is(retryErr.Final, replacement) {
			t.Errorf("Final: got %v, want replacement", retryErr.Final)
		}
	} else {
		t.Errorf("expected RetryError, got %T: %v", err, err)
	}
}

// ── RetryError ────────────────────────────────────────────────────────────────

func TestRetryError_ErrorString_Single(t *testing.T) {
	e := executor.RetryError{
		RawErrors: []error{errors.New("only error")},
	}
	if e.Error() != "only error" {
		t.Errorf("Error(): got %q, want %q", e.Error(), "only error")
	}
}

func TestRetryError_ErrorString_Multiple(t *testing.T) {
	e := executor.RetryError{
		RawErrors: []error{errors.New("first"), errors.New("second")},
	}
	got := e.Error()
	if got == "" {
		t.Error("Error() should not be empty")
	}
	// should contain "previously"
	if len(got) < 10 {
		t.Errorf("Error() too short: %q", got)
	}
}
