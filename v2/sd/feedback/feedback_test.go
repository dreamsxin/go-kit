package feedback_test

import (
	"context"
	"errors"
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
