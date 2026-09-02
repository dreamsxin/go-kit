package instance_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/instance"
)

// ─────────────────────────── helpers ───────────────────────────

func drain(ch <-chan sd.Event, timeout time.Duration) (sd.Event, bool) {
	select {
	case ev := <-ch:
		return ev, true
	case <-time.After(timeout):
		return sd.Event{}, false
	}
}

// ─────────────────────────── Cache.Update / State ───────────────────────────

func TestCache_InitialStateEmpty(t *testing.T) {
	c := instance.NewCache()
	state := c.State()
	if state.Err != nil || len(state.Instances) != 0 {
		t.Errorf("initial state should be empty, got %+v", state)
	}
}

func TestCache_UpdateSetsState(t *testing.T) {
	c := instance.NewCache()
	c.Update(sd.Event{Instances: sd.Addresses("a:80", "b:80")})
	state := c.State()
	if len(state.Instances) != 2 {
		t.Errorf("expected 2 instances, got %d", len(state.Instances))
	}
}

func TestCache_UpdateDoesNotMutateInputInstances(t *testing.T) {
	c := instance.NewCache()
	instances := sd.Addresses("b:80", "a:80")

	c.Update(sd.Event{Instances: instances})

	if instances[0].Address != "b:80" || instances[1].Address != "a:80" {
		t.Fatalf("Update mutated input slice: %v", instances)
	}
	state := c.State()
	if state.Instances[0].Address != "a:80" || state.Instances[1].Address != "b:80" {
		t.Fatalf("state instances = %v, want sorted copy", state.Instances)
	}
}

func TestCache_UpdateDeduplicates(t *testing.T) {
	c := instance.NewCache()
	ch := make(chan sd.Event, 4)
	c.Register(ch)

	ev := sd.Event{Instances: sd.Addresses("x:80")}
	c.Update(ev)
	c.Update(ev) // same event — should be ignored

	got, ok := drain(ch, 50*time.Millisecond)
	if !ok {
		t.Fatal("expected first update to be broadcast")
	}
	if len(got.Instances) != 1 {
		t.Errorf("expected 1 instance, got %d", len(got.Instances))
	}

	// second identical update should NOT produce another event
	_, ok = drain(ch, 50*time.Millisecond)
	if ok {
		t.Error("duplicate update should not be broadcast")
	}
}

func TestCache_UpdateErrorEvent(t *testing.T) {
	c := instance.NewCache()
	ch := make(chan sd.Event, 2)
	c.Register(ch)

	sentinel := errors.New("sd error")
	c.Update(sd.Event{Err: sentinel})

	got, ok := drain(ch, 50*time.Millisecond)
	if !ok {
		t.Fatal("expected error event to be broadcast")
	}
	if got.Err != sentinel {
		t.Errorf("expected sentinel error, got %v", got.Err)
	}
}

// ─────────────────────────── Register / Deregister ───────────────────────────

func TestCache_RegisterReceivesCurrentState(t *testing.T) {
	c := instance.NewCache()
	c.Update(sd.Event{Instances: sd.Addresses("h:80")})

	ch := make(chan sd.Event, 1)
	got := c.Register(ch)
	if len(got.Instances) != 1 || got.Instances[0].Address != "h:80" {
		t.Errorf("unexpected state: %+v", got)
	}
}

func TestCache_RegisterDoesNotBlockOnUnbufferedSubscriber(t *testing.T) {
	c := instance.NewCache()
	c.Update(sd.Event{Instances: sd.Addresses("h:80")})
	ch := make(chan sd.Event)
	done := make(chan struct{})
	go func() {
		_ = c.Register(ch)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Register blocked on an unbuffered subscriber")
	}
}

func TestCache_DeregisterStopsEvents(t *testing.T) {
	c := instance.NewCache()
	ch := make(chan sd.Event, 4)
	c.Register(ch)

	c.Deregister(ch)
	c.Update(sd.Event{Instances: sd.Addresses("new:80")})

	_, ok := drain(ch, 50*time.Millisecond)
	if ok {
		t.Error("deregistered channel should not receive events")
	}
}

func TestCache_UpdateDoesNotBlockOnSlowSubscriber(t *testing.T) {
	c := instance.NewCache()
	ch := make(chan sd.Event, 1)
	c.Register(ch)

	done := make(chan struct{})
	go func() {
		c.Update(sd.Event{Instances: sd.Addresses("new:80")})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Update blocked on a slow subscriber")
	}
}

// ─────────────────────────── State isolation (copy) ───────────────────────────
func TestCache_StateCopyIsIsolated(t *testing.T) {
	c := instance.NewCache()
	c.Update(sd.Event{Instances: sd.Addresses("a:80")})

	state := c.State()
	state.Instances[0] = sd.Instance{Address: "mutated"}

	// original state should be unaffected
	orig := c.State()
	if orig.Instances[0].Address == "mutated" {
		t.Error("State() should return a copy, not a reference")
	}
}

// ─────────────────────────── Concurrency ───────────────────────────

func TestCache_ConcurrentUpdates(t *testing.T) {
	c := instance.NewCache()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			c.Update(sd.Event{Instances: sd.Addresses("host:80")})
		}(i)
	}
	wg.Wait()
	// just ensure no race / panic
}

// ─────────────────────────── Metadata ───────────────────────────

// Consumers filter and weight on labels, so a relabel is a real change even
// when the address set is identical. Swallowing it would leave every subscriber
// routing on stale labels.
func TestCache_RelabelIsBroadcast(t *testing.T) {
	c := instance.NewCache()
	updates := make(chan sd.Event, 1)
	c.Register(updates)

	c.Update(sd.Event{Instances: []sd.Instance{
		{Address: "svc:80", Metadata: map[string]any{"zone": "north"}},
	}})
	if got := <-updates; len(got.Instances) != 1 {
		t.Fatalf("first update = %v, want one instance", got.Instances)
	}

	c.Update(sd.Event{Instances: []sd.Instance{
		{Address: "svc:80", Metadata: map[string]any{"zone": "south"}},
	}})
	select {
	case got := <-updates:
		zone, _ := sd.MetadataString(got.Instances[0].Metadata, "zone")
		if zone != "south" {
			t.Fatalf("broadcast zone = %q, want south", zone)
		}
	case <-time.After(time.Second):
		t.Fatal("a relabel was not broadcast")
	}
	c.Deregister(updates)
}

func TestCache_IdenticalLabelsAreNotRebroadcast(t *testing.T) {
	c := instance.NewCache()
	labels := map[string]any{"zone": "north"}
	c.Update(sd.Event{Instances: []sd.Instance{{Address: "svc:80", Metadata: labels}}})

	updates := make(chan sd.Event, 1)
	c.Register(updates)
	c.Update(sd.Event{Instances: []sd.Instance{{Address: "svc:80", Metadata: map[string]any{"zone": "north"}}}})

	select {
	case got := <-updates:
		t.Fatalf("an unchanged snapshot was rebroadcast: %v", got.Instances)
	case <-time.After(50 * time.Millisecond):
	}
	c.Deregister(updates)
}

// The cache must not alias caller metadata, or a later caller-side edit would
// silently rewrite the published snapshot.
func TestCache_CopiesMetadata(t *testing.T) {
	c := instance.NewCache()
	labels := map[string]any{"zone": "north"}
	c.Update(sd.Event{Instances: []sd.Instance{{Address: "svc:80", Metadata: labels}}})

	labels["zone"] = "mutated"

	zone, _ := sd.MetadataString(c.State().Instances[0].Metadata, "zone")
	if zone != "north" {
		t.Fatalf("published zone = %q, want north", zone)
	}
}
