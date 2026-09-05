package feedback_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/feedback"
	"github.com/dreamsxin/go-kit/v2/sd/instance"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

var discardLogger = slog.New(slog.DiscardHandler)

func echoFactory(inst sd.Instance) (endpoint.Endpoint, io.Closer, error) {
	address := inst.Address
	ep := endpoint.Endpoint(func(context.Context, any) (any, error) { return address, nil })
	return ep, io.NopCloser(nil), nil
}

// endpointsOver returns an endpoint set and the Instancer behind it. Both are
// needed: the balancer selects over the endpoints, and the accounting follows the
// registrations.
func endpointsOver(t *testing.T, factory endpointer.Factory, addrs ...string) (endpointer.InstanceEndpointer, *instance.Cache) {
	t.Helper()
	cache := instance.NewCache()
	set := endpointer.NewEndpointer(cache, factory, discardLogger)
	t.Cleanup(func() { _ = set.Close() })
	if len(addrs) > 0 {
		cache.Update(sd.Event{Instances: sd.Addresses(addrs...)})
		waitForEndpoints(t, set, len(addrs))
	}
	return set, cache
}

func waitForEndpoints(t *testing.T, set endpointer.InstanceEndpointer, want int) {
	t.Helper()
	waitFor(t, func() bool {
		endpoints, err := set.InstanceEndpoints()
		return err == nil && len(endpoints) == want
	})
}

// measuredOver is the whole assembly under test: one call produces the balancer
// and the accounting that feeds it, already following discovery.
func measuredOver(t *testing.T, addrs ...string) (*feedback.Measured, endpointer.InstanceEndpointer) {
	t.Helper()
	set, cache := endpointsOver(t, endpointer.Factory(echoFactory), addrs...)
	measured := feedback.Measure(cache)
	t.Cleanup(func() { _ = measured.Close() })
	return measured, set
}

func pickAddress(t *testing.T, lb sd.Balancer) string {
	t.Helper()
	picked, err := lb.Pick(context.Background(), nil)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	started := time.Now()
	response, err := picked.Endpoint(context.Background(), nil)
	picked.Done(sd.Outcome{Err: err, Latency: time.Since(started)})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	return response.(string)
}

func TestMeasuredLeastRequestReportsNoEndpoints(t *testing.T) {
	measured, set := measuredOver(t)
	lb := measured.LeastRequest(set)
	if _, err := lb.Pick(context.Background(), nil); !errors.Is(err, sd.ErrNoEndpoints) {
		t.Fatalf("Pick error = %v, want ErrNoEndpoints", err)
	}
}

func TestMeasuredLeastRequestPropagatesTheSourceError(t *testing.T) {
	measured, set := measuredOver(t, "A:80")
	if err := set.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := measured.LeastRequest(set).Pick(context.Background(), nil); err == nil {
		t.Fatal("expected the closed endpointer error to propagate")
	}
}

func TestMeasuredLeastRequestReachesEveryEndpoint(t *testing.T) {
	measured, set := measuredOver(t, "A:80", "B:80", "C:80")
	lb := measured.LeastRequest(set)

	seen := map[string]bool{}
	for range 200 {
		seen[pickAddress(t, lb)] = true
	}
	if len(seen) != 3 {
		t.Fatalf("selected %v, want all three endpoints", seen)
	}
}

// Every completed call must release its counter. Without the decrement the first
// instance picked would look permanently busy and all later traffic would pile
// onto the other one, which this balance check catches.
func TestMeasuredLeastRequestReleasesTheCounterWhenACallReturns(t *testing.T) {
	measured, set := measuredOver(t, "A:80", "B:80")
	lb := measured.LeastRequest(set)

	const calls = 200
	counts := map[string]int{}
	for range calls {
		counts[pickAddress(t, lb)]++
	}
	for _, address := range []string{"A:80", "B:80"} {
		if counts[address] < calls/4 {
			t.Fatalf("instance %s served %d of %d calls: %v", address, counts[address], calls, counts)
		}
	}
}

// A caller that invokes the returned endpoint twice must not drive the counter
// negative, which would pin every later selection onto that instance.
func TestMeasuredLeastRequestSurvivesADoubleReport(t *testing.T) {
	measured, set := measuredOver(t, "A:80", "B:80")
	lb := measured.LeastRequest(set)

	picked, err := lb.Pick(context.Background(), nil)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	for i := range 5 {
		if _, err := picked.Endpoint(context.Background(), nil); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		picked.Done(sd.Outcome{})
	}

	const calls = 200
	counts := map[string]int{}
	for range calls {
		counts[pickAddress(t, lb)]++
	}
	for _, address := range []string{"A:80", "B:80"} {
		if counts[address] < calls/4 {
			t.Fatalf("instance %s served %d of %d calls: %v", address, counts[address], calls, counts)
		}
	}
}

// The point of the strategy: an instance holding calls open receives less new
// traffic than an idle one, without the registry publishing any load.
func TestMeasuredLeastRequestAvoidsInstancesWithCallsInFlight(t *testing.T) {
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

	set, cache := endpointsOver(t, factory, "slow:80", "fast:80")
	measured := feedback.Measure(cache)
	t.Cleanup(func() { _ = measured.Close() })
	lb := measured.LeastRequest(set)

	const calls = 60
	var wg sync.WaitGroup
	for i := range calls {
		picked, err := lb.Pick(context.Background(), nil)
		if err != nil {
			t.Fatalf("Pick %d: %v", i, err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, callErr := picked.Endpoint(context.Background(), nil)
			picked.Done(sd.Outcome{Err: callErr})
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

func TestMeasuredLeastRequestHonoursChoices(t *testing.T) {
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
			measured, set := measuredOver(t, "A:80", "B:80", "C:80")
			lb := measured.LeastRequest(set, selector.WithChoices(tt.choices))

			known := map[string]bool{"A:80": true, "B:80": true, "C:80": true}
			for range 60 {
				if address := pickAddress(t, lb); !known[address] {
					t.Fatalf("selected unknown endpoint %s", address)
				}
			}
		})
	}
}

// The table a measured balancer reads is the table it writes, and the caller can
// reach it — which is what lets one Measured serve scoring, ejection and slow
// start at the same time.
func TestMeasuredBalancerRecordsIntoTheTableItReads(t *testing.T) {
	measured, set := measuredOver(t, "A:80")
	lb := measured.LeastRequest(set)

	picked, err := lb.Pick(context.Background(), nil)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if got := measured.Table().Stats(picked.Instance).InFlight; got != 1 {
		t.Fatalf("in-flight = %d, want 1", got)
	}
	picked.Done(sd.Outcome{Bytes: 42})
	if got := measured.Table().Stats(picked.Instance); got.InFlight != 0 || got.Bytes != 42 {
		t.Fatalf("stats = %+v, want one completed call", got)
	}
}

// Counters are keyed by address, so a changed instance set must not leave the
// balancer selecting against stale entries.
func TestMeasuredLeastRequestSurvivesInstanceSetChanges(t *testing.T) {
	set, cache := endpointsOver(t, endpointer.Factory(echoFactory), "A:80", "B:80")
	measured := feedback.Measure(cache)
	t.Cleanup(func() { _ = measured.Close() })
	lb := measured.LeastRequest(set)

	for range 20 {
		pickAddress(t, lb)
	}

	cache.Update(sd.Event{Instances: sd.Addresses("C:80")})
	waitForEndpoints(t, set, 1)

	for range 20 {
		if address := pickAddress(t, lb); address != "C:80" {
			t.Fatalf("selected %s after the set changed, want C:80", address)
		}
	}
}

// Scored reads what this table measured, so a failing instance loses traffic
// without anything outside the process reporting it.
func TestMeasuredScoredAvoidsTheFailingInstance(t *testing.T) {
	failing := errors.New("refused")
	factory := endpointer.Factory(func(inst sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		address := inst.Address
		ep := endpoint.Endpoint(func(context.Context, any) (any, error) {
			if address == "bad:80" {
				return nil, failing
			}
			return address, nil
		})
		return ep, io.NopCloser(nil), nil
	})

	set, cache := endpointsOver(t, factory, "bad:80", "good:80")
	// Alpha of one makes each sample replace the average, so one failure is
	// enough evidence and the test does not depend on a decay rate.
	measured := feedback.Measure(cache, feedback.WithAlpha(1))
	t.Cleanup(func() { _ = measured.Close() })
	lb := measured.Scored(set)

	counts := map[string]int{}
	for range 60 {
		picked, err := lb.Pick(context.Background(), nil)
		if err != nil {
			t.Fatalf("Pick: %v", err)
		}
		_, callErr := picked.Endpoint(context.Background(), nil)
		picked.Done(sd.Outcome{Err: callErr})
		counts[picked.Instance.Address]++
	}
	if counts["good:80"] <= counts["bad:80"] {
		t.Fatalf("counts = %v, want the healthy instance to win", counts)
	}
}

// Slow start dates its ramp from the subscription, which is the reason it lives
// on Measured: an instance the accounting has never heard of looks brand new
// forever, and every weight collapses to one.
func TestMeasuredSlowStartRampsFromTheSubscription(t *testing.T) {
	set, cache := endpointsOver(t, endpointer.Factory(echoFactory), "old:80")

	// selector.SlowStart measures elapsed time against the wall clock, so the
	// table's clock is shifted rather than frozen: old:80 is dated a minute into
	// the real past and is therefore past its window, while new:80 is dated now
	// and is still at the floor of its ramp.
	clock := &shiftedClock{offset: -time.Minute}
	measured := feedback.Measure(cache, feedback.WithClock(clock.now))
	t.Cleanup(func() { _ = measured.Close() })

	const window = 10 * time.Second
	weight := selector.WeightFunc(func(sd.Instance) int { return 10 })
	lb := measured.SlowStartWeighted(set, weight, window)

	old := sd.Instance{Address: "old:80"}
	newcomer := sd.Instance{Address: "new:80"}
	waitFor(t, func() bool { return !measured.Table().Stats(old).FirstSeen.IsZero() })

	clock.set(0)
	cache.Update(sd.Event{Instances: sd.Addresses("old:80", "new:80")})
	waitForEndpoints(t, set, 2)
	waitFor(t, func() bool { return !measured.Table().Stats(newcomer).FirstSeen.IsZero() })

	counts := map[string]int{}
	for range 400 {
		counts[pickAddress(t, lb)]++
	}
	if counts["new:80"] == 0 {
		t.Fatal("the warming instance received no traffic at all; a ramp must still warm it")
	}
	if counts["old:80"] <= counts["new:80"] {
		t.Fatalf("counts = %v, want the warmed instance to carry the larger share", counts)
	}
}

// shiftedClock reports real time offset by a fixed amount, so an instance dated
// through it lands a chosen distance into the real past — which is what
// selector.SlowStart measures its ramp against. It is read from the subscription
// goroutine as well as the test's, so the offset is guarded.
type shiftedClock struct {
	mtx    sync.Mutex
	offset time.Duration
}

func (c *shiftedClock) now() time.Time {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return time.Now().Add(c.offset)
}

func (c *shiftedClock) set(offset time.Duration) {
	c.mtx.Lock()
	c.offset = offset
	c.mtx.Unlock()
}


// One subscription serves the table and the ejector, so they cannot disagree
// about which instances exist. Eject joins the subscription Measure opened, and
// is handed the snapshot that already arrived rather than waiting for the next
// one — which, for a set that never changes again, is never.
func TestEjectJoinsTheExistingSubscription(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: sd.Addresses("A:80")})

	measured := feedback.Measure(cache)
	t.Cleanup(func() { _ = measured.Close() })

	ejector := measured.Eject(feedback.EjectionPolicy{MaxErrorRate: 0.5, MinSamples: 1})
	// Retained from the snapshot that predates this Ejector.
	if ejector.Ejected(sd.Instance{Address: "A:80"}) {
		t.Fatal("a healthy instance was reported as ejected")
	}

	cache.Update(sd.Event{Instances: sd.Addresses("B:80")})
	waitFor(t, func() bool {
		return measured.Table().Stats(sd.Instance{Address: "A:80"}).FirstSeen.IsZero()
	})
}

// Measure follows registrations, not a health verdict. Handed a checked view it
// resolves the Instancer behind it, so an instance a probe withdraws keeps the
// measurements that would eject it instead of returning with a clean record.
func TestMeasureFollowsTheInstancerADerivedViewComesFrom(t *testing.T) {
	root := instance.NewCache()
	root.Update(sd.Event{Instances: sd.Addresses("A:80", "B:80")})

	view := &derivedView{source: root, cache: instance.NewCache()}
	view.cache.Update(sd.Event{Instances: sd.Addresses("A:80")})

	measured := feedback.Measure(view)
	t.Cleanup(func() { _ = measured.Close() })

	// B:80 is withdrawn by the view but still registered, so the accounting must
	// know it.
	waitFor(t, func() bool {
		return !measured.Table().Stats(sd.Instance{Address: "B:80"}).FirstSeen.IsZero()
	})
}

// derivedView publishes a filtered view of another Instancer, as active health
// checking does.
type derivedView struct {
	source sd.Instancer
	cache  *instance.Cache
}

func (v *derivedView) Register(ch chan sd.Event) sd.Event { return v.cache.Register(ch) }
func (v *derivedView) Deregister(ch chan sd.Event)        { v.cache.Deregister(ch) }
func (v *derivedView) Close() error                       { return v.cache.Close() }
func (v *derivedView) Underlying() sd.Instancer           { return v.source }

func TestMeasuredCloseStopsFollowingAndIsIdempotent(t *testing.T) {
	source := &fakeInstancer{}
	measured := feedback.Measure(source)

	if got := source.subscribers(); got != 1 {
		t.Fatalf("subscribers = %d, want 1", got)
	}
	if err := measured.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := measured.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if got := source.subscribers(); got != 0 {
		t.Fatalf("subscribers after Close = %d, want 0", got)
	}
}

func TestMeasureNilInstancerPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Measure accepted a nil Instancer")
		}
	}()
	feedback.Measure(nil)
}

// Handing an already-wrapped strategy back through the accounting must not count
// one call as two in flight.
func TestWrapDoesNotStackOnItsOwnTable(t *testing.T) {
	measured, set := measuredOver(t, "A:80")
	lb := measured.Balancer(set, measured.Table().Scored())

	picked, err := lb.Pick(context.Background(), nil)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if got := measured.Table().Stats(picked.Instance).InFlight; got != 1 {
		t.Fatalf("in-flight = %d, want 1", got)
	}
	picked.Done(sd.Outcome{})
	if got := measured.Table().Stats(picked.Instance).InFlight; got != 0 {
		t.Fatalf("in-flight after the call = %d, want 0", got)
	}
}

