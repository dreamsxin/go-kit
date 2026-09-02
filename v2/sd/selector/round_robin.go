package selector

import (
	"context"
	"sync/atomic"

	"github.com/dreamsxin/go-kit/v2/sd"
)

// RoundRobin walks the snapshot in order, one step per selection. It is the
// default: with uniform instances and a single client it spreads calls exactly
// evenly and costs one atomic add.
func RoundRobin() Strategy {
	return &roundRobin{}
}

type roundRobin struct {
	next uint64
}

func (r *roundRobin) Pick(_ context.Context, _ any, instances []sd.Instance) (int, sd.Done, error) {
	if len(instances) == 0 {
		return 0, nil, sd.ErrNoEndpoints
	}
	index := atomic.AddUint64(&r.next, 1) - 1
	return int(index % uint64(len(instances))), nil, nil
}
