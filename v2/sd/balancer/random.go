package balancer

import (
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

// NewRandom picks a uniformly random endpoint from the current snapshot.
//
// Stable: balancer.random — picks are drawn only from the current snapshot and
// reach every endpoint over time; an empty snapshot reports sd.ErrNoEndpoints.
// Covered by: TestRandom_ReachesEveryEndpoint, TestRandom_SelectsOnlyKnownEndpoints, TestRandom_NoEndpoints, TestRandom_SingleEndpoint
func NewRandom(source endpointer.InstanceEndpointer) sd.Balancer {
	return New(source, selector.Random())
}
