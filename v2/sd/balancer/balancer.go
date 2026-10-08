// Package balancer turns a selection strategy into something callable.
//
// The split with sd/selector is the point of this package existing separately. A
// selector.Strategy is a pure decision: given instances, choose one. A Balancer
// holds the live endpoint set, asks the strategy, and hands back an sd.Picked —
// the endpoint to call together with the strategy's result callback. That is why
// a strategy needs no knowledge of discovery and a caller needs no knowledge of
// the strategy.
//
// Calling Done on the returned sd.Picked is how a strategy learns anything. In
// this package's terms it is optional; in practice, least-request, weighted and
// feedback-driven strategies are only correct if every pick reports its outcome,
// so sd/retry does it for you and a direct caller must do it itself.
//
// New does not close the endpoint source. One endpoint set is commonly shared by
// several balancers, so its owner stays responsible for closing it — closing the
// balancer must not take the set out from under the others.
//
// The strategies here are the ready-made ones. To write your own, implement
// selector.Strategy and pass it to New: nothing in this package needs to change.
package balancer

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

// New turns any selector.Strategy into a Balancer over an endpoint set. The
// selected instance and the strategy's result callback are preserved in the
// returned sd.Picked value, so retry and direct callers can feed outcomes back
// without a second request-aware interface.
//
// New does not close source: endpoint sets are commonly shared by multiple
// balancers and their owner remains responsible for closing the endpointer.
//
// Stable: balancer.custom-strategy — New turns any selector.Strategy into a
// Balancer over the live endpoint set, and the request reaches the strategy,
// so a keyed strategy keeps its affinity without a second interface.
// Covered by: TestNew_AppliesCustomStrategy, TestNewPassesRequestToStrategy
//
// Stable: balancer.nil-arguments-panic — New panics on a nil endpoint source
// or nil strategy.
// Covered by: TestNew_NilArgumentsPanic
func New(source endpointer.InstanceEndpointer, strategy selector.Strategy) sd.Balancer {
	if source == nil {
		panic("balancer: nil endpoint source")
	}
	if strategy == nil {
		panic("balancer: nil strategy")
	}
	return &strategyBalancer{source: source, strategy: strategy}
}

type strategyBalancer struct {
	source    endpointer.InstanceEndpointer
	strategy  selector.Strategy
	closeOnce sync.Once
	closeErr  error
	closed    atomic.Bool
}

func (b *strategyBalancer) Pick(ctx context.Context, request any) (sd.Picked, error) {
	// Stable: balancer.closed-pick — Pick after Close reports sd.ErrClosed.
	// Covered by: TestNew_CloseRejectsSubsequentPicks
	//
	// Stable: balancer.source-error — an endpoint source error is returned
	// from Pick as-is, not swallowed into a selection failure.
	// Covered by: TestWeightedRandom_PropagatesSourceError, TestConsistentHash_PropagatesSourceError, TestRandom_PropagatesSourceError
	if b.closed.Load() {
		return sd.Picked{}, sd.ErrClosed
	}
	items, err := b.source.InstanceEndpoints()
	if err != nil {
		return sd.Picked{}, err
	}
	if b.closed.Load() {
		return sd.Picked{}, sd.ErrClosed
	}
	instances := instancesOf(items)
	index, strategyDone, err := b.strategy.Pick(ctx, request, instances)
	if err != nil {
		return sd.Picked{}, err
	}
	if index < 0 || index >= len(items) {
		sd.Release(strategyDone, sd.ErrNoEndpoints)
		return sd.Picked{}, sd.ErrNoEndpoints
	}

	return sd.Picked{
		Instance: items[index].Instance,
		Endpoint: items[index].Endpoint,
		Done:     guardDone(strategyDone),
	}, nil
}

// guardDone returns the callback handed to the caller. A callback is always
// returned, even when a strategy keeps no feedback state, so callers can
// unconditionally defer picked.Done(outcome) — and a strategy without state
// costs nothing, because the no-op is shared.
//
// The guard is defensive: sd.Done asks callers to invoke it once per successful
// Pick, and a strategy that reserved an in-flight slot would have it released
// twice by a caller that got that wrong.
//
// Stable: balancer.done-forwards-outcome — the Done on a returned Picked
// forwards the outcome to the strategy that reserved the pick.
// Covered by: TestNew_ForwardsPickedDone
func guardDone(strategyDone sd.Done) sd.Done {
	if strategyDone == nil {
		return discardOutcome
	}
	return (&onceDone{strategy: strategyDone}).report
}

func discardOutcome(sd.Outcome) {}

type onceDone struct {
	strategy sd.Done
	reported atomic.Bool
}

func (d *onceDone) report(outcome sd.Outcome) {
	if d.reported.Swap(true) {
		return
	}
	d.strategy(outcome)
}

func (b *strategyBalancer) Close() error {
	// Stable: balancer.close-releases-strategy — Close releases a closable
	// strategy exactly once, through decorators, and a repeated Close is
	// idempotent.
	// Covered by: TestNew_CloseReleasesClosableStrategy, TestNew_CloseReachesStrategyThroughDecorators
	b.closeOnce.Do(func() {
		b.closed.Store(true)
		b.closeErr = selector.CloseStrategy(b.strategy)
	})
	return b.closeErr
}

// instancesOf projects the instances of one snapshot for the strategy. The
// projection is per-selection, so a strategy can never be handed a view that a
// concurrent discovery update rewrites under it: the snapshot itself is
// published whole and never edited in place.
func instancesOf(items []endpointer.InstanceEndpoint) []sd.Instance {
	instances := make([]sd.Instance, len(items))
	for i, item := range items {
		instances[i] = item.Instance
	}
	return instances
}
