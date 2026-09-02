package instance

import (
	"maps"
	"reflect"
	"sort"
	"sync"

	"github.com/dreamsxin/go-kit/v2/sd"
)

// Cache is an in-memory Instancer backed by explicit Update calls.
// It is the recommended Instancer for unit tests and local development
// where no external service registry is available.
type Cache struct {
	mtx       sync.RWMutex
	state     sd.Event
	reg       registry
	closed    bool
	closeOnce sync.Once
}

var _ sd.Instancer = (*Cache)(nil)

func NewCache() *Cache {
	return &Cache{
		reg: registry{},
	}
}

// Update sets the current instance list (or error) and broadcasts the event
// to all registered subscribers.  Duplicate events (same instances + error)
// are silently dropped.
func (c *Cache) Update(event sd.Event) {
	event = copyEvent(event)
	sortInstances(event.Instances)

	c.mtx.Lock()
	if c.closed {
		c.mtx.Unlock()
		return
	}
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
func (c *Cache) State() sd.Event {
	c.mtx.RLock()
	event := c.state
	c.mtx.RUnlock()
	eventCopy := copyEvent(event)
	return eventCopy
}

// Register subscribes ch to future events and synchronously returns the current
// state so callers can initialize before processing asynchronous updates.
func (c *Cache) Register(ch chan sd.Event) sd.Event {
	if ch == nil {
		return c.State()
	}
	c.mtx.Lock()
	if c.closed {
		event := c.state
		c.mtx.Unlock()
		return copyEvent(event)
	}
	c.reg.register(ch)
	event := c.state
	eventCopy := copyEvent(event)
	c.mtx.Unlock()
	return eventCopy
}

// Deregister removes ch from the subscriber list.
func (c *Cache) Deregister(ch chan sd.Event) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.reg.deregister(ch)
}

// Close stops accepting updates and releases the subscriber registry. Existing
// subscribers own their channels and should close their subscriptions too.
func (c *Cache) Close() error {
	c.closeOnce.Do(func() {
		c.mtx.Lock()
		c.closed = true
		c.reg = registry{}
		c.mtx.Unlock()
	})
	return nil
}

// eventsEqual compares two events without external dependencies.
func eventsEqual(a, b sd.Event) bool {
	if a.Err != b.Err {
		return false
	}
	if len(a.Instances) != len(b.Instances) {
		return false
	}
	for i := range a.Instances {
		if !instancesEqual(a.Instances[i], b.Instances[i]) {
			return false
		}
	}
	return true
}

// instancesEqual treats a relabelled instance as a change, so subscribers see
// metadata updates even when the address set is untouched.
func instancesEqual(a, b sd.Instance) bool {
	return a.Address == b.Address && maps.EqualFunc(a.Metadata, b.Metadata, valuesEqual)
}

// valuesEqual falls back to DeepEqual because metadata values are any: a
// registry may hand back a slice or nested map, which == would panic on.
func valuesEqual(a, b any) bool {
	if a == b {
		return true
	}
	return reflect.DeepEqual(a, b)
}

func sortInstances(instances []sd.Instance) {
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].Address < instances[j].Address
	})
}
