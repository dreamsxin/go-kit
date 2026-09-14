package endpoint_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

func TestNilClockIsTheWallClock(t *testing.T) {
	// A retry configured with a nil clock must behave exactly as it did before
	// the seam existed: it waits on real time and still completes.
	ep := endpoint.RetryMiddleware(2,
		endpoint.WithRetryClock(nil),
		endpoint.WithRetryBackoff(func(int) time.Duration { return time.Millisecond }),
		endpoint.WithRetryable(func(error) bool { return true }),
	)(failTimes(1))

	if _, err := ep(context.Background(), nil); err != nil {
		t.Fatalf("call: %v", err)
	}
}

func TestSystemClockReportsRealTime(t *testing.T) {
	clock := endpoint.SystemClock()
	before := time.Now()
	got := clock.Now()
	if got.Before(before.Add(-time.Second)) || got.After(time.Now().Add(time.Second)) {
		t.Fatalf("Now = %v, want a time near %v", got, before)
	}

	timer := clock.NewTimer(time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C():
	case <-time.After(2 * time.Second):
		t.Fatal("system timer never fired")
	}
}

func TestSystemClockTimerWithANonPositiveDurationIsDueImmediately(t *testing.T) {
	timer := endpoint.SystemClock().NewTimer(0)
	defer timer.Stop()
	select {
	case <-timer.C():
	case <-time.After(2 * time.Second):
		t.Fatal("a non-positive duration should be due immediately")
	}
}

func TestManualClockDoesNotMoveOnItsOwn(t *testing.T) {
	at := time.Unix(1000, 0)
	clock := endpoint.NewManualClock(at)
	if got := clock.Now(); !got.Equal(at) {
		t.Fatalf("Now = %v, want %v", got, at)
	}
	clock.Advance(90 * time.Second)
	if got := clock.Now(); !got.Equal(at.Add(90 * time.Second)) {
		t.Fatalf("Now after Advance = %v", got)
	}
	clock.Set(at)
	if got := clock.Now(); !got.Equal(at) {
		t.Fatalf("Now after Set = %v, want %v", got, at)
	}
}

func TestManualClockAdvanceFiresDueTimers(t *testing.T) {
	clock := endpoint.NewManualClock(time.Unix(0, 0))
	early := clock.NewTimer(time.Second)
	late := clock.NewTimer(time.Minute)
	if got := clock.Pending(); got != 2 {
		t.Fatalf("Pending = %d, want 2", got)
	}

	clock.Advance(time.Second)
	select {
	case <-early.C():
	default:
		t.Fatal("the one-second timer should be due")
	}
	select {
	case <-late.C():
		t.Fatal("the one-minute timer should not be due yet")
	default:
	}
	if got := clock.Pending(); got != 1 {
		t.Fatalf("Pending = %d, want 1", got)
	}

	clock.Advance(time.Minute)
	select {
	case <-late.C():
	default:
		t.Fatal("the one-minute timer should be due")
	}
}

func TestManualClockTimerWithANonPositiveDurationIsDueWithoutAdvancing(t *testing.T) {
	clock := endpoint.NewManualClock(time.Unix(0, 0))
	timer := clock.NewTimer(0)
	select {
	case <-timer.C():
	default:
		t.Fatal("a non-positive duration should be due immediately")
	}
	if got := clock.Pending(); got != 0 {
		t.Fatalf("Pending = %d, want 0", got)
	}
}

func TestManualClockStopReportsWhetherTheWaitWasPending(t *testing.T) {
	clock := endpoint.NewManualClock(time.Unix(0, 0))
	timer := clock.NewTimer(time.Second)
	if !timer.Stop() {
		t.Fatal("Stop on a pending timer should report true")
	}
	if timer.Stop() {
		t.Fatal("Stop on an already-stopped timer should report false")
	}
	clock.Advance(time.Hour)
	select {
	case <-timer.C():
		t.Fatal("a stopped timer must not fire")
	default:
	}
}

func TestManualClockStopReleasesPendingTimersWithoutAdvancing(t *testing.T) {
	clock := endpoint.NewManualClock(time.Unix(0, 0))
	live := clock.NewTimer(time.Minute)
	for range 1000 {
		timer := clock.NewTimer(time.Hour)
		if !timer.Stop() || timer.Stop() {
			t.Fatal("Stop must succeed exactly once for a pending timer")
		}
	}
	if got := clock.Pending(); got != 1 {
		t.Fatalf("Pending = %d after stopping timers without advancing, want only the live timer", got)
	}
	clock.Advance(time.Minute)
	select {
	case <-live.C():
	default:
		t.Fatal("stopping other timers removed the live timer")
	}
	if live.Stop() || clock.Pending() != 0 {
		t.Fatal("a fired timer must no longer be pending")
	}
}

func TestManualClockConcurrentStopAndAdvance(t *testing.T) {
	for range 100 {
		clock := endpoint.NewManualClock(time.Unix(0, 0))
		timer := clock.NewTimer(time.Second)
		start := make(chan struct{})
		var wg sync.WaitGroup
		var stopped bool
		wg.Add(2)
		go func() { defer wg.Done(); <-start; stopped = timer.Stop() }()
		go func() { defer wg.Done(); <-start; clock.Advance(time.Second) }()
		close(start)
		wg.Wait()
		if got := clock.Pending(); got != 0 {
			t.Fatalf("Pending = %d after Stop and Advance, want 0", got)
		}
		fired := false
		select {
		case <-timer.C():
			fired = true
		default:
		}
		if fired == stopped {
			t.Fatalf("fired=%v stopped=%v, exactly one must win", fired, stopped)
		}
		if timer.Stop() {
			t.Fatal("completed timer was stopped a second time")
		}
	}
}

func TestRetryCancellationReleasesManualClockWait(t *testing.T) {
	clock := endpoint.NewManualClock(time.Unix(0, 0))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	failure := errors.New("transient")
	call := endpoint.RetryMiddleware(2, endpoint.WithRetryClock(clock),
		endpoint.WithRetryBackoff(func(int) time.Duration { return time.Hour }),
		endpoint.WithRetryable(func(error) bool { return true }),
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

func TestManualClockAdvanceWakesAWaitingGoroutine(t *testing.T) {
	clock := endpoint.NewManualClock(time.Unix(0, 0))
	woke := make(chan struct{})
	timer := clock.NewTimer(time.Hour)
	go func() {
		<-timer.C()
		close(woke)
	}()

	clock.Advance(time.Hour)
	select {
	case <-woke:
	case <-time.After(2 * time.Second):
		t.Fatal("advancing the clock did not wake the waiting goroutine")
	}
}

func TestRetryMiddlewareWaitsOnTheConfiguredClock(t *testing.T) {
	clock := endpoint.NewManualClock(time.Unix(0, 0))
	ep := endpoint.RetryMiddleware(2,
		endpoint.WithRetryClock(clock),
		// An hour of backoff: if the middleware used the wall clock, this test
		// would not finish.
		endpoint.WithRetryBackoff(func(int) time.Duration { return time.Hour }),
		endpoint.WithRetryable(func(error) bool { return true }),
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

func TestMetricsObserveUsesTheConfiguredClock(t *testing.T) {
	at := time.Unix(1700000000, 0)
	clock := endpoint.NewManualClock(at)
	metrics := endpoint.Metrics{Clock: clock}

	metrics.Observe(context.Background(), endpoint.Observation{
		Operation: "GET /users",
		Duration:  250 * time.Millisecond,
	})

	snapshot := metrics.Snapshot()
	if !snapshot.LastRequestTime.Equal(at) {
		t.Fatalf("LastRequestTime = %v, want %v", snapshot.LastRequestTime, at)
	}
	// The duration is measured, not decided: the clock never sees it.
	if snapshot.TotalDuration != 250*time.Millisecond {
		t.Fatalf("TotalDuration = %v, want 250ms", snapshot.TotalDuration)
	}

	clock.Advance(time.Minute)
	metrics.Observe(context.Background(), endpoint.Observation{Operation: "GET /users"})
	if got := metrics.Snapshot().LastRequestTime; !got.Equal(at.Add(time.Minute)) {
		t.Fatalf("LastRequestTime after Advance = %v", got)
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
