package retry_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/retry"
)

type optionBalancer struct {
	choose func(context.Context, any) (sd.Picked, error)
}

func (b optionBalancer) Pick(ctx context.Context, req any) (sd.Picked, error) {
	return b.choose(ctx, req)
}
func (optionBalancer) Close() error { return nil }

// Publishing after timer registration lets a test advance without racing NewTimer.
type observedClock struct {
	*endpoint.ManualClock
	waits chan time.Duration
}

func newObservedClock() *observedClock {
	return &observedClock{endpoint.NewManualClock(time.Unix(0, 0)), make(chan time.Duration, 16)}
}

func (c *observedClock) NewTimer(delay time.Duration) endpoint.Timer {
	timer := c.ManualClock.NewTimer(delay)
	c.waits <- delay
	return timer
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for retry progress")
		var zero T
		return zero
	}
}

func TestNewRetryUsesConfiguredClockAndBackoff(t *testing.T) {
	clock := newObservedClock()
	var calls atomic.Int32
	var scheduled []int
	b := optionBalancer{choose: func(ctx context.Context, req any) (sd.Picked, error) {
		if req != "tenant" {
			return sd.Picked{}, fmt.Errorf("request lost: %v", req)
		}
		n := calls.Add(1)
		return sd.Picked{Instance: sd.Instance{Address: fmt.Sprintf("host-%d", n)},
			Endpoint: func(context.Context, any) (any, error) {
				if n < 3 {
					return nil, sd.ErrNoEndpoints
				}
				return "ok", nil
			}}, nil
	}}
	call := retry.New(b, retry.WithMaxAttempts(3), retry.WithClock(clock), retry.WithBackoff(func(attempt int) time.Duration {
		scheduled = append(scheduled, attempt)
		return time.Duration(attempt) * time.Hour
	}), retry.WithClock(nil), retry.WithBackoff(nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		response, err := call(ctx, "tenant")
		if err == nil && response != "ok" {
			err = fmt.Errorf("response lost: %v", response)
		}
		done <- err
	}()
	for attempt := 1; attempt <= 2; attempt++ {
		delay := receive(t, clock.waits)
		if want := time.Duration(attempt) * time.Hour; delay != want {
			t.Fatalf("delay=%v want=%v", delay, want)
		}
		if got := calls.Load(); got != int32(attempt) {
			t.Fatalf("calls=%d before advance, want %d", got, attempt)
		}
		clock.Advance(delay - time.Nanosecond)
		if clock.Pending() != 1 {
			t.Fatal("timer fired before the configured deadline")
		}
		clock.Advance(time.Nanosecond)
	}
	if err := receive(t, done); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(scheduled) != "[1 2]" || calls.Load() != 3 || clock.Pending() != 0 {
		t.Fatalf("schedule=%v calls=%d pending=%d", scheduled, calls.Load(), clock.Pending())
	}
}

func TestNewRetryDefaultBackoffRestartsForEachCall(t *testing.T) {
	clock := newObservedClock()
	b := optionBalancer{choose: func(context.Context, any) (sd.Picked, error) { return sd.Picked{}, sd.ErrNoEndpoints }}
	call := retry.New(b, retry.WithMaxAttempts(4), retry.WithClock(clock))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for range 2 {
		done := make(chan error, 1)
		go func() { _, err := call(ctx, nil); done <- err }()
		var previous time.Duration
		for attempt := 1; attempt <= 3; attempt++ {
			delay := receive(t, clock.waits)
			if attempt == 1 && delay != 10*time.Millisecond {
				t.Fatalf("first delay=%v, want 10ms", delay)
			}
			if attempt > 1 && (delay < previous || delay > 3*previous) {
				t.Fatalf("delay=%v outside next jitter range for %v", delay, previous)
			}
			previous = delay
			clock.Advance(delay)
		}
		var exhausted retry.Error
		if err := receive(t, done); !errors.As(err, &exhausted) || len(exhausted.Attempts) != 4 {
			t.Fatalf("error=%v, want four attempts", err)
		}
	}
}

func TestNewRetryCancellationStopsBackoff(t *testing.T) {
	clock := newObservedClock()
	var calls atomic.Int32
	b := optionBalancer{choose: func(context.Context, any) (sd.Picked, error) { calls.Add(1); return sd.Picked{}, sd.ErrNoEndpoints }}
	call := retry.New(b, retry.WithMaxAttempts(3), retry.WithClock(clock), retry.WithBackoff(func(int) time.Duration { return time.Hour }))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := call(ctx, nil); done <- err }()
	receive(t, clock.waits)
	cancel()
	var reported retry.Error
	err := receive(t, done)
	if !errors.Is(err, context.Canceled) || !errors.As(err, &reported) || len(reported.Attempts) != 1 {
		t.Fatalf("error=%v, want cancellation with one attempt", err)
	}
	if clock.Pending() != 0 || calls.Load() != 1 {
		t.Fatalf("pending=%d calls=%d", clock.Pending(), calls.Load())
	}
	clock.Advance(time.Hour)
	if calls.Load() != 1 {
		t.Fatal("cancelled call dispatched another attempt")
	}
}

func TestNewRetryWallClockBudgetBoundsManualWait(t *testing.T) {
	clock := newObservedClock()
	var calls atomic.Int32
	b := optionBalancer{choose: func(context.Context, any) (sd.Picked, error) { calls.Add(1); return sd.Picked{}, sd.ErrNoEndpoints }}
	call := retry.New(b, retry.WithMaxAttempts(3), retry.WithTimeout(100*time.Millisecond), retry.WithClock(clock),
		retry.WithBackoff(func(int) time.Duration { return time.Hour }))
	done := make(chan error, 1)
	go func() { _, err := call(context.Background(), nil); done <- err }()
	receive(t, clock.waits)
	err := receive(t, done)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v, want real deadline", err)
	}
	if clock.Pending() != 0 || calls.Load() != 1 {
		t.Fatalf("pending=%d calls=%d", clock.Pending(), calls.Load())
	}
}

func TestNewRetryOptionsKeepAttemptAndErrorPolicies(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(sd.Balancer) endpoint.Endpoint
		want  int32
	}{
		{"new defaults", func(b sd.Balancer) endpoint.Endpoint {
			return retry.New(b, nil, retry.WithClock(nil), retry.WithBackoff(nil), retry.WithAttemptCallback(nil), retry.WithErrorClassifier(nil))
		}, 1},
		{"new clamps attempts", func(b sd.Balancer) endpoint.Endpoint { return retry.New(b, retry.WithMaxAttempts(-1)) }, 1},
		{"new independent cap", func(b sd.Balancer) endpoint.Endpoint {
			return retry.New(b, retry.WithMaxAttempts(2), retry.WithAttemptCallback(func(int, error) (bool, error) { return true, nil }), retry.WithBackoff(func(int) time.Duration { return 0 }))
		}, 2},
		{"legacy clamps attempts", func(b sd.Balancer) endpoint.Endpoint { return retry.Retry(0, 0, b) }, 1},
		{"new nil clock uses wall time", func(b sd.Balancer) endpoint.Endpoint {
			return retry.New(b, retry.WithMaxAttempts(3), retry.WithClock(nil))
		}, 3},
		{"callback stops before cap", func(b sd.Balancer) endpoint.Endpoint {
			return retry.New(b, retry.WithMaxAttempts(5), retry.WithBackoff(func(int) time.Duration { return -time.Second }),
				retry.WithAttemptCallback(func(n int, _ error) (bool, error) { return n < 2, nil }))
		}, 2},
		{"legacy nil callback", func(b sd.Balancer) endpoint.Endpoint { return retry.WithCallback(0, b, nil) }, 3},
		{"legacy nil classifier", func(b sd.Balancer) endpoint.Endpoint { return retry.WithClassifier(0, b, nil, nil) }, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			b := optionBalancer{choose: func(context.Context, any) (sd.Picked, error) {
				if calls.Add(1) < 3 {
					return sd.Picked{}, sd.ErrNoEndpoints
				}
				return sd.Picked{Endpoint: endpoint.Nop}, nil
			}}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := tc.build(b)(ctx, nil)
			if got := calls.Load(); got != tc.want {
				t.Fatalf("calls=%d want=%d", got, tc.want)
			}
			if tc.want < 3 && !errors.Is(err, sd.ErrNoEndpoints) {
				t.Fatalf("error=%v", err)
			}
			if tc.want == 3 && err != nil {
				t.Fatal(err)
			}
		})
	}
	original, replacement := errors.New("wire failure"), errors.New("replacement")
	b := optionBalancer{choose: func(context.Context, any) (sd.Picked, error) { return sd.Picked{}, original }}
	classified := false
	call := retry.New(b, retry.WithMaxAttempts(3), retry.WithAttemptCallback(func(n int, err error) (bool, error) {
		if n != 1 || err != original {
			t.Errorf("callback got %d,%v", n, err)
		}
		return true, replacement
	}), retry.WithErrorClassifier(func(err error) bool { classified = err == replacement; return false }))
	_, err := call(context.Background(), nil)
	var reported retry.Error
	if !classified || !errors.As(err, &reported) || reported.Final != replacement || len(reported.Attempts) != 1 || reported.Attempts[0].Err != original {
		t.Fatalf("classification=%v error=%v", classified, err)
	}
}

func TestNewRetryKeepsCallerContextAndMeasuredLatency(t *testing.T) {
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "trace")
	clock := endpoint.NewManualClock(time.Unix(0, 0))
	outcomes := make(chan sd.Outcome, 1)
	b := optionBalancer{choose: func(ctx context.Context, request any) (sd.Picked, error) {
		if _, ok := ctx.Deadline(); ok {
			return sd.Picked{}, errors.New("default New invented a deadline")
		}
		if ctx.Value(contextKey{}) != "trace" || request != "body" {
			return sd.Picked{}, errors.New("context or request lost")
		}
		return sd.Picked{Instance: sd.Instance{Address: "a:80"}, Endpoint: func(context.Context, any) (any, error) {
			clock.Advance(24 * time.Hour)
			return nil, sd.ErrNoEndpoints
		}, Done: func(outcome sd.Outcome) { outcomes <- outcome }}, nil
	}}
	_, err := retry.New(b, retry.WithClock(clock))(ctx, "body")
	var reported retry.Error
	if !errors.As(err, &reported) || len(reported.Attempts) != 1 || reported.Attempts[0].Address != "a:80" {
		t.Fatalf("error=%v, want one attributed attempt", err)
	}
	outcome := receive(t, outcomes)
	if !errors.Is(outcome.Err, sd.ErrNoEndpoints) || outcome.Latency >= time.Hour || reported.Attempts[0].Latency >= time.Hour {
		t.Fatalf("manual clock altered measured outcome=%v attempt=%v", outcome, reported.Attempts[0])
	}
}

func TestNewRetryRejectsNilBalancer(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("nil balancer accepted")
		}
	}()
	retry.New(nil)
}

func TestNewRetrySharesClockWithoutSharingAttemptProgress(t *testing.T) {
	clock := newObservedClock()
	call := retry.New(optionBalancer{choose: func(context.Context, any) (sd.Picked, error) { return sd.Picked{}, sd.ErrNoEndpoints }},
		retry.WithMaxAttempts(2), retry.WithClock(clock), retry.WithBackoff(func(n int) time.Duration { return time.Duration(n) * time.Hour }))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 4)
	for range 4 {
		go func() { _, err := call(ctx, nil); done <- err }()
	}
	for range 4 {
		if delay := receive(t, clock.waits); delay != time.Hour {
			t.Fatalf("first delay=%v", delay)
		}
	}
	clock.Advance(time.Hour)
	for range 4 {
		var reported retry.Error
		if err := receive(t, done); !errors.As(err, &reported) || len(reported.Attempts) != 2 {
			t.Fatalf("error=%v", err)
		}
	}
	if clock.Pending() != 0 {
		t.Fatal("completed callers left pending timers")
	}
}
