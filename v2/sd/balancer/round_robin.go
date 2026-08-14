// Package balancer provides service-discovery balancing strategies.
package balancer

import (
	"sync/atomic"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
)

// NewRoundRobin distributes calls over the current endpoint snapshot.
func NewRoundRobin(source endpointer.Endpointer) sd.Balancer {
	return &roundRobin{source: source}
}

type roundRobin struct {
	source endpointer.Endpointer
	next   uint64
}

func (r *roundRobin) Endpoint() (endpoint.Endpoint, error) {
	endpoints, err := r.source.Endpoints()
	if err != nil {
		return nil, err
	}
	if len(endpoints) == 0 {
		return nil, sd.ErrNoEndpoints
	}
	index := atomic.AddUint64(&r.next, 1) - 1
	return endpoints[index%uint64(len(endpoints))], nil
}
