// Package sd defines protocol-neutral service-discovery contracts.
package sd

import (
	"context"
	"errors"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// Event is a snapshot of the currently healthy service instances. It is an
// alias so providers can implement Instancer without importing this module.
// When Err is non-nil, consumers may continue using the previous snapshot for
// a grace period before invalidating it.
type Event = struct {
	Instances []string
	Err       error
}

// Instancer publishes service-discovery snapshots. Lifecycle remains owned by
// the concrete provider; for example, Consul callers close its Instancer with
// Stop after deregistering all subscribers.
type Instancer interface {
	Register(chan Event) Event
	Deregister(chan Event)
}

// Registrar registers and deregisters one service instance.
type Registrar interface {
	Register() error
	Deregister() error
}

// Balancer selects an endpoint from a dynamic endpoint set.
type Balancer interface {
	Endpoint() (endpoint.Endpoint, error)
}

// RequestBalancer selects an endpoint using the in-flight request. Strategies
// that key on request content, such as consistent hashing, implement it in
// addition to Balancer; executors that hold the request should prefer
// EndpointFor and fall back to Endpoint.
type RequestBalancer interface {
	Balancer
	EndpointFor(ctx context.Context, request any) (endpoint.Endpoint, error)
}

// ErrNoEndpoints indicates that a balancer currently has no endpoint to select.
var ErrNoEndpoints = errors.New("no endpoints available")
