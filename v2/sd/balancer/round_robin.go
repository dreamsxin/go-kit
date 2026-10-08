package balancer

import (
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

// NewRoundRobin distributes picks over the current endpoint snapshot.
//
// Stable: balancer.round-robin — picks cycle through the snapshot in order,
// repeating from the start; an empty snapshot reports sd.ErrNoEndpoints.
// Covered by: TestRoundRobin_DistributesEvenly, TestRoundRobin_ThreeEndpoints_Cycles, TestRoundRobin_NoEndpoints, TestRoundRobin_SingleEndpoint
func NewRoundRobin(source endpointer.InstanceEndpointer) sd.Balancer {
	return New(source, selector.RoundRobin())
}
