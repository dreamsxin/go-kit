package selector

import (
	"context"

	"github.com/dreamsxin/go-kit/v2/sd"
)

// Filtered narrows the candidate set before strategy picks from it, and maps
// the choice back onto the caller's snapshot.
//
// This is where dynamic policies belong. sd/endpointer.Filter builds a fixed
// view of an endpoint set and suits static labels — zone, version, tenant — that
// only change when discovery changes. A passive health filter changes with every
// measurement, so it has to run per selection instead, and it has to see the
// whole candidate set at once to honour an ejection cap. Filters run in order,
// so cheap label matching should precede expensive measurement policies.
//
// Filtered panics on a nil strategy. Nil filters are skipped, which keeps
// conditional assembly — one filter only in production, say — from needing a
// slice built by hand.
func Filtered(strategy Strategy, filters ...sd.InstanceFilter) Strategy {
	if strategy == nil {
		panic("selector: nil strategy")
	}
	return filtered{strategy: strategy, filters: filters}
}

type filtered struct {
	strategy Strategy
	filters  []sd.InstanceFilter
}

func (f filtered) Pick(ctx context.Context, request any, instances []sd.Instance) (int, sd.Done, error) {
	candidates := instances
	for _, filter := range f.filters {
		if filter == nil {
			continue
		}
		candidates = filter(ctx, candidates)
		if len(candidates) == 0 {
			return 0, nil, sd.ErrNoEndpoints
		}
	}

	index, done, err := f.strategy.Pick(ctx, request, candidates)
	if err != nil {
		return 0, nil, err
	}
	if index < 0 || index >= len(candidates) {
		return 0, nil, sd.ErrNoEndpoints
	}

	// The strategy indexed the narrowed slice; the caller indexes its own.
	// Addresses are the identity of an instance within one snapshot, so they
	// are what translates between the two.
	address := candidates[index].Address
	for i := range instances {
		if instances[i].Address == address {
			return i, done, nil
		}
	}
	// A filter returned something that was never a candidate. Refusing beats
	// dialling an address the caller never discovered.
	return 0, nil, sd.ErrNoEndpoints
}
