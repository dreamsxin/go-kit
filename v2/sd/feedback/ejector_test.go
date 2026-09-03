package feedback_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/feedback"
)

// fakeClock lets the tests step through an ejection window instead of sleeping
// through it.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func TestEjectorRemovesTheFailingCandidate(t *testing.T) {
	table := feedback.NewTable(feedback.WithAlpha(1))
	bad := sd.Instance{Address: "bad:80"}
	good := sd.Instance{Address: "good:80"}
	other := sd.Instance{Address: "other:80"}
	table.Observe(bad, sd.Outcome{Err: errors.New("failed"), Latency: 50 * time.Millisecond})
	table.Observe(good, sd.Outcome{})
	table.Observe(other, sd.Outcome{})

	ejector := feedback.NewEjector(table, feedback.EjectionPolicy{
		MaxErrorRate: .5, MaxLatency: time.Second, MinSamples: 1,
	})
	kept := ejector.Filter()(context.Background(), []sd.Instance{bad, good, other})

	if addresses := addressesOf(kept); len(addresses) != 2 || addresses[0] == bad.Address {
		t.Fatalf("kept = %v, want the failing instance removed", addresses)
	}
	if !ejector.Ejected(bad) {
		t.Fatal("Ejected does not report the instance it just removed")
	}
	if ejector.Ejected(good) {
		t.Fatal("a healthy instance is reported as ejected")
	}
}

func TestEjectorHonoursMinSamples(t *testing.T) {
	table := feedback.NewTable(feedback.WithAlpha(1))
	bad := sd.Instance{Address: "bad:80"}
	good := sd.Instance{Address: "good:80"}
	table.Observe(bad, sd.Outcome{Err: errors.New("failed")})

	ejector := feedback.NewEjector(table, feedback.EjectionPolicy{MaxErrorRate: .5, MinSamples: 5})
	kept := ejector.Filter()(context.Background(), []sd.Instance{bad, good})
	if len(kept) != 2 {
		t.Fatalf("kept = %v, want both: one sample is not enough to judge", addressesOf(kept))
	}
}

func TestEjectorCapAdmitsAllWhenMostCandidatesAreUnhealthy(t *testing.T) {
	table := feedback.NewTable(feedback.WithAlpha(1))
	bad := sd.Instance{Address: "bad:80"}
	otherBad := sd.Instance{Address: "bad-2:80"}
	good := sd.Instance{Address: "good:80"}
	for _, instance := range []sd.Instance{bad, otherBad} {
		table.Observe(instance, sd.Outcome{Err: errors.New("failed")})
	}
	table.Observe(good, sd.Outcome{})

	ejector := feedback.NewEjector(table, feedback.EjectionPolicy{
		MaxErrorRate: .5, MinSamples: 1, MaxEjectionPercent: 50,
	})
	kept := ejector.Filter()(context.Background(), []sd.Instance{bad, otherBad, good})
	if len(kept) != 3 {
		t.Fatalf("kept = %v, want every candidate: ejecting two of three exceeds the cap", addressesOf(kept))
	}
	if ejector.Ejected(bad) {
		t.Fatal("the cap was exceeded, so nothing should have been ejected")
	}
}

func TestEjectorIgnoresMeasurementsOutsideTheCandidateSet(t *testing.T) {
	// Addresses that have left discovery must not count towards the ejection
	// cap. Otherwise a long-running table accumulates dead unhealthy entries
	// until every candidate set looks doomed and nothing is ever ejected.
	table := feedback.NewTable(feedback.WithAlpha(1))
	failed := errors.New("failed")
	for i := range 10 {
		table.Observe(sd.Instance{Address: fmt.Sprintf("retired-%d:80", i)}, sd.Outcome{Err: failed})
	}

	bad := sd.Instance{Address: "bad:80"}
	table.Observe(bad, sd.Outcome{Err: failed})
	candidates := []sd.Instance{bad}
	for i := range 3 {
		good := sd.Instance{Address: fmt.Sprintf("good-%d:80", i)}
		table.Observe(good, sd.Outcome{})
		candidates = append(candidates, good)
	}

	ejector := feedback.NewEjector(table, feedback.EjectionPolicy{MaxErrorRate: .5, MinSamples: 1})
	kept := ejector.Filter()(context.Background(), candidates)
	if len(kept) != 3 {
		t.Fatalf("kept = %v, want the one failing candidate ejected", addressesOf(kept))
	}
}

// A call issued before an ejection expired can answer after the instance is
// back in service, reporting the very failure that ejected it. Recording it
// would undo the recovery on the next selection — and with a doubled window, so
// a call longer than the ejection window could hold an instance out
// indefinitely.
func TestEjectorIgnoresStragglersFromBeforeTheInstanceReturned(t *testing.T) {
	clock := newFakeClock()
	failed := errors.New("refused")
	table := feedback.NewTable(feedback.WithAlpha(1))
	bad := sd.Instance{Address: "bad:80"}
	good := sd.Instance{Address: "good:80"}

	// A slow call, issued while the instance was still taking traffic.
	straggler := table.Track(bad)
	table.Observe(bad, sd.Outcome{Err: failed})

	ejector := feedback.NewEjector(table, feedback.EjectionPolicy{
		MaxErrorRate: .5, MinSamples: 1, BaseDuration: time.Minute,
	}, feedback.WithEjectorClock(clock.Now))
	filter := ejector.Filter()
	candidates := []sd.Instance{bad, good}

	if kept := filter(context.Background(), candidates); len(kept) != 1 {
		t.Fatalf("kept = %v, want the failing instance ejected", addressesOf(kept))
	}

	clock.advance(61 * time.Second)
	if kept := filter(context.Background(), candidates); len(kept) != 2 {
		t.Fatalf("kept = %v, want the instance back in service", addressesOf(kept))
	}

	straggler(sd.Outcome{Err: failed})

	if kept := filter(context.Background(), candidates); len(kept) != 2 {
		t.Fatalf("kept = %v, want the instance kept: a result from before the "+
			"reset is exactly what the reset discarded", addressesOf(kept))
	}
	if ejector.Ejected(bad) {
		t.Fatal("a straggling failure re-ejected an instance that had recovered")
	}
}

// The point of the whole component: an instance receiving no traffic produces no
// new measurements, so ejection has to expire and the measurements that caused
// it have to be cleared. Otherwise the first ejection is permanent.
func TestEjectorReturnsTheInstanceAndClearsWhatEjectedIt(t *testing.T) {
	clock := newFakeClock()
	table := feedback.NewTable(feedback.WithAlpha(1))
	bad := sd.Instance{Address: "bad:80"}
	good := sd.Instance{Address: "good:80"}
	table.Observe(bad, sd.Outcome{Err: errors.New("failed")})
	table.Observe(good, sd.Outcome{})
	table.Observe(good, sd.Outcome{})

	ejector := feedback.NewEjector(table, feedback.EjectionPolicy{
		MaxErrorRate: .5, MinSamples: 1, BaseDuration: time.Minute,
	}, feedback.WithEjectorClock(clock.Now))
	filter := ejector.Filter()
	candidates := []sd.Instance{bad, good}

	if kept := filter(context.Background(), candidates); len(kept) != 1 {
		t.Fatalf("kept = %v, want the failing instance ejected", addressesOf(kept))
	}

	// Still inside the window.
	clock.advance(30 * time.Second)
	if kept := filter(context.Background(), candidates); len(kept) != 1 {
		t.Fatalf("kept = %v, want the instance still ejected", addressesOf(kept))
	}

	clock.advance(31 * time.Second)
	kept := filter(context.Background(), candidates)
	if len(kept) != 2 {
		t.Fatalf("kept = %v, want the instance back in service", addressesOf(kept))
	}
	if ejector.Ejected(bad) {
		t.Fatal("Ejected still reports an expired ejection")
	}
	if stats := table.Stats(bad); stats.Samples != 0 || stats.ErrorRate != 0 {
		t.Fatalf("stats after return = %+v, want the measurements cleared; a decayed "+
			"average never recovers without traffic, so keeping it re-ejects immediately", stats)
	}
}

func TestEjectorBacksOffForRepeatOffenders(t *testing.T) {
	clock := newFakeClock()
	table := feedback.NewTable(feedback.WithAlpha(1))
	bad := sd.Instance{Address: "bad:80"}
	good := sd.Instance{Address: "good:80"}
	failed := errors.New("failed")
	table.Observe(good, sd.Outcome{})
	table.Observe(good, sd.Outcome{})

	ejector := feedback.NewEjector(table, feedback.EjectionPolicy{
		MaxErrorRate: .5, MinSamples: 1,
		BaseDuration: 10 * time.Second, MaxDuration: time.Minute,
	}, feedback.WithEjectorClock(clock.Now))
	filter := ejector.Filter()
	candidates := []sd.Instance{bad, good}

	// First offence: a ten second window.
	table.Observe(bad, sd.Outcome{Err: failed})
	filter(context.Background(), candidates)
	clock.advance(11 * time.Second)
	filter(context.Background(), candidates)
	if ejector.Ejected(bad) {
		t.Fatal("the first ejection did not expire")
	}

	// Second offence: twenty seconds, because it failed again.
	table.Observe(bad, sd.Outcome{Err: failed})
	filter(context.Background(), candidates)
	clock.advance(11 * time.Second)
	if !ejector.Ejected(bad) {
		t.Fatal("the second ejection lasted no longer than the first")
	}
	clock.advance(10 * time.Second)
	filter(context.Background(), candidates)
	if ejector.Ejected(bad) {
		t.Fatal("the second ejection outlasted its doubled window")
	}
}

func TestEjectorRetainDropsStateForRemovedInstances(t *testing.T) {
	clock := newFakeClock()
	table := feedback.NewTable(feedback.WithAlpha(1))
	bad := sd.Instance{Address: "bad:80"}
	good := sd.Instance{Address: "good:80"}
	table.Observe(bad, sd.Outcome{Err: errors.New("failed")})
	table.Observe(good, sd.Outcome{})
	table.Observe(good, sd.Outcome{})

	ejector := feedback.NewEjector(table, feedback.EjectionPolicy{
		MaxErrorRate: .5, MinSamples: 1, BaseDuration: time.Hour,
	}, feedback.WithEjectorClock(clock.Now))
	ejector.Filter()(context.Background(), []sd.Instance{bad, good})
	if !ejector.Ejected(bad) {
		t.Fatal("the failing instance was not ejected")
	}

	ejector.Retain([]sd.Instance{good})
	if ejector.Ejected(bad) {
		t.Fatal("ejection state survived the instance leaving discovery")
	}
}

// Follow drives every retainer from one subscription, so a table and the ejector
// reading it cannot disagree about which instances exist.
func TestFollowDrivesTableAndEjector(t *testing.T) {
	clock := newFakeClock()
	bad := sd.Instance{Address: "bad:80"}
	good := sd.Instance{Address: "good:80"}
	instancer := &fakeInstancer{state: sd.Event{Instances: []sd.Instance{bad, good}}}

	table := feedback.NewTable(feedback.WithAlpha(1))
	table.Observe(bad, sd.Outcome{Err: errors.New("failed")})
	table.Observe(good, sd.Outcome{})
	table.Observe(good, sd.Outcome{})

	ejector := feedback.NewEjector(table, feedback.EjectionPolicy{
		MaxErrorRate: .5, MinSamples: 1, BaseDuration: time.Hour,
	}, feedback.WithEjectorClock(clock.Now))
	ejector.Filter()(context.Background(), []sd.Instance{bad, good})

	following := feedback.Follow(instancer, table, ejector)
	defer following.Close() //nolint:errcheck

	instancer.publish(sd.Event{Instances: []sd.Instance{good}})
	waitFor(t, func() bool { return table.Stats(bad).Samples == 0 && !ejector.Ejected(bad) })
}

func TestFollowNilInstancerPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()
	feedback.Follow(nil)
}

func TestNewEjectorNilTablePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()
	feedback.NewEjector(nil, feedback.EjectionPolicy{})
}
