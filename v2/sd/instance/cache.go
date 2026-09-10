// Package instance provides an Instancer driven by explicit updates.
//
// Every other Instancer in this framework watches something — Consul, etcd, DNS.
// Cache watches nothing: the instance list is whatever the last Update call said.
// That makes it the Instancer for a test, for local development with no registry
// running, and for an application whose instance list comes from somewhere this
// framework has no provider for.
//
// It satisfies the same sd.Instancer contract as a real provider, so everything
// downstream — endpointer, selector, balancer, retry — behaves identically. A
// test that drives Cache is exercising the real selection path, not a stub of it.
//
// Two behaviours are worth knowing before relying on it. Duplicate events are
// dropped: an Update whose instances and error match the current state
// broadcasts nothing, so a caller can poll a source without waking subscribers
// for no reason. And instances are sorted before being stored, so an event's
// identity does not depend on the order a source happened to return.
package instance

import (
	"maps"
	"reflect"
	"sync"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/internal/subscription"
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
	subscription.SortInstances(event.Instances)

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
	// Broadcasting happens under the lock, so the order subscribers observe is the
	// order the state was set in. It used to happen after unlocking: two callers
	// updating concurrently could set A then B and deliver B then A, and sendLatest
	// made it worse — finding the buffer full, it drains and rewrites, so the older
	// event replaced the newer one. State() then disagreed with every subscriber
	// until the next update, and since an event equal to the stored one is dropped, a
	// repeat of the stale list kept the divergence instead of correcting it.
	//
	// Holding the lock across the send is safe because sendLatest never blocks: every
	// channel operation in it has a default case. A slow subscriber cannot stall an
	// update, which is the property that made the unlocked broadcast look necessary.
	// Stable: sd.instance-subscribers-are-never-left-behind-state — concurrent updates deliver in the order they were stored, so no subscriber holds an event older than the one State reports.
	// Covered by: TestCache_ConcurrentUpdatesLeaveSubscribersOnTheStoredState
	broadcast(c.reg.subscribers(), event)
	c.mtx.Unlock()
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
