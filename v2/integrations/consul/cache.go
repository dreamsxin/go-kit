package consul

import (
	"maps"
	"reflect"
	"sort"
	"sync"

	"github.com/dreamsxin/go-kit/v2/sd"
)

// Instance and Event are aliases for the core discovery snapshot types, so a
// value built here is interchangeable with sd.Instance and sd.Event and the
// compiler keeps the two sides from drifting apart.
type Instance = sd.Instance

type Event = sd.Event

type eventCache struct {
	mu          sync.RWMutex
	state       Event
	subscribers map[chan Event]struct{}
}

func newEventCache() *eventCache {
	return &eventCache{subscribers: make(map[chan Event]struct{})}
}

func (c *eventCache) Update(event Event) {
	event = copyEvent(event)
	sort.Slice(event.Instances, func(i, j int) bool {
		return event.Instances[i].Address < event.Instances[j].Address
	})

	c.mu.Lock()
	if eventsEqual(c.state, event) {
		c.mu.Unlock()
		return
	}
	c.state = event
	subscribers := make([]chan Event, 0, len(c.subscribers))
	for subscriber := range c.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	c.mu.Unlock()

	for _, subscriber := range subscribers {
		sendLatest(subscriber, event)
	}
}

func (c *eventCache) Register(ch chan Event) Event {
	c.mu.Lock()
	if ch != nil {
		c.subscribers[ch] = struct{}{}
	}
	event := copyEvent(c.state)
	c.mu.Unlock()
	return event
}

func (c *eventCache) Deregister(ch chan Event) {
	c.mu.Lock()
	delete(c.subscribers, ch)
	c.mu.Unlock()
}

func sendLatest(ch chan Event, event Event) {
	event = copyEvent(event)
	select {
	case ch <- event:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- event:
	default:
	}
}

func copyEvent(event Event) Event {
	instances := make([]Instance, len(event.Instances))
	for i, item := range event.Instances {
		instances[i] = Instance{Address: item.Address, Metadata: maps.Clone(item.Metadata)}
	}
	event.Instances = instances
	return event
}

func eventsEqual(a, b Event) bool {
	if a.Err != b.Err || len(a.Instances) != len(b.Instances) {
		return false
	}
	for i := range a.Instances {
		if a.Instances[i].Address != b.Instances[i].Address {
			return false
		}
		// A relabelled instance is a change: consumers filter on labels, so
		// swallowing the update would leave them routing on stale ones.
		if !maps.EqualFunc(a.Instances[i].Metadata, b.Instances[i].Metadata, reflect.DeepEqual) {
			return false
		}
	}
	return true
}
