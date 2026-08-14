package consul

import (
	"sort"
	"sync"
)

// Event mirrors the protocol-neutral discovery snapshot structurally, without
// coupling this provider module to a specific core module version.
type Event = struct {
	Instances []string
	Err       error
}

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
	if event.Instances != nil {
		sort.Strings(event.Instances)
	}

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
	event.Instances = append([]string(nil), event.Instances...)
	return event
}

func eventsEqual(a, b Event) bool {
	if a.Err != b.Err || len(a.Instances) != len(b.Instances) {
		return false
	}
	for i := range a.Instances {
		if a.Instances[i] != b.Instances[i] {
			return false
		}
	}
	return true
}
