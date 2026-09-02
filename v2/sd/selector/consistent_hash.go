package selector

import (
	"context"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/dreamsxin/go-kit/v2/sd"
)

// KeyFunc derives the partition key of a request. An empty key means the
// request cannot be routed stably, and selection falls back to a random
// instance rather than pinning every unkeyed request onto one of them.
type KeyFunc func(ctx context.Context, request any) string

// DefaultReplicas is the number of virtual nodes each instance gets on the
// hash ring. More replicas smooth the distribution at the cost of a larger
// ring; 100 keeps a handful of instances within a few percent of even.
const DefaultReplicas = 100

// HashOption configures ConsistentHash.
type HashOption func(*consistentHash)

// WithReplicas sets the virtual nodes per instance. Values below one fall back
// to DefaultReplicas.
func WithReplicas(replicas int) HashOption {
	return func(c *consistentHash) {
		if replicas > 0 {
			c.replicas = replicas
		}
	}
}

// ConsistentHash routes requests that share a key to the same instance for as
// long as that instance stays in the set. Adding or removing one instance only
// remaps the keys it owned, which is what makes it usable for cache affinity or
// per-entity ordering.
//
// The request is part of the common Strategy contract, so callers always keep
// the key without a second request-aware interface.
//
// ConsistentHash panics on a nil key function, which is a programming error
// rather than a runtime condition.
func ConsistentHash(key KeyFunc, options ...HashOption) Strategy {
	if key == nil {
		panic("selector: nil key function")
	}
	hash := &consistentHash{key: key, replicas: DefaultReplicas}
	for _, option := range options {
		option(hash)
	}
	return hash
}

type consistentHash struct {
	key      KeyFunc
	replicas int

	mtx  sync.Mutex
	ring hashRing
}

type hashRing struct {
	fingerprint string
	points      []uint64
	owners      []int
}

func (c *consistentHash) pickKeyed(ctx context.Context, request any, instances []sd.Instance) (int, sd.Done, error) {
	if len(instances) == 0 {
		return 0, nil, sd.ErrNoEndpoints
	}

	key := c.key(ctx, request)
	if key == "" {
		return rand.N(len(instances)), nil, nil
	}

	ring := c.ringFor(instances)
	if len(ring.points) == 0 {
		return 0, nil, sd.ErrNoEndpoints
	}

	target := hashKey(key)
	index := sort.Search(len(ring.points), func(i int) bool { return ring.points[i] >= target })
	if index == len(ring.points) {
		index = 0
	}
	return ring.owners[index], nil, nil
}

func (c *consistentHash) Pick(ctx context.Context, request any, instances []sd.Instance) (int, sd.Done, error) {
	return c.pickKeyed(ctx, request, instances)
}

// ringFor returns the ring for the current snapshot, rebuilding it only when
// the instance set changed. Rebuilding per call would hash replicas*instances
// keys on every request.
func (c *consistentHash) ringFor(instances []sd.Instance) hashRing {
	fingerprint := fingerprintOf(instances)

	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.ring.fingerprint == fingerprint {
		return c.ring
	}

	points := make([]uint64, 0, len(instances)*c.replicas)
	owners := make(map[uint64]int, len(instances)*c.replicas)
	for i, instance := range instances {
		for replica := 0; replica < c.replicas; replica++ {
			point := hashKey(instance.Address + "#" + strconv.Itoa(replica))
			if _, taken := owners[point]; taken {
				continue
			}
			owners[point] = i
			points = append(points, point)
		}
	}
	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })

	ordered := make([]int, len(points))
	for i, point := range points {
		ordered[i] = owners[point]
	}

	c.ring = hashRing{fingerprint: fingerprint, points: points, owners: ordered}
	return c.ring
}

// fingerprintOf identifies an instance set by its addresses. Labels are left
// out on purpose: relabelling an instance must not reshuffle the ring and move
// keys that still have a healthy owner.
func fingerprintOf(instances []sd.Instance) string {
	var builder strings.Builder
	for _, instance := range instances {
		builder.WriteString(instance.Address)
		builder.WriteByte(0)
	}
	return builder.String()
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
