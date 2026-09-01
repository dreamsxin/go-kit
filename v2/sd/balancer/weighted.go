package balancer

import (
	"math/rand/v2"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
)

// WeightFunc reports the relative capacity of a discovered instance. Instances
// weighted zero or below are never selected, which is how a caller drains one
// without waiting for service discovery to withdraw it.
type WeightFunc func(instance string) int

// NewWeightedRandom selects an instance with probability proportional to its
// weight. Selection is stateless, so a changing instance set never leaves the
// distribution stuck mid-cycle the way a weighted round-robin counter would.
// When every eligible weight is zero or below, Endpoint reports
// sd.ErrNoEndpoints: endpoints exist but none is selectable.
//
// NewWeightedRandom panics on a nil weight function, which is a programming
// error rather than a runtime condition.
func NewWeightedRandom(source endpointer.InstanceEndpointer, weight WeightFunc) sd.Balancer {
	if weight == nil {
		panic("balancer: nil weight function")
	}
	return &weightedRandom{source: source, weight: weight}
}

type weightedRandom struct {
	source endpointer.InstanceEndpointer
	weight WeightFunc
}

func (w *weightedRandom) Endpoint() (endpoint.Endpoint, error) {
	instances, err := w.source.InstanceEndpoints()
	if err != nil {
		return nil, err
	}
	if len(instances) == 0 {
		return nil, sd.ErrNoEndpoints
	}

	// Weights are read once per selection so a weight function backed by
	// changing metadata cannot make the running total disagree with the scan
	// that follows it.
	weights := make([]int, len(instances))
	total := 0
	for i, item := range instances {
		if weight := w.weight(item.Instance); weight > 0 {
			weights[i] = weight
			total += weight
		}
	}
	if total <= 0 {
		return nil, sd.ErrNoEndpoints
	}

	target := rand.N(total)
	for i, weight := range weights {
		if weight <= 0 {
			continue
		}
		if target < weight {
			return instances[i].Endpoint, nil
		}
		target -= weight
	}
	return nil, sd.ErrNoEndpoints
}
