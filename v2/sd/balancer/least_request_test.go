package balancer_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/balancer"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/feedback"
	"github.com/dreamsxin/go-kit/v2/sd/instance"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

// leastRequest is the assembly these tests cover: the endpoint layer knows
// nothing about feedback, so least request is composed rather than constructed.
// A private table is fine here because each test outlives it.
func leastRequest(source endpointer.InstanceEndpointer, options ...selector.LeastRequestOption) sd.Balancer {
	return balancer.New(source, feedback.NewTable().LeastRequest(options...))
}

func TestLeastRequest_NoEndpoints(t *testing.T) {
	lb := leastRequest(newEndpointer(t))
	if _, err := lb.Pick(context.Background(), nil); !errors.Is(err, sd.ErrNoEndpoints) {
		t.Fatalf("Endpoint() error = %v, want ErrNoEndpoints", err)
	}
}

func TestLeastRequest_SingleEndpoint(t *testing.T) {
	lb := leastRequest(newEndpointer(t, "only:80"))

	for i := 0; i < 5; i++ {
		if address := selectAddress(t, lb); address != "only:80" {
			t.Fatalf("selected %s, want only:80", address)
		}
	}
}

func TestLeastRequest_PropagatesSourceError(t *testing.T) {
	source := newEndpointer(t, "A:80")
	if err := source.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lb := leastRequest(source)
	if _, err := lb.Pick(context.Background(), nil); err == nil {
		t.Fatal("expected the closed endpointer error to propagate")
	}
}

// Every completed call must release its counter. If the decrement were missing,
// the first instance picked would look permanently busy and all later traffic
// would pile onto the other one, which this balance check would catch.
func TestLeastRequest_ReleasesCounterWhenCallReturns(t *testing.T) {
	lb := leastRequest(newEndpointer(t, "A:80", "B:80"))

	const calls = 200
	counts := map[string]int{}
	for i := 0; i < calls; i++ {
		counts[selectAddress(t, lb)]++
	}
	for _, address := range []string{"A:80", "B:80"} {
		if counts[address] < calls/4 {
			t.Fatalf("instance %s served %d of %d calls: %v", address, counts[address], calls, counts)
		}
	}
}

// A caller that invokes the returned endpoint twice must not drive the counter
// negative, which would pin every later selection onto that instance.
func TestLeastRequest_DoubleInvocationDoesNotCorruptCounters(t *testing.T) {
	lb := leastRequest(newEndpointer(t, "A:80", "B:80"))

	selected, err := lb.Pick(context.Background(), nil)
	if err != nil {
		t.Fatalf("Endpoint: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := selected.Endpoint(context.Background(), nil); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		selected.Done(sd.Outcome{})
	}

	const calls = 200
	counts := map[string]int{}
	for i := 0; i < calls; i++ {
		counts[selectAddress(t, lb)]++
	}
	for _, address := range []string{"A:80", "B:80"} {
		if counts[address] < calls/4 {
			t.Fatalf("instance %s served %d of %d calls: %v", address, counts[address], calls, counts)
		}
	}
}

func TestLeastRequest_ReachesEveryEndpoint(t *testing.T) {
	lb := leastRequest(newEndpointer(t, "A:80", "B:80", "C:80"))

	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		seen[selectAddress(t, lb)] = true
	}
	if len(seen) != 3 {
		t.Fatalf("expected all 3 endpoints to be selected, got %v", seen)
	}
}

// The point of the strategy: an instance holding calls open should receive less
// new traffic than an idle one, without the registry publishing any load.
func TestLeastRequest_AvoidsInstancesWithCallsInFlight(t *testing.T) {
	release := make(chan struct{})
	var mtx sync.Mutex
	served := map[string]int{}

	factory := endpointer.Factory(func(inst sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		address := inst.Address
		ep := endpoint.Endpoint(func(ctx context.Context, _ any) (any, error) {
			mtx.Lock()
			served[address]++
			mtx.Unlock()
			if address == "slow:80" {
				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return address, nil
		})
		return ep, io.NopCloser(nil), nil
	})

	cache := instance.NewCache()
	set := endpointer.NewEndpointer(cache, factory, nopLogger)
	t.Cleanup(func() { _ = set.Close() })
	cache.Update(sd.Event{Instances: sd.Addresses("slow:80", "fast:80")})
	time.Sleep(20 * time.Millisecond)

	lb := leastRequest(set)

	const calls = 60
	var wg sync.WaitGroup
	for i := 0; i < calls; i++ {
		selected, err := lb.Pick(context.Background(), nil)
		if err != nil {
			t.Fatalf("Endpoint %d: %v", i, err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, callErr := selected.Endpoint(context.Background(), nil)
			selected.Done(sd.Outcome{Err: callErr})
		}()
		// Give the in-flight count a moment to be visible to the next pick.
		time.Sleep(time.Millisecond)
	}

	close(release)
	wg.Wait()

	mtx.Lock()
	defer mtx.Unlock()
	if served["slow:80"] >= served["fast:80"] {
		t.Fatalf("slow instance served %d calls and fast served %d; expected the idle one to win",
			served["slow:80"], served["fast:80"])
	}
}

func TestLeastRequest_WithChoices(t *testing.T) {
	tests := []struct {
		name    string
		choices int
	}{
		{name: "below two falls back to the default", choices: 1},
		{name: "zero falls back to the default", choices: 0},
		{name: "explicit default", choices: selector.DefaultChoices},
		{name: "scans every instance", choices: 3},
		{name: "more choices than instances", choices: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lb := leastRequest(newEndpointer(t, "A:80", "B:80", "C:80"),
				selector.WithChoices(tt.choices))

			known := map[string]bool{"A:80": true, "B:80": true, "C:80": true}
			for i := 0; i < 60; i++ {
				if address := selectAddress(t, lb); !known[address] {
					t.Fatalf("selected unknown endpoint %s", address)
				}
			}
		})
	}
}

// The table the strategy reads is the table it writes, and the caller holds it,
// which is what lets one table serve scoring, ejection, and slow start too.
func TestLeastRequest_UsesTheCallersFeedbackTable(t *testing.T) {
	table := feedback.NewTable()
	lb := balancer.New(newEndpointer(t, "A:80"), table.LeastRequest())
	picked, err := lb.Pick(context.Background(), nil)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if got := table.Stats(picked.Instance).InFlight; got != 1 {
		t.Fatalf("in-flight = %d, want 1", got)
	}
	picked.Done(sd.Outcome{Bytes: 42})
	if got := table.Stats(picked.Instance); got.InFlight != 0 || got.Bytes != 42 {
		t.Fatalf("feedback stats = %+v, want completed call", got)
	}
}

// Counters are keyed by address, so a changed instance set must not leave the
// balancer selecting against stale entries.
func TestLeastRequest_SurvivesInstanceSetChanges(t *testing.T) {
	cache := instance.NewCache()
	set := endpointer.NewEndpointer(cache, endpointer.Factory(echoFactory), nopLogger)
	t.Cleanup(func() { _ = set.Close() })
	cache.Update(sd.Event{Instances: sd.Addresses("A:80", "B:80")})
	time.Sleep(20 * time.Millisecond)

	lb := leastRequest(set)
	for i := 0; i < 20; i++ {
		selectAddress(t, lb)
	}

	cache.Update(sd.Event{Instances: sd.Addresses("C:80")})
	time.Sleep(20 * time.Millisecond)

	for i := 0; i < 20; i++ {
		if address := selectAddress(t, lb); address != "C:80" {
			t.Fatalf("selected %s after the set changed, want C:80", address)
		}
	}
}
