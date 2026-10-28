package retry_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/balancer"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/instance"
	"github.com/dreamsxin/go-kit/v2/sd/retry"
)

// The hedging fixtures run two instances behind a round-robin balancer. Each
// instance's endpoint waits on its own release channel, so a test decides
// which attempt finishes and when — no sleeps, and the hedge delay runs on a
// manual clock the test advances, which makes the timing a decision rather
// than a wait.
type hedgeFixture struct {
	clock      *endpoint.ManualClock
	endpointer endpointer.InstanceEndpointer
	balancer   sd.Balancer
	release    map[string]chan hedgeResult
	entered    map[string]chan struct{}
}

type hedgeResult struct {
	value string
	err   error
}

func newHedgeFixture(t *testing.T, addresses ...string) *hedgeFixture {
	t.Helper()
	f := &hedgeFixture{
		clock:   endpoint.NewManualClock(time.Unix(0, 0)),
		release: map[string]chan hedgeResult{},
		entered: map[string]chan struct{}{},
	}
	for _, address := range addresses {
		f.release[address] = make(chan hedgeResult, 1)
		f.entered[address] = make(chan struct{})
	}
	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: sd.Addresses(addresses...)})
	time.Sleep(20 * time.Millisecond)
	f.endpointer = endpointer.NewEndpointer(cache, endpointer.Factory(
		func(inst sd.Instance) (endpoint.Endpoint, io.Closer, error) {
			address := inst.Address
			ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
				close(f.entered[address])
				result := <-f.release[address]
				return result.value, result.err
			})
			return ep, io.NopCloser(nil), nil
		}), nopLogger)
	t.Cleanup(func() { _ = f.endpointer.Close() })
	f.balancer = balancer.NewRoundRobin(f.endpointer)
	_ = f.balancer
	return f
}

func (f *hedgeFixture) hedged(maxAttempts int) endpoint.Endpoint {
	return retry.New(f.balancer,
		retry.WithMaxAttempts(maxAttempts),
		retry.WithHedging(10*time.Millisecond),
		retry.WithClock(f.clock),
		retry.WithErrorClassifier(func(error) bool { return true }),
	)
}

func waitForEntered(t *testing.T, f *hedgeFixture, address string) {
	t.Helper()
	select {
	case <-f.entered[address]:
	case <-time.After(2 * time.Second):
		t.Fatalf("attempt did not reach %s", address)
	}
}

// TestHedgeDispatchesAfterTheDelay pins sd.hedging's schedule: the original
// attempt parks, the manual clock advances past the hedge delay, and the
// hedge lands on the other instance — whose answer wins.
func TestHedgeDispatchesAfterTheDelay(t *testing.T) {
	f := newHedgeFixture(t, "a:80", "b:80")
	ep := f.hedged(3)

	done := make(chan struct{})
	var response any
	var callErr error
	go func() {
		response, callErr = ep(context.Background(), nil)
		close(done)
	}()

	waitForEntered(t, f, "a:80")
	f.clock.Advance(10 * time.Millisecond)
	waitForEntered(t, f, "b:80")
	f.release["b:80"] <- hedgeResult{value: "from-b"}

	<-done
	if callErr != nil {
		t.Fatalf("call error: %v", callErr)
	}
	if response != "from-b" {
		t.Fatalf("response = %v, want the hedge's answer", response)
	}
	// The loser drains through its buffered release channel.
	f.release["a:80"] <- hedgeResult{value: "late-a"}
}

// TestHedgeFirstSuccessWins pins the win rule: the hedge answers while the
// original is still parked; the original's late result changes nothing.
func TestHedgeFirstSuccessWins(t *testing.T) {
	f := newHedgeFixture(t, "a:80", "b:80")
	ep := f.hedged(3)

	done := make(chan struct{})
	var response any
	var callErr error
	go func() {
		response, callErr = ep(context.Background(), nil)
		close(done)
	}()

	waitForEntered(t, f, "a:80")
	f.clock.Advance(10 * time.Millisecond)
	waitForEntered(t, f, "b:80")
	f.release["b:80"] <- hedgeResult{value: "from-b"}
	<-done
	if callErr != nil || response != "from-b" {
		t.Fatalf("response=%v err=%v, want the hedge win", response, callErr)
	}
	f.release["a:80"] <- hedgeResult{value: "late-a"}
}

// TestHedgeFastFailureDispatchesAtOnce pins the gRPC rule: a failure that
// arrives at once dispatches the hedge without waiting for the delay — no
// clock advance happens in this test.
func TestHedgeFastFailureDispatchesAtOnce(t *testing.T) {
	f := newHedgeFixture(t, "a:80", "b:80")
	ep := f.hedged(3)

	done := make(chan struct{})
	go func() {
		_, _ = ep(context.Background(), nil)
		close(done)
	}()

	waitForEntered(t, f, "a:80")
	f.release["a:80"] <- hedgeResult{err: errors.New("fast fail")}
	waitForEntered(t, f, "b:80")
	f.release["b:80"] <- hedgeResult{value: "from-b"}
	<-done
}

// TestHedgingRespectsTheAttemptCap pins the fan-out bound: the total attempt
// cap (original plus one hedge) holds, and the failures are carried in the
// retry error's history with their addresses.
func TestHedgingRespectsTheAttemptCap(t *testing.T) {
	f := newHedgeFixture(t, "a:80", "b:80")
	ep := f.hedged(2)

	done := make(chan struct{})
	var callErr error
	go func() {
		_, callErr = ep(context.Background(), nil)
		close(done)
	}()

	waitForEntered(t, f, "a:80")
	f.release["a:80"] <- hedgeResult{err: errors.New("a fails")}
	waitForEntered(t, f, "b:80")
	f.release["b:80"] <- hedgeResult{err: errors.New("b fails")}
	<-done

	if callErr == nil {
		t.Fatal("all attempts failed; an error was expected")
	}
	var retryErr retry.Error
	if !errors.As(callErr, &retryErr) {
		t.Fatalf("error = %T, want retry.Error", callErr)
	}
	if len(retryErr.Attempts) != 2 {
		t.Fatalf("attempts = %d, want the capped 2", len(retryErr.Attempts))
	}
	if retryErr.Attempts[0].Address != "a:80" || retryErr.Attempts[1].Address != "b:80" {
		t.Fatalf("attempt addresses = %v/%v, want a:80 then b:80",
			retryErr.Attempts[0].Address, retryErr.Attempts[1].Address)
	}
}
