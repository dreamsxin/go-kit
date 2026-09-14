package selector

import (
	"context"
	"math/rand/v2"

	"github.com/dreamsxin/go-kit/v2/sd"
)

// LoadFunc reports the current load of an instance; lower is better. It exists
// so the strategy stays independent of who measures: sd/feedback.Measured
// supplies the in-process measurement together with the accounting that
// produces it, but a caller with its own accounting can supply this instead.
type LoadFunc func(instance sd.Instance) int64

// DefaultChoices is how many candidates a least-request selection samples. Two
// is the value Envoy and gRPC use: it removes almost all of the worst-case
// imbalance of random selection without the cost of scanning every instance.
const DefaultChoices = 2

// LeastRequestOption configures LeastRequest.
type LeastRequestOption func(*leastRequest)

// WithChoices sets how many candidates each selection samples. Values below two
// fall back to DefaultChoices; a value at or above the instance count
// degenerates to an exact scan of every instance.
func WithChoices(choices int) LeastRequestOption {
	return func(l *leastRequest) {
		if choices >= 2 {
			l.choices = choices
		}
	}
}

// LeastRequest sends each call to the sampled instance carrying the least load
// — power of two choices.
//
// The strategy only reads load; something has to record it. Pair it with
// sd/feedback.Table so that picking and accounting share one measurement
// stream, most simply through Table.LeastRequest. On its own, with a load
// function that never changes, this degenerates into random selection.
func LeastRequest(load LoadFunc, options ...LeastRequestOption) Strategy {
	if load == nil {
		panic("selector: nil load function")
	}
	strategy := &leastRequest{load: load, choices: DefaultChoices}
	for _, option := range options {
		if option != nil {
			option(strategy)
		}
	}
	return strategy
}

type leastRequest struct {
	load    LoadFunc
	choices int
}

func (l *leastRequest) Pick(_ context.Context, _ any, instances []sd.Instance) (int, sd.Done, error) {
	if len(instances) == 0 {
		return 0, nil, sd.ErrNoEndpoints
	}
	// Stable: sd.selector-least-request-full-scan — when choices cover the pool, every instance is measured once and a minimum-load instance is selected, with equal minima distributed at random.
	// Covered by: TestLeastRequest_ScansTheWholePoolWhenChoicesCoverIt, TestLeastRequest_FullScanSpreadsTies
	if l.choices >= len(instances) {
		// Once choices cover the whole snapshot, sample each instance exactly
		// once. Reusing the power-of-two random path here could pick the same
		// instance repeatedly and miss the actual minimum, contradicting the
		// option's documented full-scan behavior.
		best := 0
		lowest := l.load(instances[best])
		ties := 1
		for i := 1; i < len(instances); i++ {
			load := l.load(instances[i])
			switch {
			case load < lowest:
				best, lowest, ties = i, load, 1
			case load == lowest:
				ties++
				if rand.N(ties) == 0 {
					best = i
				}
			}
		}
		return best, nil, nil
	}

	best := rand.N(len(instances))
	lowest := l.load(instances[best])
	for i := 1; i < l.choices; i++ {
		candidate := rand.N(len(instances))
		if load := l.load(instances[candidate]); load < lowest {
			best, lowest = candidate, load
		}
	}
	return best, nil, nil
}
