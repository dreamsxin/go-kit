package balancer

import (
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
)

// KeyFunc derives the partition key of a request. An empty key means the
// request cannot be routed stably, and the balancer falls back to a random
// endpoint rather than pinning every unkeyed request onto one instance.
type KeyFunc = selector.KeyFunc

// DefaultReplicas is the number of virtual nodes each instance gets on the
// hash ring. More replicas smooth the distribution at the cost of a larger
// ring; 100 keeps a handful of instances within a few percent of even.
const DefaultReplicas = selector.DefaultReplicas

// ConsistentHashOption configures NewConsistentHash.
type ConsistentHashOption = selector.HashOption

// WithReplicas sets the virtual nodes per instance. Values below one fall back
// to DefaultReplicas.
func WithReplicas(replicas int) ConsistentHashOption {
	return selector.WithReplicas(replicas)
}

// NewConsistentHash routes requests that share a key to the same instance for
// as long as that instance stays in the set. Adding or removing one instance
// only remaps the keys it owned, which is what makes it usable for cache
// affinity or per-entity ordering.
//
// The request is passed through the common Balancer.Pick contract, so callers
// do not need a second request-aware interface.
//
// NewConsistentHash panics on a nil key function, which is a programming error
// rather than a runtime condition.
func NewConsistentHash(source endpointer.InstanceEndpointer, key KeyFunc, options ...ConsistentHashOption) sd.Balancer {
	return New(source, selector.ConsistentHash(key, options...))
}
