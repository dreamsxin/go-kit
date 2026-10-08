package balancer

import (
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

// WeightFunc reports the relative capacity of a discovered instance. Instances
// weighted zero or below are never selected, which is how a caller drains one
// without waiting for service discovery to withdraw it.
type WeightFunc = selector.WeightFunc

// DefaultWeightKey is the metadata label MetadataWeight reads. It matches the
// convention used by Envoy and Nomad for a per-endpoint load-balancing weight.
const DefaultWeightKey = selector.DefaultWeightKey

// MetadataWeight reads the weight an instance reported when it registered,
// falling back to fallback when the label is absent or unparsable. Registries
// deliver labels as strings, so "10" and 10 both work.
func MetadataWeight(key string, fallback int) WeightFunc {
	return selector.MetadataWeight(key, fallback)
}

// NewWeightedRandom selects an instance with probability proportional to its
// weight. Selection is stateless, so a changing instance set never leaves the
// distribution stuck mid-cycle the way a weighted round-robin counter would.
// When every eligible weight is zero or below, Endpoint reports
// sd.ErrNoEndpoints: endpoints exist but none is selectable.
//
// NewWeightedRandom panics on a nil weight function, which is a programming
// error rather than a runtime condition.
//
// Stable: balancer.weighted-random — an instance is selected with probability
// proportional to its weight; a weight of zero or below is never selected.
// Covered by: TestWeightedRandom_DistributesProportionally, TestWeightedRandom_SkipsNonPositiveWeights, TestWeightedRandom_NegativeWeightIsExcluded
//
// Stable: balancer.weighted-all-zero — when every eligible weight is zero or
// below, Pick reports sd.ErrNoEndpoints even though endpoints exist.
// Covered by: TestWeightedRandom_AllWeightsZero
//
// Stable: balancer.weighted-nil-weight-panic — NewWeightedRandom panics on a
// nil weight function.
// Covered by: TestWeightedRandom_NilWeightFunctionPanics
//
// Stable: balancer.metadata-weight — MetadataWeight reads a registered weight
// where registry strings and numbers both parse, falling back when the label
// is absent or unparsable.
// Covered by: TestMetadataWeight
func NewWeightedRandom(source endpointer.InstanceEndpointer, weight WeightFunc) sd.Balancer {
	return New(source, selector.WeightedRandom(weight))
}
