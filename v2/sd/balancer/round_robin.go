package balancer

import (
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

// NewRoundRobin distributes picks over the current endpoint snapshot.
func NewRoundRobin(source endpointer.InstanceEndpointer) sd.Balancer {
	return New(source, selector.RoundRobin())
}
