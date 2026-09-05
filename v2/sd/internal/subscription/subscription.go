// Package subscription is the one subscribe, error-grace, and invalidation
// state machine behind every consumer of an sd.Instancer in this module.
//
// The consumers publish different things — sd/selector.Subscription publishes a
// sorted instance snapshot, sd/endpointer publishes instance/endpoint pairs,
// sd/feedback keeps per-instance accounting aligned — but everything between the
// Instancer and that published value is the same problem: pump events off the
// channel, hold the last good snapshot through a registry outage, and drop it
// once the grace period has elapsed. Written twice it was answered twice, so it
// lives here and each consumer supplies only its own projection.
package subscription

import (
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
)

// SortInstances orders a snapshot by address, so every consumer walks the same
// sequence rather than whatever order the registry replied in. Round robin needs
// it to be a rotation instead of a shuffle, and a hash ring built from an
// unsorted snapshot would remap keys on a reordering that changed nothing.
func SortInstances(instances []sd.Instance) {
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].Address < instances[j].Address
	})
}

// Options controls how a State reacts to service-discovery errors.
type Options struct {
	// InvalidateOnError drops the published value once InvalidateTimeout has
	// elapsed from the first error of a failure streak. Without it the last good
	// snapshot is served indefinitely, which is the right default for a registry
	// outage: instances that are still up should keep receiving traffic.
	InvalidateOnError bool
	InvalidateTimeout time.Duration
	// Now replaces the clock. A zero value means time.Now.
	Now func() time.Time
}

// Reconcile projects a discovery snapshot into the value a consumer publishes.
//
// It runs under the State lock, so a consumer may keep whatever bookkeeping the
// projection needs — an address-to-endpoint map, for one — without a second lock
// of its own. The returned release reports what the previous value owned and the
// new one does not; it is nil for a projection that owns nothing. It runs after
// the lock is dropped, because closing a retired connection while holding the
// lock would park every reader on a network timeout.
//
// Reconcile is also how invalidation and closing are expressed: both hand it a
// nil snapshot, so an implementation that releases what it owns gets that for
// free rather than having to remember it three times.
type Reconcile[T any] func(instances []sd.Instance) (value T, release func() error)

// State holds one subscription's published value, the discovery error covering
// it, and the deadline at which that error stops being covered.
type State[T any] struct {
	reconcile Reconcile[T]
	closedErr error
	options   Options
	logger    *slog.Logger
	now       func() time.Time

	mtx      sync.RWMutex
	value    T
	err      error
	deadline time.Time
	closed   bool
}

// NewState creates a state machine that publishes what reconcile projects and
// reports closedErr once Close has run.
func NewState[T any](reconcile Reconcile[T], closedErr error, logger *slog.Logger, options Options) *State[T] {
	if reconcile == nil {
		panic("subscription: nil reconcile")
	}
	if closedErr == nil {
		panic("subscription: nil closed error")
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &State[T]{
		reconcile: reconcile,
		closedErr: closedErr,
		options:   options,
		logger:    logger,
		now:       now,
	}
}

// Update applies one discovery event.
func (s *State[T]) Update(event sd.Event) {
	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		return
	}

	if event.Err == nil {
		value, release := s.reconcile(event.Instances)
		s.value = value
		s.err = nil
		s.mtx.Unlock()
		s.release(release)
		return
	}

	s.logger.Debug("service discovery update failed", "err", event.Err)
	// Only the first error of a streak arms the deadline. A registry that keeps
	// failing would otherwise keep pushing the moment its snapshot goes stale
	// further out, and the grace period would never end.
	if !s.options.InvalidateOnError || s.err != nil {
		s.mtx.Unlock()
		return
	}
	s.err = event.Err
	s.deadline = s.now().Add(s.options.InvalidateTimeout)
	s.mtx.Unlock()
}

// Value reports the published value while a discovery error is within its grace
// period, and the error itself once the value has been invalidated. A closed
// state reports the error it was built with.
func (s *State[T]) Value() (T, error) {
	var zero T

	s.mtx.RLock()
	if s.closed {
		s.mtx.RUnlock()
		return zero, s.closedErr
	}
	if s.coveredLocked() {
		value := s.value
		s.mtx.RUnlock()
		return value, nil
	}
	s.mtx.RUnlock()

	// Invalidation mutates, so the read lock has to be exchanged for the write
	// one, and everything checked again: another reader may have invalidated
	// already, or a successful event may have arrived in between.
	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		return zero, s.closedErr
	}
	if s.coveredLocked() {
		value := s.value
		s.mtx.Unlock()
		return value, nil
	}
	value, release := s.reconcile(nil)
	s.value = value
	err := s.err
	s.mtx.Unlock()
	s.release(release)
	return zero, err
}

// coveredLocked reports whether the published value still stands: either
// discovery is healthy, or its error is still inside the grace period.
func (s *State[T]) coveredLocked() bool {
	return s.err == nil || s.now().Before(s.deadline)
}

// Close stops accepting events and hands back the release for everything the
// last value owned, so the caller decides what to do with its error: an endpoint
// cache reports it, a snapshot holder has none to report. Close returns nil on a
// second call.
//
// It does not stop a Feed; the consumer owns the order, and the pump has to be
// joined before the state is closed or an in-flight event would be dropped
// silently instead of arriving before the shutdown.
func (s *State[T]) Close() func() error {
	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		return nil
	}
	s.closed = true
	value, release := s.reconcile(nil)
	s.value = value
	s.err = s.closedErr
	s.mtx.Unlock()
	return release
}

func (s *State[T]) release(release func() error) {
	if release == nil {
		return
	}
	if err := release(); err != nil {
		s.logger.Warn("release retired discovery resources", "err", err)
	}
}

// Feed pumps an Instancer's events into an update function.
type Feed struct {
	instancer sd.Instancer
	events    chan sd.Event
	done      chan struct{}
	wg        sync.WaitGroup
	stopOnce  sync.Once
}

// Start registers with instancer and applies its initial snapshot before
// returning, so a caller holding a subscription already has instances — or the
// error explaining why it does not. Later events arrive on a goroutine that Stop
// joins.
//
// Start panics on a nil Instancer, which is a programming error rather than a
// runtime condition.
func Start(instancer sd.Instancer, update func(sd.Event)) *Feed {
	if instancer == nil {
		panic("subscription: nil instancer")
	}
	feed := &Feed{
		instancer: instancer,
		events:    make(chan sd.Event, 1),
		done:      make(chan struct{}),
	}
	update(instancer.Register(feed.events))

	feed.wg.Add(1)
	go func() {
		defer feed.wg.Done()
		for {
			select {
			case event, ok := <-feed.events:
				// A provider must not close a channel it was handed (see
				// sd.Instancer), but this loop is not the place to spin if one
				// does.
				if !ok {
					return
				}
				update(event)
			case <-feed.done:
				return
			}
		}
	}()
	return feed
}

// Stop deregisters from the Instancer and joins the pump, so no update runs
// after it returns. It is idempotent.
//
// It does not close the Instancer: one Instancer commonly serves several
// subscriptions, and closing it stays with whoever created it.
func (f *Feed) Stop() {
	f.stopOnce.Do(func() {
		f.instancer.Deregister(f.events)
		close(f.done)
		f.wg.Wait()
	})
}
