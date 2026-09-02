// Package selector picks one instance out of a service-discovery snapshot.
//
// It is the layer below sd/balancer. A Strategy decides which instance of a
// snapshot to use and knows nothing about endpoints, which serves two
// audiences from one implementation:
//
//   - Callers that issue the request assemble sd/balancer — these strategies
//     plus endpoint lookup.
//   - Callers that only need an address assemble Instancer → Source →
//     Selector. A proxy that dials the instance itself, or an API that answers
//     "where should I connect?", never builds an endpoint, so no factory runs
//     and no connection is created for an instance nobody calls.
//
// Every strategy lives here, including the ones that need to observe the calls
// they selected: LeastRequest reads a load function and sd/feedback.Table
// supplies both that function and the accounting behind it. Signals that arrive
// out of band — a push report, ORCA, your own metrics table — belong in Scored,
// and their staleness is a property of that reporting channel, not of this
// package.
package selector

import (
	"context"

	"github.com/dreamsxin/go-kit/v2/sd"
)

// Source supplies the current instance snapshot. sd/instance.Cache and every
// provider Instancer can back one through Subscribe; Static covers fixed sets.
type Source interface {
	Instances() ([]sd.Instance, error)
}

// SourceFunc adapts a function to Source.
type SourceFunc func() ([]sd.Instance, error)

// Instances implements Source.
func (f SourceFunc) Instances() ([]sd.Instance, error) { return f() }

// Strategy picks the index of the instance to use out of a snapshot and may
// return a callback that receives the outcome of the call. The request is
// always passed, even to strategies that do not inspect it; this keeps one
// obvious extension point for keyed strategies.
//
// The snapshot is passed in rather than read by the strategy so that
// sd/balancer can apply these strategies to an endpoint set it has already
// read, and so a stateful strategy can never pick against a snapshot its
// caller did not see.
//
// Implementations must be safe for concurrent use: one Strategy is shared by
// every caller of the selector or balancer it backs. Pick reports
// sd.ErrNoEndpoints when nothing is selectable, an empty snapshot included.
type Strategy interface {
	Pick(ctx context.Context, request any, instances []sd.Instance) (index int, done sd.Done, err error)
}

// Selector reports which instance to use next.
type Selector interface {
	Select(ctx context.Context, request any) (sd.Instance, error)
}

// New binds a strategy to a source. The request is always passed through to
// the strategy, so keyed and unkeyed strategies share one contract.
//
// New panics on a nil source or strategy, which is a programming error rather
// than a runtime condition.
func New(source Source, strategy Strategy) Selector {
	if source == nil {
		panic("selector: nil source")
	}
	if strategy == nil {
		panic("selector: nil strategy")
	}
	return &bound{source: source, strategy: strategy}
}

// Select is a convenience helper for callers that do not need to retain the
// concrete selector.
func Select(ctx context.Context, selector Selector, request any) (sd.Instance, error) {
	if selector == nil {
		return sd.Instance{}, sd.ErrNoEndpoints
	}
	return selector.Select(ctx, request)
}

type bound struct {
	source   Source
	strategy Strategy
}

func (b *bound) Select(ctx context.Context, request any) (sd.Instance, error) {
	instances, err := b.source.Instances()
	if err != nil {
		return sd.Instance{}, err
	}
	index, _, err := b.strategy.Pick(ctx, request, instances)
	if err != nil {
		return sd.Instance{}, err
	}
	if index < 0 || index >= len(instances) {
		return sd.Instance{}, sd.ErrNoEndpoints
	}
	return instances[index], nil
}
