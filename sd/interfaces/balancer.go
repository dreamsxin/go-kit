package interfaces

import (
	"errors"

	"github.com/dreamsxin/go-kit/endpoint"
)

// Balancer selects one Endpoint from a pool of available endpoints.
// Implementations may use any strategy: round-robin, random, least-loaded, etc.
type Balancer interface {
	Endpoint() (endpoint.Endpoint, error)
}

// ErrNoEndpoints is returned by a Balancer when no endpoints are available,
// e.g. because service discovery has not yet received any instances.
var ErrNoEndpoints = errors.New("no endpoints available")
