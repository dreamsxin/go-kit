package balancer

import (
	"sync/atomic"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/sd/interfaces"

	"github.com/dreamsxin/go-kit/sd/endpointer"
)

// NewRoundRobin returns a Balancer that distributes calls across the
// Endpoints returned by s in round-robin order using a lock-free counter.
func NewRoundRobin(s endpointer.Endpointer) interfaces.Balancer {
	return &roundRobin{
		s: s,
		c: 0,
	}
}

type roundRobin struct {
	s endpointer.Endpointer
	c uint64
}

func (rr *roundRobin) Endpoint() (endpoint.Endpoint, error) {
	endpoints, err := rr.s.Endpoints()
	if err != nil {
		return nil, err
	}
	if len(endpoints) <= 0 {
		return nil, interfaces.ErrNoEndpoints
	}
	old := atomic.AddUint64(&rr.c, 1) - 1
	idx := old % uint64(len(endpoints))
	return endpoints[idx], nil
}
