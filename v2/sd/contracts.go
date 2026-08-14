// Package sd defines protocol-neutral service-discovery contracts.
package sd

import (
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

// ErrNoEndpoints indicates that a balancer currently has no endpoint to select.
var ErrNoEndpoints = errors.New("no endpoints available")
