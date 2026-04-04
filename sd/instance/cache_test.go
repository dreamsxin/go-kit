package instance_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/sd/events"
	"github.com/dreamsxin/go-kit/sd/instance"
)

// ─────────────────────────── helpers ───────────────────────────

func drain(ch <-chan events.Event, timeout time.Duration) (events.Event, bool) {
	select {
	case ev := <-ch:
		return ev, true
	case <-time.After(timeout):
		return events.Event{}, false
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
	c.Update(events.Event{Instances: []string{"a:80", "b:80"}})
	state := c.State()
	if len(state.Instances) != 2 {
		t.Errorf("expected 2 instances, got %d", len(state.Instances))
	}
}

func TestCache_UpdateDeduplicates(t *testing.T) {
	c := instance.NewCache()
	ch := make(chan events.Event, 4)
	c.Register(ch)
	drain(ch, 50*time.Millisecond) // consume initial empty event

	ev := events.Event{Instances: []string{"x:80"}}
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
	ch := make(chan events.Event, 2)
	c.Register(ch)
	drain(ch, 50*time.Millisecond)

	sentinel := errors.New("sd error")
	c.Update(events.Event{Err: sentinel})

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
	c.Update(events.Event{Instances: []string{"h:80"}})

	ch := make(chan events.Event, 1)
	c.Register(ch)

	got, ok := drain(ch, 100*time.Millisecond)
	if !ok {
		t.Fatal("Register should immediately send current state")
	}
	if len(got.Instances) != 1 || got.Instances[0] != "h:80" {
		t.Errorf("unexpected state: %+v", got)
	}
}

func TestCache_DeregisterStopsEvents(t *testing.T) {
	c := instance.NewCache()
	ch := make(chan events.Event, 4)
	c.Register(ch)
	drain(ch, 50*time.Millisecond)

	c.Deregister(ch)
	c.Update(events.Event{Instances: []string{"new:80"}})

	_, ok := drain(ch, 50*time.Millisecond)
	if ok {
		t.Error("deregistered channel should not receive events")
	}
}

// ─────────────────────────── State isolation (copy) ───────────────────────────

func TestCache_StateCopyIsIsolated(t *testing.T) {
	c := instance.NewCache()
	c.Update(events.Event{Instances: []string{"a:80"}})

	state := c.State()
	state.Instances[0] = "mutated"

	// original state should be unaffected
	orig := c.State()
	if orig.Instances[0] == "mutated" {
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
			c.Update(events.Event{Instances: []string{"host:80"}})
		}(i)
	}
	wg.Wait()
	// just ensure no race / panic
}
