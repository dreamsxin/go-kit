package balancer

import (
	"context"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
)

// KeyFunc derives the partition key of a request. An empty key means the
// request cannot be routed stably, and the balancer falls back to a random
// endpoint rather than pinning every unkeyed request onto one instance.
type KeyFunc func(ctx context.Context, request any) string

// DefaultReplicas is the number of virtual nodes each instance gets on the
// hash ring. More replicas smooth the distribution at the cost of a larger
// ring; 100 keeps a handful of instances within a few percent of even.
const DefaultReplicas = 100

// ConsistentHashOption configures NewConsistentHash.
type ConsistentHashOption func(*consistentHash)

// WithReplicas sets the virtual nodes per instance. Values below one fall back
// to DefaultReplicas.
func WithReplicas(replicas int) ConsistentHashOption {
	return func(c *consistentHash) {
		if replicas > 0 {
			c.replicas = replicas
		}
	}
}

// NewConsistentHash routes requests that share a key to the same instance for
// as long as that instance stays in the set. Adding or removing one instance
// only remaps the keys it owned, which is what makes it usable for cache
// affinity or per-entity ordering.
//
// The returned Balancer also implements sd.RequestBalancer; executors that hold
// the request, including sd/retry, use the keyed path automatically. Calling
// Endpoint directly has no request to key on and selects at random.
//
// NewConsistentHash panics on a nil key function, which is a programming error
// rather than a runtime condition.
func NewConsistentHash(source endpointer.InstanceEndpointer, key KeyFunc, options ...ConsistentHashOption) sd.RequestBalancer {
	if key == nil {
		panic("balancer: nil key function")
	}
	hash := &consistentHash{source: source, key: key, replicas: DefaultReplicas}
	for _, option := range options {
		option(hash)
	}
	return hash
}

type consistentHash struct {
	source   endpointer.InstanceEndpointer
	key      KeyFunc
	replicas int

	mtx  sync.Mutex
	ring hashRing
}

type hashRing struct {
	fingerprint string
	points      []uint64
	endpoints   []endpoint.Endpoint
}

func (c *consistentHash) Endpoint() (endpoint.Endpoint, error) {
	instances, err := c.source.InstanceEndpoints()
	if err != nil {
		return nil, err
	}
	if len(instances) == 0 {
		return nil, sd.ErrNoEndpoints
	}
	return instances[rand.N(len(instances))].Endpoint, nil
}

func (c *consistentHash) EndpointFor(ctx context.Context, request any) (endpoint.Endpoint, error) {
	instances, err := c.source.InstanceEndpoints()
	if err != nil {
		return nil, err
	}
	if len(instances) == 0 {
		return nil, sd.ErrNoEndpoints
	}

	key := c.key(ctx, request)
	if key == "" {
		return instances[rand.N(len(instances))].Endpoint, nil
	}

	ring := c.ringFor(instances)
	if len(ring.points) == 0 {
		return nil, sd.ErrNoEndpoints
	}

	target := hashKey(key)
	index := sort.Search(len(ring.points), func(i int) bool { return ring.points[i] >= target })
	if index == len(ring.points) {
		index = 0
	}
	return ring.endpoints[index], nil
}

// ringFor returns the ring for the current snapshot, rebuilding it only when
// the instance set changed. Rebuilding per call would hash replicas*instances
// keys on every request.
func (c *consistentHash) ringFor(instances []endpointer.InstanceEndpoint) hashRing {
	fingerprint := fingerprintOf(instances)

	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.ring.fingerprint == fingerprint {
		return c.ring
	}

	points := make([]uint64, 0, len(instances)*c.replicas)
	owners := make(map[uint64]endpoint.Endpoint, len(instances)*c.replicas)
	for _, item := range instances {
		for replica := 0; replica < c.replicas; replica++ {
			point := hashKey(item.Instance + "#" + strconv.Itoa(replica))
			if _, taken := owners[point]; taken {
				continue
			}
			owners[point] = item.Endpoint
			points = append(points, point)
		}
	}
	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })

	endpoints := make([]endpoint.Endpoint, len(points))
	for i, point := range points {
		endpoints[i] = owners[point]
	}

	c.ring = hashRing{fingerprint: fingerprint, points: points, endpoints: endpoints}
	return c.ring
}

func fingerprintOf(instances []endpointer.InstanceEndpoint) string {
	addresses := make([]string, len(instances))
	for i, item := range instances {
		addresses[i] = item.Instance
	}
	return strings.Join(addresses, "\x00")
}

func hashKey(key string) uint64 {
	hash := uint64(fnvOffset64)
	for i := 0; i < len(key); i++ {
		hash ^= uint64(key[i])
		hash *= fnvPrime64
	}
	return avalanche(hash)
}

// FNV-1a is inlined rather than taken from hash/fnv because hashKey runs on
// every keyed request and the stdlib constructor allocates.
const (
	fnvOffset64 = 14695981039346656037
	fnvPrime64  = 1099511628211
)

// avalanche is the SplitMix64 finaliser. Raw FNV-1a leaves its high bits
// dominated by the leading bytes of the input, so keys sharing a prefix such
// as "tenant-1" and "tenant-2" would otherwise land on the same ring segment
// and defeat the whole point of the ring.
func avalanche(x uint64) uint64 {
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}
