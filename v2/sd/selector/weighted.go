package selector

import (
	"context"
	"math"
	"math/rand/v2"

	"github.com/dreamsxin/go-kit/v2/sd"
)

// WeightFunc reports the relative capacity of an instance. Instances weighted
// zero or below are never selected, which is how a caller drains one without
// waiting for service discovery to withdraw it.
type WeightFunc func(instance sd.Instance) int

// DefaultWeightKey is the metadata label MetadataWeight reads. It matches the
// convention used by Envoy and Nomad for a per-endpoint load-balancing weight.
const DefaultWeightKey = "weight"

// MetadataWeight reads the weight an instance reported when it registered,
// falling back to fallback when the label is absent or unparsable. Registries
// deliver labels as strings, so "10" and 10 both work.
func MetadataWeight(key string, fallback int) WeightFunc {
	if key == "" {
		key = DefaultWeightKey
	}
	return func(instance sd.Instance) int {
		if weight, ok := sd.MetadataInt(instance.Metadata, key); ok {
			return weight
		}
		return fallback
	}
}

// WeightedRandom selects with probability proportional to weight. Selection is
// stateless, so a changing instance set never leaves the distribution stuck
// mid-cycle the way a weighted round-robin counter would.
//
// When every weight is zero or below, Pick reports sd.ErrNoEndpoints:
// instances exist but none is selectable, which the default retry classifier
// treats as temporary.
//
// WeightedRandom panics on a nil weight function, which is a programming error
// rather than a runtime condition.
func WeightedRandom(weight WeightFunc) Strategy {
	if weight == nil {
		panic("selector: nil weight function")
	}
	return weighted{weight: weight}
}

type weighted struct {
	weight WeightFunc
}

func (w weighted) Pick(_ context.Context, _ any, instances []sd.Instance) (int, sd.Done, error) {
	if len(instances) == 0 {
		return 0, nil, sd.ErrNoEndpoints
	}

	// Weights are read once per selection so a weight function backed by
	// changing metadata cannot make the running total disagree with the scan
	// that follows it.
	//
	// Each weight is clamped so the total cannot overflow. A weight is deployment
	// data — it arrives as a metadata string and sd.MetadataInt accepts any int —
	// so two instances registered with a weight near MaxInt is a configuration
	// mistake, not an impossibility. Summing them wrapped negative, the guard below
	// fired, and a fully healthy pool became unroutable through ErrNoEndpoints,
	// which callers classify as temporary and retry against until their budget is
	// gone. Wrapping to a smaller positive total was quieter and worse: the
	// distribution stopped resembling the configuration with nothing to notice.
	//
	// Stable: sd.selector-weights-cannot-overflow-the-total — a weighted pick treats absurdly large weights as large rather than letting their sum wrap and report that no endpoint is selectable.
	// Covered by: TestWeightedRandom_HugeWeightsStillSelect
	maxWeight := math.MaxInt / len(instances)
	weights := make([]int, len(instances))
	total := 0
	for i, instance := range instances {
		weight := w.weight(instance)
		if weight <= 0 {
			continue
		}
		if weight > maxWeight {
			weight = maxWeight
		}
		weights[i] = weight
		total += weight
	}
	if total <= 0 {
		return 0, nil, sd.ErrNoEndpoints
	}

	target := rand.N(total)
	for i, weight := range weights {
		if weight <= 0 {
			continue
		}
		if target < weight {
			return i, nil, nil
		}
		target -= weight
	}
	return 0, nil, sd.ErrNoEndpoints
}
