package selector

import (
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
)

// FirstSeenFunc reports when an instance was first observed, and whether it is
// known at all. sd/feedback.Measured.SlowStartWeighted is the in-process
// implementation, and it supplies the discovery subscription that dates each
// instance — which is what a ramp needs and what a caller supplying this itself
// has to provide some other way.
type FirstSeenFunc func(instance sd.Instance) (time.Time, bool)

// SlowStart ramps an instance's weight from nothing to its full value over
// window.
//
// A freshly started instance has cold caches, an unwarmed JIT and no
// connection pool, so giving it a full share of traffic the moment it appears
// is how a deployment turns into a latency spike. Envoy and NGINX both solve
// this at the weight, which is why this is a WeightFunc decorator rather than a
// strategy: it composes with weighted random exactly as an operator-set weight
// does.
//
// An instance the FirstSeenFunc does not know is treated as brand new, so a
// FirstSeenFunc that knows nothing collapses every weight to the minimum and
// weighted selection degenerates into uniform selection. Feed it from the same
// table the balancer records into.
func SlowStart(weight WeightFunc, first FirstSeenFunc, window time.Duration) WeightFunc {
	if weight == nil {
		panic("selector: nil weight function")
	}
	if first == nil {
		panic("selector: nil first-seen function")
	}
	if window <= 0 {
		return weight
	}

	return func(instance sd.Instance) int {
		full := weight(instance)
		if full <= 0 {
			// Zero means "never pick me"; ramping would contradict that.
			return full
		}

		since, known := first(instance)
		if !known {
			return 1
		}
		elapsed := time.Since(since)
		if elapsed >= window {
			return full
		}
		if elapsed <= 0 {
			return 1
		}

		ramped := int(float64(full) * float64(elapsed) / float64(window))
		// One, not zero: a warming instance must still receive the traffic that
		// warms it.
		return max(ramped, 1)
	}
}
