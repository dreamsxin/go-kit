package instance

import (
	"sort"
	"sync"

	"github.com/dreamsxin/go-kit/v2/sd/events"
)

// Cache is an in-memory Instancer backed by explicit Update calls.
// It is the recommended Instancer for unit tests and local development
// where no external service registry is available.
type Cache struct {
	mtx   sync.RWMutex
	state events.Event
	reg   registry
}

func NewCache() *Cache {
	return &Cache{
		reg: registry{},
	}
}

// Update sets the current instance list (or error) and broadcasts the event
// to all registered subscribers.  Duplicate events (same instances + error)
// are silently dropped.
func (c *Cache) Update(event events.Event) {
	event = copyEvent(event)
	if event.Instances != nil {
		sort.Strings(event.Instances)
	}

	c.mtx.Lock()
	if eventsEqual(c.state, event) {
		c.mtx.Unlock()
		return
	}

	c.state = event
	subscribers := c.reg.subscribers()
	c.mtx.Unlock()

	broadcast(subscribers, event)
}

// State returns a copy of the most recently broadcast event.
func (c *Cache) State() events.Event {
	c.mtx.RLock()
	event := c.state
	c.mtx.RUnlock()
	eventCopy := copyEvent(event)
	return eventCopy
}

// 预留
func (c *Cache) Stop() {}

// Register subscribes ch to future events and synchronously returns the current
// state so callers can initialize before processing asynchronous updates.
func (c *Cache) Register(ch chan events.Event) events.Event {
	if ch == nil {
		return c.State()
	}
	c.mtx.Lock()
	c.reg.register(ch)
	event := c.state
	eventCopy := copyEvent(event)
	c.mtx.Unlock()
	return eventCopy
}

// Deregister removes ch from the subscriber list.
func (c *Cache) Deregister(ch chan events.Event) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.reg.deregister(ch)
}

// eventsEqual compares two events without external dependencies.
func eventsEqual(a, b events.Event) bool {
	if a.Err != b.Err {
		return false
	}
	if len(a.Instances) != len(b.Instances) {
		return false
	}
	for i := range a.Instances {
		if a.Instances[i] != b.Instances[i] {
			return false
		}
	}
	return true
}
