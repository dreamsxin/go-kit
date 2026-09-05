package subscription

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
)

var errClosed = errors.New("test: closed")

func addresses(addrs ...string) []sd.Instance {
	instances := make([]sd.Instance, len(addrs))
	for i, addr := range addrs {
		instances[i] = sd.Instance{Address: addr}
	}
	return instances
}

func addressesOf(instances []sd.Instance) []string {
	addrs := make([]string, len(instances))
	for i, item := range instances {
		addrs[i] = item.Address
	}
	return addrs
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSortInstancesOrdersByAddress(t *testing.T) {
	instances := addresses("c:80", "a:80", "b:80")
	SortInstances(instances)
	if got := addressesOf(instances); !equal(got, []string{"a:80", "b:80", "c:80"}) {
		t.Fatalf("sorted = %v, want a, b, c", got)
	}
}

// snapshot is the projection used throughout: a sorted copy that owns nothing.
func snapshot(instances []sd.Instance) ([]sd.Instance, func() error) {
	sorted := append([]sd.Instance(nil), instances...)
	SortInstances(sorted)
	return sorted, nil
}

func TestStatePublishesSortedSnapshot(t *testing.T) {
	state := NewState(snapshot, errClosed, nil, Options{})
	state.Update(sd.Event{Instances: addresses("b:80", "a:80")})

	value, err := state.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if got := addressesOf(value); !equal(got, []string{"a:80", "b:80"}) {
		t.Fatalf("published = %v, want a, b", got)
	}
}

// Without InvalidateOnError a registry outage must not take instances that are
// still up out of service, however long it lasts.
func TestStateServesLastGoodSnapshotForever(t *testing.T) {
	state := NewState(snapshot, errClosed, nil, Options{})
	state.Update(sd.Event{Instances: addresses("a:80")})
	state.Update(sd.Event{Err: errors.New("registry down")})

	value, err := state.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if got := addressesOf(value); !equal(got, []string{"a:80"}) {
		t.Fatalf("published = %v, want the last good snapshot", got)
	}
}

func TestStateInvalidatesOnceTheGracePeriodElapses(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	state := NewState(snapshot, errClosed, nil, Options{
		InvalidateOnError: true,
		InvalidateTimeout: time.Minute,
		Now:               clock,
	})
	state.Update(sd.Event{Instances: addresses("a:80")})

	outage := errors.New("registry down")
	state.Update(sd.Event{Err: outage})
	value, err := state.Value()
	if err != nil {
		t.Fatalf("inside the grace period: %v", err)
	}
	if got := addressesOf(value); !equal(got, []string{"a:80"}) {
		t.Fatalf("inside the grace period published %v, want a:80", got)
	}

	now = now.Add(time.Minute)
	if _, err := state.Value(); !errors.Is(err, outage) {
		t.Fatalf("past the grace period error = %v, want the discovery error", err)
	}

	// A successful event ends the outage and republishes.
	state.Update(sd.Event{Instances: addresses("b:80")})
	value, err = state.Value()
	if err != nil {
		t.Fatalf("after recovery: %v", err)
	}
	if got := addressesOf(value); !equal(got, []string{"b:80"}) {
		t.Fatalf("after recovery published %v, want b:80", got)
	}
}

// Only the first error of a streak arms the deadline. A registry failing every
// second would otherwise keep pushing the deadline out and the snapshot would
// never be dropped.
func TestStateLaterErrorsDoNotExtendTheGracePeriod(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	state := NewState(snapshot, errClosed, nil, Options{
		InvalidateOnError: true,
		InvalidateTimeout: time.Minute,
		Now:               clock,
	})
	state.Update(sd.Event{Instances: addresses("a:80")})

	first := errors.New("first failure")
	state.Update(sd.Event{Err: first})
	now = now.Add(30 * time.Second)
	state.Update(sd.Event{Err: errors.New("second failure")})
	now = now.Add(31 * time.Second)

	if _, err := state.Value(); !errors.Is(err, first) {
		t.Fatalf("error = %v, want the first failure of the streak", err)
	}
}

// The release callback closes connections, which can block on a network
// timeout. Calling Value from inside it deadlocks unless the state has already
// dropped its lock.
func TestStateReleasesOutsideTheLock(t *testing.T) {
	var state *State[[]sd.Instance]
	released := make(chan struct{}, 1)
	first := true
	reconcile := func(instances []sd.Instance) ([]sd.Instance, func() error) {
		value, _ := snapshot(instances)
		if len(instances) > 0 || !first {
			return value, nil
		}
		first = false
		return value, func() error {
			// Would block forever if release ran under the write lock.
			state.Value()
			released <- struct{}{}
			return nil
		}
	}

	now := time.Now()
	state = NewState(reconcile, errClosed, nil, Options{
		InvalidateOnError: true,
		InvalidateTimeout: time.Minute,
		Now:               func() time.Time { return now },
	})
	state.Update(sd.Event{Instances: addresses("a:80")})
	state.Update(sd.Event{Err: errors.New("registry down")})
	now = now.Add(time.Minute)

	done := make(chan struct{})
	go func() {
		defer close(done)
		state.Value()
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("invalidation deadlocked: release ran while the lock was held")
	}
	select {
	case <-released:
	default:
		t.Fatal("release was not called on invalidation")
	}
}

func TestStateCloseReportsClosedErrorAndReleasesOnce(t *testing.T) {
	releaseErr := errors.New("close failed")
	calls := 0
	reconcile := func(instances []sd.Instance) ([]sd.Instance, func() error) {
		value, _ := snapshot(instances)
		return value, func() error {
			calls++
			return releaseErr
		}
	}

	state := NewState(reconcile, errClosed, nil, Options{})
	state.Update(sd.Event{Instances: addresses("a:80")})
	calls = 0

	release := state.Close()
	if release == nil {
		t.Fatal("Close returned no release for a projection that owns resources")
	}
	if err := release(); !errors.Is(err, releaseErr) {
		t.Fatalf("release error = %v, want %v", err, releaseErr)
	}
	if calls != 1 {
		t.Fatalf("release called %d times, want 1", calls)
	}

	if _, err := state.Value(); !errors.Is(err, errClosed) {
		t.Fatalf("Value after Close = %v, want the closed error", err)
	}
	if again := state.Close(); again != nil {
		t.Fatal("the second Close returned a release")
	}

	// A closed state ignores further events rather than republishing.
	state.Update(sd.Event{Instances: addresses("b:80")})
	if _, err := state.Value(); !errors.Is(err, errClosed) {
		t.Fatalf("Value after a post-close update = %v, want the closed error", err)
	}
}

// instancer is a minimal Instancer: one subscriber, an initial state, and
// manual updates.
type instancer struct {
	mtx         sync.Mutex
	initial     sd.Event
	subscribers map[chan sd.Event]struct{}
	deregisters int
}

func newInstancer(initial sd.Event) *instancer {
	return &instancer{initial: initial, subscribers: map[chan sd.Event]struct{}{}}
}

func (i *instancer) Register(ch chan sd.Event) sd.Event {
	i.mtx.Lock()
	defer i.mtx.Unlock()
	i.subscribers[ch] = struct{}{}
	return i.initial
}

func (i *instancer) Deregister(ch chan sd.Event) {
	i.mtx.Lock()
	defer i.mtx.Unlock()
	delete(i.subscribers, ch)
	i.deregisters++
}

func (i *instancer) Close() error { return nil }

func (i *instancer) publish(event sd.Event) {
	i.mtx.Lock()
	subscribers := make([]chan sd.Event, 0, len(i.subscribers))
	for subscriber := range i.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	i.mtx.Unlock()
	for _, subscriber := range subscribers {
		subscriber <- event
	}
}

func (i *instancer) deregistered() int {
	i.mtx.Lock()
	defer i.mtx.Unlock()
	return i.deregisters
}

// A caller holding a subscription already has instances, so the initial
// snapshot has to be applied before Start returns rather than on the pump.
func TestStartAppliesTheInitialSnapshotSynchronously(t *testing.T) {
	source := newInstancer(sd.Event{Instances: addresses("a:80")})
	state := NewState(snapshot, errClosed, nil, Options{})

	feed := Start(source, state.Update)
	defer feed.Stop()

	value, err := state.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if got := addressesOf(value); !equal(got, []string{"a:80"}) {
		t.Fatalf("published = %v, want the initial snapshot", got)
	}
}

func TestFeedPumpsUpdatesAndStopIsIdempotent(t *testing.T) {
	source := newInstancer(sd.Event{})
	applied := make(chan sd.Event, 4)
	feed := Start(source, func(event sd.Event) { applied <- event })

	<-applied // the initial snapshot
	source.publish(sd.Event{Instances: addresses("a:80")})
	select {
	case event := <-applied:
		if got := addressesOf(event.Instances); !equal(got, []string{"a:80"}) {
			t.Fatalf("applied %v, want a:80", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the pump did not apply a published event")
	}

	feed.Stop()
	feed.Stop()
	if got := source.deregistered(); got != 1 {
		t.Fatalf("deregistered %d times, want exactly 1", got)
	}
}

func TestStartPanicsOnNilInstancer(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Start accepted a nil Instancer")
		}
	}()
	Start(nil, func(sd.Event) {})
}
