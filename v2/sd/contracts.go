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
// Address is both the dial target and the identity of an instance. Every
// component that keeps per-instance state — endpoint caches, feedback tables,
// ejectors, health state — keys on it, so a provider must publish a non-empty
// address, unique within one snapshot. Duplicates are dropped rather than
// merged, and reusing an address for a different process hands the new one the
// state of the old (its first-seen time, its ejection status) until discovery
// drops it from a snapshot.
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
//
// The subscriber owns its channel. Register starts delivery and returns the
// current snapshot; Deregister only stops delivery. An Instancer must never
// close a channel it was handed — a subscriber that sees its own channel closed
// under it cannot tell an empty snapshot from a dead provider — and it must not
// block on a subscriber that is not reading. Give it a buffer of one and drop
// the stale event rather than the new one, as sd/instance.Cache does: on a full
// channel it discards the queued event and enqueues the current one, so a slow
// subscriber falls behind but never reads a snapshot that is already obsolete.
type Instancer interface {
	Register(chan Event) Event
	Deregister(chan Event)
	Close() error
}

// DerivedInstancer is an Instancer publishing a filtered view of another one —
// active health checking, for one.
//
// Withdrawing an instance is not the same claim as deregistering it, so a
// consumer that releases per-instance state when an instance disappears has to
// follow the Instancer a view derives from rather than the view itself: to that
// consumer a withdrawal and a deregistration look alike, and releasing the state
// that caused the withdrawal would admit the instance again the moment probing
// recovered. Declaring the source is what lets such a consumer resolve it
// instead of relying on the caller to remember — see sd/feedback.Measure.
type DerivedInstancer interface {
	Instancer
	// Underlying reports the Instancer this view derives from.
	Underlying() Instancer
}

// Registrar registers and deregisters one service instance.
//
// Deregister is the whole shutdown story: it must be idempotent and must
// release everything Register started — renewal goroutines, leases, watch
// contexts — so a caller never needs a second teardown call. A Registrar may be
// reused: Register after Deregister registers the same instance again. Clients
// and connections handed to the constructor stay the caller's to close.
//
// A Registrar owns one instance key, and Conflict is what it does when that key
// is already taken. Overwrite is the default; a provider takes anything
// stricter as a constructor option and documents which values it can enforce,
// because the guarantee comes from the registry rather than from this package.
type Registrar interface {
	Register() error
	Deregister() error
}

// Conflict is the semantics a Registrar applies when the instance key it owns
// already exists in the registry.
//
// The distinction only matters for a registry that can compare before it
// writes. A provider that cannot — Consul registers through its local agent,
// which upserts by service ID — supports overwrite alone and says so rather
// than pretending to check.
type Conflict int

const (
	// ConflictOverwrite claims the key unconditionally. It is the default
	// because it is the semantics that always recovers: an instance whose
	// previous run exited uncleanly, or whose lease expired during a partition,
	// registers again without a human deleting the key it left behind.
	ConflictOverwrite Conflict = iota

	// ConflictCreateOnly registers only while the key is absent and reports
	// ErrConflict otherwise. Choose it when a duplicate instance identity is a
	// deployment mistake worth failing start-up over — two processes configured
	// with the same instance ID — and accept that a key outliving an unclean
	// exit blocks registration until it expires.
	ConflictCreateOnly

	// ConflictCompareAndSwap registers while the key is absent or still holds
	// what this Registrar last wrote, and reports ErrConflict once another
	// writer has taken it. It is create-only for a first registration and
	// overwrite for the registrar's own renewals, so an instance recovers from
	// a lost lease without stealing a key that now belongs to somebody else.
	ConflictCompareAndSwap
)

func (c Conflict) String() string {
	switch c {
	case ConflictOverwrite:
		return "overwrite"
	case ConflictCreateOnly:
		return "create-only"
	case ConflictCompareAndSwap:
		return "compare-and-swap"
	default:
		return "unknown"
	}
}

// ErrConflict reports that a registration was refused because another writer
// holds the instance key. It is a permanent condition as far as the registrar
// is concerned: retrying writes the same key with the same identity and is
// refused again, so a caller treats it as a configuration or ownership problem
// rather than a transient registry error.
var ErrConflict = errors.New("instance key already registered")

// Outcome is the result of one call made through a Picked endpoint.
// Latency includes endpoint execution time, not time spent waiting to pick an
// instance.
//
// Bytes is the application-defined total volume the call moved, in both
// directions. sd/retry does not fill it, because an endpoint's request and
// response are opaque values at that layer; a caller that knows the protocol — a
// proxy counting the bytes it relayed, a transport reading a content length —
// reports it by wrapping Picked.Done. Zero means nobody measured it. A caller
// that needs per-direction or protocol-specific detail keeps it in its own
// table, keyed by instance, rather than in this struct: every call on the hot
// path pays for whatever lands here.
type Outcome struct {
	Err     error
	Latency time.Duration
	Bytes   int64
}

// Done feeds the result of a picked call back to the selector. It is safe for
// implementations to make a Done function idempotent; callers invoke it once
// for every successful Pick, including calls that return an error.
type Done func(Outcome)

// Release invokes done with err when done is non-nil. A Strategy or Balancer
// that wraps another must call it on every path that discards an inner Pick that
// succeeded: the inner strategy may already have reserved an in-flight slot, and
// dropping its Done leaks that reservation for the life of the process.
func Release(done Done, err error) {
	if done == nil {
		return
	}
	done(Outcome{Err: err})
}

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

// ErrClosed indicates that a selector or balancer has been closed and cannot
// accept new picks. An endpoint already returned remains the caller's
// responsibility to finish.
//
// The check is best effort, not a barrier: Close does not wait for a pick that
// is already inside the strategy, so a call that started before Close may still
// complete normally. Making it a barrier would put every pick behind a lock for
// the sake of a shutdown path. A Strategy whose Close releases state that Pick
// reads must therefore be safe against a concurrent Pick; the built-in
// strategies are.
var ErrClosed = errors.New("selector or balancer is closed")
