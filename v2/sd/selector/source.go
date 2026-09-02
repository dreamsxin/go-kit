package selector

import (
	"errors"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
)

// ErrClosed is returned by a Subscription after it has been closed.
var ErrClosed = errors.New("selector: subscription closed")

// Static returns a Source over a fixed instance set, for tests, local
// development, and configuration-driven pools that never change at runtime.
func Static(instances ...sd.Instance) Source {
	fixed := append([]sd.Instance(nil), instances...)
	sortInstances(fixed)
	return SourceFunc(func() ([]sd.Instance, error) {
		return append([]sd.Instance(nil), fixed...), nil
	})
}

// Filter narrows a source to the instances a match accepts. Nothing matching
// means an empty snapshot, so selection reports sd.ErrNoEndpoints rather than
// falling back to an instance the caller ruled out.
//
// Filter panics on a nil match, which is a programming error rather than a
// runtime condition.
func Filter(source Source, match sd.Match) Source {
	if source == nil {
		panic("selector: nil source")
	}
	if match == nil {
		panic("selector: nil match")
	}
	return SourceFunc(func() ([]sd.Instance, error) {
		instances, err := source.Instances()
		if err != nil {
			return nil, err
		}
		return matching(instances, match), nil
	})
}

// Prefer narrows a source but falls back to the full snapshot when nothing
// matches. This is how zone affinity degrades: stay local while local
// instances exist, spill over rather than fail.
func Prefer(source Source, match sd.Match) Source {
	if source == nil {
		panic("selector: nil source")
	}
	if match == nil {
		panic("selector: nil match")
	}
	return SourceFunc(func() ([]sd.Instance, error) {
		instances, err := source.Instances()
		if err != nil {
			return nil, err
		}
		if matched := matching(instances, match); len(matched) > 0 {
			return matched, nil
		}
		return instances, nil
	})
}

func matching(instances []sd.Instance, match sd.Match) []sd.Instance {
	matched := make([]sd.Instance, 0, len(instances))
	for _, instance := range instances {
		if match(instance) {
			matched = append(matched, instance)
		}
	}
	return matched
}

// Options controls how a Subscription reacts to service-discovery errors.
type Options struct {
	InvalidateOnError bool
	InvalidateTimeout time.Duration
	Logger            *slog.Logger
}

// Option configures a Subscription.
type Option func(*Options)

// InvalidateOnError drops the cached snapshot after timeout has elapsed from
// the first service-discovery error. Without it a Subscription serves the last
// good snapshot indefinitely, which is the right default for a registry
// outage: instances that are still up should keep receiving traffic.
func InvalidateOnError(timeout time.Duration) Option {
	return func(options *Options) {
		options.InvalidateOnError = true
		options.InvalidateTimeout = timeout
	}
}

// WithLogger sets the logger a Subscription reports discovery errors to.
func WithLogger(logger *slog.Logger) Option {
	return func(options *Options) {
		options.Logger = logger
	}
}

// Subscription keeps the latest snapshot of an Instancer. It is the Source for
// callers that select instances without building endpoints; callers that do
// build endpoints use sd/endpointer instead, which owns the factory lifecycle.
//
// Close must be called to stop the background goroutine and deregister from
// the Instancer. Stopping the Instancer itself stays with whoever created it.
type Subscription struct {
	instancer sd.Instancer
	options   Options
	logger    *slog.Logger
	ch        chan sd.Event
	done      chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once

	mtx       sync.RWMutex
	instances []sd.Instance
	err       error
	deadline  time.Time
	timeNow   func() time.Time
	closed    bool
}

// Subscribe registers with an Instancer and tracks its snapshots.
//
// Subscribe panics on a nil Instancer, which is a programming error rather
// than a runtime condition.
func Subscribe(instancer sd.Instancer, options ...Option) *Subscription {
	if instancer == nil {
		panic("selector: nil instancer")
	}
	opts := Options{}
	for _, option := range options {
		option(&opts)
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	s := &Subscription{
		instancer: instancer,
		options:   opts,
		logger:    logger,
		ch:        make(chan sd.Event, 1),
		done:      make(chan struct{}),
		timeNow:   time.Now,
	}
	s.update(instancer.Register(s.ch))
	s.wg.Add(1)
	go s.receive()
	return s
}

func (s *Subscription) receive() {
	defer s.wg.Done()
	for {
		select {
		case event, ok := <-s.ch:
			if !ok {
				return
			}
			s.update(event)
		case <-s.done:
			return
		}
	}
}

func (s *Subscription) update(event sd.Event) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.closed {
		return
	}

	if event.Err == nil {
		instances := append([]sd.Instance(nil), event.Instances...)
		sortInstances(instances)
		s.instances = instances
		s.err = nil
		return
	}

	s.logger.Debug("service discovery update failed", "err", event.Err)
	if !s.options.InvalidateOnError || s.err != nil {
		return
	}
	s.err = event.Err
	s.deadline = s.timeNow().Add(s.options.InvalidateTimeout)
}

// Instances implements Source. It reports the last good snapshot while a
// discovery error is within its grace period, and the error itself once the
// snapshot has been invalidated.
func (s *Subscription) Instances() ([]sd.Instance, error) {
	s.mtx.RLock()
	if s.closed {
		s.mtx.RUnlock()
		return nil, ErrClosed
	}
	if s.err == nil || s.timeNow().Before(s.deadline) {
		instances := append([]sd.Instance(nil), s.instances...)
		s.mtx.RUnlock()
		return instances, nil
	}
	s.mtx.RUnlock()

	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	if s.err == nil || s.timeNow().Before(s.deadline) {
		return append([]sd.Instance(nil), s.instances...), nil
	}
	s.instances = nil
	return nil, s.err
}

// Close deregisters from the Instancer and stops the background goroutine.
func (s *Subscription) Close() error {
	s.closeOnce.Do(func() {
		s.instancer.Deregister(s.ch)
		close(s.done)
		s.wg.Wait()

		s.mtx.Lock()
		s.closed = true
		s.instances = nil
		s.err = ErrClosed
		s.mtx.Unlock()
	})
	return nil
}

// sortInstances orders a snapshot by address so that round robin walks a
// stable sequence instead of following whatever order the registry replied in.
func sortInstances(instances []sd.Instance) {
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].Address < instances[j].Address
	})
}
