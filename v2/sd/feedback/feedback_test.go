package feedback_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/feedback"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

func TestTrackRecordsOutcomeAndReleasesInflight(t *testing.T) {
	table := feedback.NewTable(feedback.WithAlpha(1))
	instance := sd.Instance{Address: "svc:80"}
	done := table.Track(instance)
	if got := table.Stats(instance).InFlight; got != 1 {
		t.Fatalf("in-flight before done = %d, want 1", got)
	}
	done(sd.Outcome{Err: errors.New("failed"), Latency: 25 * time.Millisecond, Bytes: 10})
	done(sd.Outcome{Latency: time.Second})
	stats := table.Stats(instance)
	if stats.InFlight != 0 || stats.Samples != 1 {
		t.Fatalf("stats after done = %+v, want one completed call", stats)
	}
	if stats.ErrorRate != 1 || stats.Latency != 25*time.Millisecond || stats.Bytes != 10 {
		t.Fatalf("stats = %+v, want recorded outcome", stats)
	}
}

func TestRetainDropsMeasurementsForAddressesThatLeftDiscovery(t *testing.T) {
	table := feedback.NewTable(feedback.WithAlpha(1))
	kept := sd.Instance{Address: "kept:80"}
	gone := sd.Instance{Address: "gone:80"}
	table.Observe(kept, sd.Outcome{Latency: time.Millisecond})
	table.Observe(gone, sd.Outcome{Latency: time.Millisecond})

	table.Retain([]sd.Instance{kept})

	if table.Stats(gone).Samples != 0 {
		t.Fatal("measurements for a removed address survived Retain")
	}
	if table.Stats(kept).Samples != 1 {
		t.Fatal("Retain dropped an address that is still discovered")
	}
}

// A rolling deployment replaces addresses one for one, so a stale table can be
// exactly as large as the live snapshot. Retain must not decide there is nothing
// to do by comparing sizes.
func TestRetainDropsReplacedAddressesOfTheSameCount(t *testing.T) {
	table := feedback.NewTable(feedback.WithAlpha(1))
	old := sd.Instance{Address: "v1:80"}
	kept := sd.Instance{Address: "kept:80"}
	table.Observe(old, sd.Outcome{})
	table.Observe(kept, sd.Outcome{})

	replacement := sd.Instance{Address: "v2:80"}
	table.Retain([]sd.Instance{replacement, kept})

	if table.Stats(old).Samples != 0 {
		t.Fatal("the replaced address kept its measurements")
	}
}

func TestRetainKeepsInstancesWithCallsStillInFlight(t *testing.T) {
	table := feedback.NewTable()
	leaving := sd.Instance{Address: "leaving:80"}
	staying := sd.Instance{Address: "staying:80"}
	table.Observe(staying, sd.Outcome{})
	done := table.Track(leaving)

	table.Retain([]sd.Instance{staying})
	if table.Stats(leaving).InFlight != 1 {
		t.Fatal("Retain dropped an instance with a call in flight")
	}

	// No second Retain: the completion itself closes the loop. Waiting for the
	// next snapshot would keep the entry forever in a service whose instance
	// set never changes again.
	done(sd.Outcome{})
	if got := table.Stats(leaving); got.Samples != 0 || !got.FirstSeen.IsZero() {
		t.Fatalf("stats after the last call = %+v, want the entry gone", got)
	}
}

// An address can be retired and then come back — a restart that reuses the
// address, a flapping registration. Its retirement must not follow it.
func TestRetainClearsRetirementWhenAnAddressReturns(t *testing.T) {
	table := feedback.NewTable()
	flapping := sd.Instance{Address: "flapping:80"}
	done := table.Track(flapping)

	table.Retain(sd.Addresses("other:80"))
	table.Retain([]sd.Instance{flapping})
	done(sd.Outcome{Latency: time.Millisecond})

	if got := table.Stats(flapping); got.Samples != 1 {
		t.Fatalf("stats = %+v, want the measurement kept for a live address", got)
	}
}

// Reset returns an ejected instance to service by discarding what got it
// ejected. A call issued before the reset belongs to that discarded period, and
// recording it would reverse the recovery: Reset zeroes the sample count, so a
// stale result seeds the average at full weight instead of decaying into it, and
// one straggler would eject the instance again immediately.
func TestResetDiscardsResultsFromCallsAlreadyInFlight(t *testing.T) {
	table := feedback.NewTable(feedback.WithAlpha(1))
	recovering := sd.Instance{Address: "recovering:80"}

	straggler := table.Track(recovering)
	table.Reset(recovering)
	straggler(sd.Outcome{Err: errors.New("refused")})

	stats := table.Stats(recovering)
	if stats.Samples != 0 || stats.ErrorRate != 0 {
		t.Fatalf("stats = %+v, want the pre-reset failure discarded", stats)
	}
	if stats.InFlight != 0 {
		t.Fatalf("in flight = %d, want the slot released even for a discarded result", stats.InFlight)
	}

	// A call issued after the reset is the evidence that counts.
	table.Track(recovering)(sd.Outcome{Err: errors.New("refused again")})
	if got := table.Stats(recovering); got.Samples != 1 || got.ErrorRate != 1 {
		t.Fatalf("stats = %+v, want the post-reset failure recorded", got)
	}
}

// Slow start ramps against the moment an instance joined the service. Following
// discovery is what supplies that moment; without it an instance nobody has
// called yet is unknown, and slow start treats unknown as brand new forever.
func TestRetainRegistersTheArrivalTimeOfNewInstances(t *testing.T) {
	now := time.Unix(1000, 0)
	table := feedback.NewTable(feedback.WithClock(func() time.Time { return now }))
	firstSeen := func(instance sd.Instance) (time.Time, bool) {
		at := table.Stats(instance).FirstSeen
		return at, !at.IsZero()
	}

	early := sd.Instance{Address: "early:80"}
	table.Retain([]sd.Instance{early})

	at, known := firstSeen(early)
	if !known || !at.Equal(time.Unix(1000, 0)) {
		t.Fatalf("first seen = (%v, %v), want the arrival time with no call made", at, known)
	}

	// Arrival, not last seen: a later snapshot must not restart the ramp.
	now = time.Unix(2000, 0)
	late := sd.Instance{Address: "late:80"}
	table.Retain([]sd.Instance{early, late})

	if at, _ := firstSeen(early); !at.Equal(time.Unix(1000, 0)) {
		t.Errorf("first seen for a known address = %v, want it unchanged", at)
	}
	if at, known := firstSeen(late); !known || !at.Equal(time.Unix(2000, 0)) {
		t.Errorf("first seen for a new address = (%v, %v), want the second snapshot's time", at, known)
	}
}

// Claiming the entry and the in-flight slot in one critical section is what
// keeps a concurrent Retain from deleting an entry a call is about to use.
func TestTrackAccountsEveryCallWhileRetainRuns(t *testing.T) {
	table := feedback.NewTable(feedback.WithAlpha(1))
	live := sd.Instance{Address: "live:80"}
	snapshot := []sd.Instance{live}

	const calls = 500
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < calls; i++ {
			table.Retain(snapshot)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < calls; i++ {
			table.Track(live)(sd.Outcome{})
		}
	}()
	wg.Wait()

	if got := table.Stats(live); got.Samples != calls || got.InFlight != 0 {
		t.Fatalf("stats = %+v, want %d samples and nothing in flight", got, calls)
	}
}

func TestFollowRetainsOnEverySnapshot(t *testing.T) {
	first := sd.Instance{Address: "a:80"}
	second := sd.Instance{Address: "b:80"}
	instancer := &fakeInstancer{state: sd.Event{Instances: []sd.Instance{first, second}}}

	table := feedback.NewTable(feedback.WithAlpha(1))
	table.Observe(first, sd.Outcome{})
	table.Observe(second, sd.Outcome{})

	following := feedback.Follow(instancer, table)
	defer following.Close()

	instancer.publish(sd.Event{Instances: []sd.Instance{first}})
	waitFor(t, func() bool { return table.Stats(second).Samples == 0 })

	// A failed snapshot says nothing about which instances exist.
	instancer.publish(sd.Event{Err: errors.New("registry down")})
	time.Sleep(10 * time.Millisecond)
	if table.Stats(first).Samples != 1 {
		t.Fatal("a discovery error dropped live measurements")
	}

	if err := following.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if instancer.subscribers() != 0 {
		t.Fatal("Close did not deregister the follower")
	}
}

func TestWrapForwardsCloseToTheStrategyItDecorates(t *testing.T) {
	table := feedback.NewTable()
	inner := &closableStrategy{}
	// The assembly a caller actually writes: accounting over a filter over a
	// strategy that owns something. Only the outermost layer is ever handed to
	// selector.New or balancer.New, so every layer has to forward Close.
	strategy := table.Wrap(selector.Filtered(inner, nil))

	pick := selector.New(selector.Static(sd.Addresses("a:80")...), strategy)
	if err := pick.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := pick.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if inner.closes != 1 {
		t.Fatalf("strategy closed %d times through Wrap and Filtered, want 1", inner.closes)
	}
}

func TestFollowStopsWhenAProviderClosesTheChannel(t *testing.T) {
	instancer := &fakeInstancer{state: sd.Event{Instances: sd.Addresses("a:80")}}
	counter := &countingRetainer{}

	following := feedback.Follow(instancer, counter)
	defer following.Close()

	// Closing a subscriber channel breaks the sd.Instancer contract, but a
	// provider that does it must not turn the follower into a hot loop
	// retaining zero-value snapshots.
	instancer.closeSubscribers()
	time.Sleep(20 * time.Millisecond)

	if got := counter.count(); got != 1 {
		t.Fatalf("Retain called %d times, want 1 (the initial snapshot only)", got)
	}
}

type countingRetainer struct {
	mu     sync.Mutex
	calls  int
	latest []sd.Instance
}

func (c *countingRetainer) Retain(instances []sd.Instance) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	c.latest = instances
}

func (c *countingRetainer) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

type closableStrategy struct {
	closes int
}

func (s *closableStrategy) Pick(context.Context, any, []sd.Instance) (int, sd.Done, error) {
	return 0, nil, nil
}

func (s *closableStrategy) Close() error { s.closes++; return nil }

func TestLeastRequestPrefersTheLeastLoadedInstance(t *testing.T) {
	table := feedback.NewTable()
	busy := sd.Instance{Address: "busy:80"}
	idle := sd.Instance{Address: "idle:80"}
	// Two calls stay outstanding on busy and none on idle.
	table.Track(busy)
	table.Track(busy)

	strategy := table.LeastRequest(selector.WithChoices(8))
	instances := []sd.Instance{busy, idle}

	// Candidates are sampled with replacement, so every sample landing on the
	// busy instance is possible — 1/256 here. The guarantee is the
	// distribution, not any single pick.
	const rounds = 400
	picks := map[string]int{}
	for range rounds {
		index, done, err := strategy.Pick(context.Background(), nil, instances)
		if err != nil {
			t.Fatalf("Pick: %v", err)
		}
		if done == nil {
			t.Fatal("least request must report its own outcome back to the table")
		}
		picks[instances[index].Address]++
		done(sd.Outcome{})
	}

	if picks[idle.Address] < rounds*9/10 {
		t.Fatalf("picks = %v, want the idle instance to take the large majority", picks)
	}
	if got := table.Stats(idle).Samples; got != uint64(picks[idle.Address]) {
		t.Fatalf("samples recorded for the idle instance = %d, want %d", got, picks[idle.Address])
	}
}

func addressesOf(instances []sd.Instance) []string {
	addresses := make([]string, len(instances))
	for i, instance := range instances {
		addresses[i] = instance.Address
	}
	return addresses
}

func waitFor(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met in time")
}

type fakeInstancer struct {
	state       sd.Event
	subscribed  []chan sd.Event
	deregisters int
}

func (f *fakeInstancer) Register(ch chan sd.Event) sd.Event {
	if ch != nil {
		f.subscribed = append(f.subscribed, ch)
	}
	return f.state
}

func (f *fakeInstancer) Deregister(ch chan sd.Event) {
	for i, subscriber := range f.subscribed {
		if subscriber == ch {
			f.subscribed = append(f.subscribed[:i], f.subscribed[i+1:]...)
			f.deregisters++
			return
		}
	}
}

func (f *fakeInstancer) Close() error { return nil }

func (f *fakeInstancer) publish(event sd.Event) {
	f.state = event
	for _, subscriber := range f.subscribed {
		subscriber <- event
	}
}

func (f *fakeInstancer) subscribers() int { return len(f.subscribed) }

// closeSubscribers breaks the sd.Instancer contract on purpose: no provider may
// close a channel it was handed, and consumers must survive one that does.
func (f *fakeInstancer) closeSubscribers() {
	for _, subscriber := range f.subscribed {
		close(subscriber)
	}
	f.subscribed = nil
}
