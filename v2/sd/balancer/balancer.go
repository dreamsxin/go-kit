// Package balancer provides service-discovery balancing strategies.
package balancer

import (
	"context"
	"sync"

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
}

func (b *strategyBalancer) Pick(ctx context.Context, request any) (sd.Picked, error) {
	items, err := b.source.InstanceEndpoints()
	if err != nil {
		return sd.Picked{}, err
	}
	instances := instancesOf(items)
	index, strategyDone, err := b.strategy.Pick(ctx, request, instances)
	if err != nil {
		return sd.Picked{}, err
	}
	if index < 0 || index >= len(items) {
		return sd.Picked{}, sd.ErrNoEndpoints
	}

	// A callback is always returned, even when a strategy has no feedback
	// state, so callers can unconditionally defer picked.Done(outcome).
	var once sync.Once
	done := func(outcome sd.Outcome) {
		once.Do(func() {
			if strategyDone != nil {
				strategyDone(outcome)
			}
		})
	}
	return sd.Picked{
		Instance: items[index].Instance,
		Endpoint: items[index].Endpoint,
		Done:     done,
	}, nil
}

func (b *strategyBalancer) Close() error {
	b.closeOnce.Do(func() {
		b.closeErr = selector.CloseStrategy(b.strategy)
	})
	return b.closeErr
}

// instancesOf projects the instances of one snapshot for the strategy. Both
// the snapshot and this projection are per-selection copies, so a strategy can
// never be handed a view that a concurrent discovery update rewrites under it.
func instancesOf(items []endpointer.InstanceEndpoint) []sd.Instance {
	instances := make([]sd.Instance, len(items))
	for i, item := range items {
		instances[i] = item.Instance
	}
	return instances
}
