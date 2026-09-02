package selector

import (
	"context"
	"math/rand/v2"

	"github.com/dreamsxin/go-kit/v2/sd"
)

// Random draws uniformly from the snapshot. It keeps no counter, so it stays
// fair when many clients share one instance set and round robin would march
// them in lockstep onto the same instance.
func Random() Strategy {
	return random{}
}

type random struct{}

func (random) Pick(_ context.Context, _ any, instances []sd.Instance) (int, sd.Done, error) {
	if len(instances) == 0 {
		return 0, nil, sd.ErrNoEndpoints
	}
	return rand.N(len(instances)), nil, nil
}
