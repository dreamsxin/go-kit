package selector

import (
	"errors"
	"log/slog"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/internal/subscription"
)

// ErrClosed is returned by a Subscription after it has been closed.
var ErrClosed = errors.New("selector: subscription closed")

// Static returns a Source over a fixed instance set, for tests, local
// development, and configuration-driven pools that never change at runtime.
func Static(instances ...sd.Instance) Source {
	fixed := append([]sd.Instance(nil), instances...)
	subscription.SortInstances(fixed)
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
// The subscribe, error-grace, and invalidation behaviour is the shared state
// machine in sd/internal/subscription, so a Subscription and an Endpointer
// answer a registry outage the same way.
//
// Close must be called to stop the background goroutine and deregister from
// the Instancer. Stopping the Instancer itself stays with whoever created it.
type Subscription struct {
	feed  *subscription.Feed
	state *subscription.State[[]sd.Instance]
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

	s := &Subscription{
		state: subscription.NewState(sortedSnapshot, ErrClosed, opts.Logger, subscription.Options{
			InvalidateOnError: opts.InvalidateOnError,
			InvalidateTimeout: opts.InvalidateTimeout,
		}),
	}
	s.feed = subscription.Start(instancer, s.state.Update)
	return s
}

// sortedSnapshot is this consumer's whole projection: a sorted copy of the
// snapshot, owning nothing that has to be released.
func sortedSnapshot(instances []sd.Instance) ([]sd.Instance, func() error) {
	sorted := append([]sd.Instance(nil), instances...)
	subscription.SortInstances(sorted)
	return sorted, nil
}

// Instances implements Source. It reports the last good snapshot while a
// discovery error is within its grace period, and the error itself once the
// snapshot has been invalidated.
//
// The result is a copy, so a caller is free to sort or filter it in place.
func (s *Subscription) Instances() ([]sd.Instance, error) {
	instances, err := s.state.Value()
	if err != nil {
		return nil, err
	}
	return append([]sd.Instance(nil), instances...), nil
}

// Close deregisters from the Instancer and stops the background goroutine.
//
// The pump is joined before the state closes, so an event already in flight is
// applied rather than dropped. A snapshot owns nothing, so the state's release
// is always nil and there is no error to report.
func (s *Subscription) Close() error {
	s.feed.Stop()
	s.state.Close()
	return nil
}
