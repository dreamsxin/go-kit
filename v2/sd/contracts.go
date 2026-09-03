// Package sd defines protocol-neutral service-discovery contracts.
package sd

import (
	"context"
	"errors"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// Instance is one discovered service instance: an address plus the labels the
// instance reported when it registered. It is an alias so providers can build
// snapshots without importing this module.
//
// Metadata carries *static* labels — zone, version, protocol, capability,
// weight, tenant — the kind of thing a registry is designed to hold. It is not
// a channel for live load signals: a registry write per metric sample would
// hammer the catalog, and every consumer would read stale numbers. Balancers
// that need live signals measure them in process; see sd/balancer.
type Instance = struct {
	Address  string
	Metadata map[string]any
}

// Event is a snapshot of the currently healthy service instances. It is an
// alias so providers can implement Instancer without importing this module.
// When Err is non-nil, consumers may continue using the previous snapshot for
// a grace period before invalidating it.
//
// A published Event belongs to its consumers. A provider must not mutate
// Instances or any Metadata map after handing the event over, and must not reuse
// the backing array for the next watch: consumers such as sd/health keep the
// snapshot between probe rounds, and a filter may hold a subset of it. Build a
// fresh slice per event. Consumers that pass an event on to another component
// are, by the same rule, free to hand over what they were given.
type Event = struct {
	Instances []Instance
	Err       error
}

// Addresses builds a snapshot from bare addresses, for tests, local
// development, and registries that expose no labels.
func Addresses(addresses ...string) []Instance {
	instances := make([]Instance, len(addresses))
	for i, address := range addresses {
		instances[i] = Instance{Address: address}
	}
	return instances
}

// Instancer publishes service-discovery snapshots and owns the provider
// lifecycle. Close is idempotent and releases watches and other resources.
type Instancer interface {
	Register(chan Event) Event
	Deregister(chan Event)
	Close() error
}

// Registrar registers and deregisters one service instance.
type Registrar interface {
	Register() error
	Deregister() error
}

// Outcome is the result of one call made through a Picked endpoint.
// Latency includes endpoint execution time, not time spent waiting to pick an
// instance.
//
// Bytes is the application-defined volume the call moved. sd/retry does not fill
// it, because an endpoint's request and response are opaque values at that
// layer; a caller that knows the protocol — a proxy counting the bytes it
// relayed, a transport reading a content length — reports it by wrapping
// Picked.Done. Zero means nobody measured it.
type Outcome struct {
	Err     error
	Latency time.Duration
	Bytes   int64
}

// Done feeds the result of a picked call back to the selector. It is safe for
// implementations to make a Done function idempotent; callers invoke it once
// for every successful Pick, including calls that return an error.
type Done func(Outcome)

// Picked is the endpoint and identity selected for one attempt. The caller
// must invoke Done after the endpoint returns, even when it returns an error.
type Picked struct {
	Instance Instance
	Endpoint endpoint.Endpoint
	Done     Done
}

// Balancer selects an endpoint from a dynamic endpoint set and returns the
// identity needed to feed the call result back into its strategy.
//
// Implementations must be safe for concurrent use: one Balancer is shared by
// every caller of the endpoint it backs.
type Balancer interface {
	Pick(ctx context.Context, request any) (Picked, error)
	Close() error
}

// ErrNoEndpoints indicates that a balancer currently has no endpoint to select.
var ErrNoEndpoints = errors.New("no endpoints available")
