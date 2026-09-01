package balancer

import (
	"math/rand/v2"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
)

// NewRandom picks a uniformly random endpoint from the current snapshot. It
// needs no shared counter, so it stays fair when several clients share one
// instance set and round robin would otherwise march in lockstep.
func NewRandom(source endpointer.Endpointer) sd.Balancer {
	return &random{source: source}
}

type random struct {
	source endpointer.Endpointer
}

func (r *random) Endpoint() (endpoint.Endpoint, error) {
	endpoints, err := r.source.Endpoints()
	if err != nil {
		return nil, err
	}
	if len(endpoints) == 0 {
		return nil, sd.ErrNoEndpoints
	}
	return endpoints[rand.N(len(endpoints))], nil
}
