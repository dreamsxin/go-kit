package balancer

import (
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

// NewRandom picks a uniformly random endpoint from the current snapshot.
func NewRandom(source endpointer.InstanceEndpointer) sd.Balancer {
	return New(source, selector.Random())
}
