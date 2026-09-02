package retry_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/balancer"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/instance"
	"github.com/dreamsxin/go-kit/v2/sd/retry"
	"log/slog"
)

var nopLogger = slog.New(slog.DiscardHandler)

type permanentError struct {
	error
}

func (permanentError) Retryable() bool { return false }

type transientError struct {
	error
}

func (transientError) Retryable() bool { return true }

func newBalancer(t *testing.T, factory endpointer.Factory) sd.Balancer {
	t.Helper()
	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: sd.Addresses("svc:80")})
	time.Sleep(20 * time.Millisecond)
	ep := endpointer.NewEndpointer(cache, factory, nopLogger)
	t.Cleanup(func() { _ = ep.Close() })
	return balancer.NewRoundRobin(ep)
}

// ── Retry ─────────────────────────────────────────────────────────────────────

func TestRetry_SucceedsOnFirstAttempt(t *testing.T) {
	f := endpointer.Factory(func(_ sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) { return "ok", nil })
		return ep, io.NopCloser(nil), nil
	})
	lb := newBalancer(t, f)
	ep := retry.Retry(3, time.Second, lb)

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
	f := endpointer.Factory(func(_ sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			attempts++
			if attempts < 3 {
				return nil, transientError{fmt.Errorf("attempt %d failed", attempts)}
			}
			return "success", nil
		})
		return ep, io.NopCloser(nil), nil
	})
	lb := newBalancer(t, f)
	ep := retry.Retry(5, time.Second, lb)

	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "success" {
		t.Errorf("got %v, want success", resp)
	}
}

func TestRetry_ExceedsMax(t *testing.T) {
	f := endpointer.Factory(func(_ sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			return nil, transientError{errors.New("always fails")}
		})
		return ep, io.NopCloser(nil), nil
	})
	lb := newBalancer(t, f)
	ep := retry.Retry(3, time.Second, lb)

	_, err := ep(context.Background(), nil)
	if err == nil {
		t.Error("expected error after max retries")
	}
}

func TestRetry_DoesNotRetryNonRetryableError(t *testing.T) {
	attempts := 0
	f := endpointer.Factory(func(_ sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			attempts++
			return nil, permanentError{errors.New("validation failed")}
		})
		return ep, io.NopCloser(nil), nil
	})
	lb := newBalancer(t, f)
	ep := retry.Retry(5, time.Second, lb)

	_, err := ep(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestRetry_ContextCancelled(t *testing.T) {
	f := endpointer.Factory(func(_ sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(ctx context.Context, _ any) (any, error) {
			time.Sleep(50 * time.Millisecond)
			return nil, transientError{errors.New("slow fail")}
		})
		return ep, io.NopCloser(nil), nil
	})
	lb := newBalancer(t, f)
	ep := retry.Retry(10, time.Second, lb)

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	_, err := ep(ctx, nil)
	if err == nil {
		t.Error("expected error from context cancellation")
	}
}

func TestRetry_BackoffStopsOnContextCancel(t *testing.T) {
	f := endpointer.Factory(func(_ sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			return nil, transientError{errors.New("transient")}
		})
		return ep, io.NopCloser(nil), nil
	})
	lb := newBalancer(t, f)

	var cancel context.CancelFunc
	ep := retry.WithCallback(time.Second, lb, func(n int, _ error) (bool, error) {
		if n == 1 {
			cancel()
		}
		return true, nil
	})

	ctx, cancelFn := context.WithCancel(context.Background())
	cancel = cancelFn
	start := time.Now()
	_, err := ep(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("retry returned after %v, want prompt cancellation", elapsed)
	}
}

func TestDefaultClassifierUnknownErrorIsPermanent(t *testing.T) {
	if retry.DefaultClassifier(errors.New("business failure")) {
		t.Fatal("unknown errors should not be retryable")
	}
}

func TestDefaultClassifierKnownTransientErrors(t *testing.T) {
	for _, err := range []error{
		transientError{errors.New("temporary")},
		sd.ErrNoEndpoints,
	} {
		if !retry.DefaultClassifier(err) {
			t.Fatalf("%v should be retryable", err)
		}
	}
}

// ── RetryWithCallback ─────────────────────────────────────────────────────────

func TestRetryWithCallback_StopsOnFalse(t *testing.T) {
	calls := 0
	f := endpointer.Factory(func(_ sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			calls++
			return nil, errors.New("fail")
		})
		return ep, io.NopCloser(nil), nil
	})
	lb := newBalancer(t, f)
	ep := retry.WithCallback(time.Second, lb,
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
	f := endpointer.Factory(func(_ sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			return nil, errors.New("original")
		})
		return ep, io.NopCloser(nil), nil
	})
	lb := newBalancer(t, f)
	ep := retry.WithCallback(time.Second, lb,
		func(n int, _ error) (bool, error) {
			return false, replacement
		},
	)

	_, err := ep(context.Background(), nil)
	var retryErr retry.Error
	if errors.As(err, &retryErr) {
		if !errors.Is(retryErr.Final, replacement) {
			t.Errorf("Final: got %v, want replacement", retryErr.Final)
		}
	} else {
		t.Errorf("expected RetryError, got %T: %v", err, err)
	}
}

// ── Request propagation ──────────────────────────────────────────────────────

// keyedBalancer records the request handed to Pick. Selection happens
// on a goroutine inside retry, so the request travels back over a channel.
type keyedBalancer struct {
	requests chan any
}

func (b *keyedBalancer) Pick(_ context.Context, request any) (sd.Picked, error) {
	b.requests <- request
	return sd.Picked{Endpoint: func(_ context.Context, _ any) (any, error) { return "ok", nil }, Done: func(sd.Outcome) {}}, nil
}
func (*keyedBalancer) Close() error { return nil }

func TestRetry_PassesRequestToBalancer(t *testing.T) {
	lb := &keyedBalancer{requests: make(chan any, 1)}
	ep := retry.Retry(1, time.Second, lb)

	resp, err := ep(context.Background(), "tenant-9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("got %v, want ok", resp)
	}
	select {
	case request := <-lb.requests:
		if request != "tenant-9" {
			t.Fatalf("balancer received %v, want tenant-9", request)
		}
	default:
		t.Fatal("Pick was never called")
	}
}

// A plain balancer receives the request through the same common contract.
type plainBalancer struct {
	calls chan struct{}
}

func (b *plainBalancer) Pick(_ context.Context, _ any) (sd.Picked, error) {
	b.calls <- struct{}{}
	return sd.Picked{Endpoint: func(_ context.Context, _ any) (any, error) { return "ok", nil }, Done: func(sd.Outcome) {}}, nil
}
func (*plainBalancer) Close() error { return nil }

func TestRetry_PassesRequestToPlainBalancer(t *testing.T) {
	lb := &plainBalancer{calls: make(chan struct{}, 1)}
	ep := retry.Retry(1, time.Second, lb)

	if _, err := ep(context.Background(), "tenant-9"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	select {
	case <-lb.calls:
	default:
		t.Fatal("Endpoint was never called")
	}
}

// ── RetryError ────────────────────────────────────────────────────────────────

func TestRetryError_ErrorString_Single(t *testing.T) {
	e := retry.Error{
		Attempts: []retry.Attempt{{Err: errors.New("only error")}},
	}
	if e.Error() != "only error" {
		t.Errorf("Error(): got %q, want %q", e.Error(), "only error")
	}
}

func TestRetryError_ErrorString_Multiple(t *testing.T) {
	e := retry.Error{
		Attempts: []retry.Attempt{{Err: errors.New("first")}, {Err: errors.New("second")}},
	}
	got := e.Error()
	if got != "second (previously: first)" {
		t.Errorf("Error(): got %q", got)
	}
}

// A retry history is only actionable if it says which instance produced each
// failure, so the addresses recorded on the attempts must reach the message.
func TestRetryError_ErrorStringAttributesFailuresToInstances(t *testing.T) {
	e := retry.Error{
		Attempts: []retry.Attempt{
			{Address: "10.0.0.1:80", Err: errors.New("connection refused")},
			{Address: "10.0.0.2:80", Err: errors.New("timeout")},
		},
	}
	want := "10.0.0.2:80: timeout (previously: 10.0.0.1:80: connection refused)"
	if got := e.Error(); got != want {
		t.Errorf("Error():\n got %q\nwant %q", got, want)
	}
}

// A replaced final error keeps the address of the attempt it replaces.
func TestRetryError_FinalErrorKeepsTheLastAddress(t *testing.T) {
	e := retry.Error{
		Attempts: []retry.Attempt{{Address: "10.0.0.9:80", Err: errors.New("timeout")}},
		Final:    errors.New("gave up"),
	}
	if got := e.Error(); got != "10.0.0.9:80: gave up" {
		t.Errorf("Error(): got %q", got)
	}
}

// Selection can fail before any instance is known; the message must not invent
// an attribution for it.
func TestRetryError_ErrorStringOmitsUnknownAddresses(t *testing.T) {
	e := retry.Error{Attempts: []retry.Attempt{{Err: errors.New("no endpoints available")}}}
	if got := e.Error(); got != "no endpoints available" {
		t.Errorf("Error(): got %q", got)
	}
}
