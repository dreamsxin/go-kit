package resilience_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/endpoint/resilience"
)

func TestNilClockIsTheWallClock(t *testing.T) {
	// A retry configured with a nil clock must behave exactly as it did before
	// the seam existed: it waits on real time and still completes.
	ep := resilience.RetryMiddleware(2,
		resilience.WithRetryClock(nil),
		resilience.WithRetryBackoff(func(int) time.Duration { return time.Millisecond }),
		resilience.WithRetryable(func(error) bool { return true }),
	)(failTimes(1))

	if _, err := ep(context.Background(), nil); err != nil {
		t.Fatalf("call: %v", err)
	}
}

func TestRetryCancellationReleasesManualClockWait(t *testing.T) {
	clock := endpoint.NewManualClock(time.Unix(0, 0))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	failure := errors.New("transient")
	call := resilience.RetryMiddleware(2, resilience.WithRetryClock(clock),
		resilience.WithRetryBackoff(func(int) time.Duration { return time.Hour }),
		resilience.WithRetryable(func(error) bool { return true }),
	)(func(context.Context, any) (any, error) { return nil, failure })
	done := make(chan error, 1)
	go func() { _, err := call(ctx, nil); done <- err }()
	waitForPendingTimer(t, clock)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, failure) {
			t.Fatalf("error = %v, want original failure", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("retry did not stop on cancellation")
	}
	if got := clock.Pending(); got != 0 {
		t.Fatalf("cancelled retry left %d pending timers, want 0", got)
	}
}

func TestRetryMiddlewareWaitsOnTheConfiguredClock(t *testing.T) {
	clock := endpoint.NewManualClock(time.Unix(0, 0))
	ep := resilience.RetryMiddleware(2,
		resilience.WithRetryClock(clock),
		// An hour of backoff: if the middleware used the wall clock, this test
		// would not finish.
		resilience.WithRetryBackoff(func(int) time.Duration { return time.Hour }),
		resilience.WithRetryable(func(error) bool { return true }),
	)(failTimes(1))

	done := make(chan error, 1)
	go func() { _, err := ep(context.Background(), nil); done <- err }()

	waitForPendingTimer(t, clock)
	clock.Advance(time.Hour)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("call: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the retry never completed after the clock advanced")
	}
}

// failTimes returns an endpoint that fails its first failures calls and then
// succeeds, so a retry test can assert the loop ran.
func failTimes(failures int) endpoint.Endpoint {
	remaining := failures
	return func(context.Context, any) (any, error) {
		if remaining > 0 {
			remaining--
			return nil, errors.New("transient")
		}
		return "ok", nil
	}
}

func waitForPendingTimer(t *testing.T, clock *endpoint.ManualClock) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for clock.Pending() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the code under test never reached its wait")
		}
		time.Sleep(time.Millisecond)
	}
}
