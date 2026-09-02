package balancer_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/balancer"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/instance"
)

// newUpdatableEndpointer exposes the instance cache so a test can change the
// instance set after the balancer has been built.
func newUpdatableEndpointer(t *testing.T, addrs ...string) (*instance.Cache, endpointer.InstanceEndpointer) {
	t.Helper()
	cache := instance.NewCache()
	set := endpointer.NewEndpointer(cache, endpointer.Factory(echoFactory), nopLogger)
	t.Cleanup(func() { _ = set.Close() })
	updateInstances(t, cache, addrs...)
	return cache, set
}

func updateInstances(t *testing.T, cache *instance.Cache, addrs ...string) {
	t.Helper()
	cache.Update(sd.Event{Instances: sd.Addresses(addrs...)})
	time.Sleep(20 * time.Millisecond)
}

// requestKey routes on the request value itself, which keeps the tests focused
// on ring behaviour rather than on key extraction.
func requestKey(_ context.Context, request any) string {
	key, _ := request.(string)
	return key
}

func routeKey(t *testing.T, lb sd.Balancer, key string) string {
	t.Helper()
	selected, err := lb.Pick(context.Background(), key)
	if err != nil {
		t.Fatalf("Pick(%q) error: %v", key, err)
	}
	resp, err := selected.Endpoint(context.Background(), key)
	if selected.Done != nil {
		selected.Done(sd.Outcome{Err: err})
	}
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	return resp.(string)
}

func TestConsistentHash_NilKeyFunctionPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic on a nil key function")
		}
	}()
	balancer.NewConsistentHash(newEndpointer(t, "A:80"), nil)
}

func TestConsistentHash_NoEndpoints(t *testing.T) {
	lb := balancer.NewConsistentHash(newEndpointer(t), requestKey)
	if _, err := lb.Pick(context.Background(), "tenant-1"); !errors.Is(err, sd.ErrNoEndpoints) {
		t.Fatalf("Pick error = %v, want ErrNoEndpoints", err)
	}
}

func TestConsistentHash_SameKeyReachesSameInstance(t *testing.T) {
	lb := balancer.NewConsistentHash(newEndpointer(t, "A:80", "B:80", "C:80"), requestKey)

	first := routeKey(t, lb, "tenant-42")
	for i := 0; i < 100; i++ {
		if again := routeKey(t, lb, "tenant-42"); again != first {
			t.Fatalf("key moved from %s to %s", first, again)
		}
	}
}

// Spread matters as much as stability: a ring whose hash lacks avalanche keeps
// every prefixed key on one instance while still passing a stability test.
func TestConsistentHash_DifferentKeysSpreadAcrossInstances(t *testing.T) {
	lb := balancer.NewConsistentHash(newEndpointer(t, "A:80", "B:80", "C:80"), requestKey)

	const keys = 300
	seen := map[string]int{}
	for i := 0; i < keys; i++ {
		seen[routeKey(t, lb, "key-"+strconv.Itoa(i))]++
	}
	if len(seen) != 3 {
		t.Fatalf("expected keys to spread over all 3 instances, got %v", seen)
	}
	for instance, count := range seen {
		if count < keys/10 {
			t.Fatalf("instance %s owns only %d of %d keys: %v", instance, count, keys, seen)
		}
	}
}

// Removing one instance must only remap the keys it owned. This is the whole
// reason to prefer a hash ring over hashing modulo the instance count.
func TestConsistentHash_RemovalOnlyRemapsAffectedKeys(t *testing.T) {
	cache, set := newUpdatableEndpointer(t, "A:80", "B:80", "C:80")
	lb := balancer.NewConsistentHash(set, requestKey)

	keys := make([]string, 300)
	before := make(map[string]string, len(keys))
	for i := range keys {
		keys[i] = "key-" + strconv.Itoa(i)
		before[keys[i]] = routeKey(t, lb, keys[i])
	}

	updateInstances(t, cache, "A:80", "B:80")

	moved := 0
	for _, key := range keys {
		after := routeKey(t, lb, key)
		if before[key] == "C:80" {
			moved++
			continue
		}
		if after != before[key] {
			t.Fatalf("key %q moved from %s to %s despite its owner staying", key, before[key], after)
		}
	}
	if moved == 0 {
		t.Fatal("no key was owned by the removed instance, so nothing was verified")
	}
}

// The ring is cached per instance set; a re-added instance must reclaim exactly
// the keys it held before, not a fresh assignment.
func TestConsistentHash_ReAddedInstanceReclaimsItsKeys(t *testing.T) {
	cache, set := newUpdatableEndpointer(t, "A:80", "B:80", "C:80")
	lb := balancer.NewConsistentHash(set, requestKey)

	keys := make([]string, 200)
	before := make(map[string]string, len(keys))
	for i := range keys {
		keys[i] = "key-" + strconv.Itoa(i)
		before[keys[i]] = routeKey(t, lb, keys[i])
	}

	updateInstances(t, cache, "A:80", "B:80")
	updateInstances(t, cache, "A:80", "B:80", "C:80")

	for _, key := range keys {
		if after := routeKey(t, lb, key); after != before[key] {
			t.Fatalf("key %q moved from %s to %s after the set was restored", key, before[key], after)
		}
	}
}

// An unkeyed request must not pin every caller onto one instance.
func TestConsistentHash_EmptyKeyFallsBackToRandom(t *testing.T) {
	lb := balancer.NewConsistentHash(newEndpointer(t, "A:80", "B:80", "C:80"), requestKey)

	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		seen[routeKey(t, lb, "")] = true
	}
	if len(seen) != 3 {
		t.Fatalf("expected the unkeyed fallback to spread over all 3 instances, got %v", seen)
	}
}

func TestConsistentHash_EmptyRequestFallsBackToRandom(t *testing.T) {
	lb := balancer.NewConsistentHash(newEndpointer(t, "A:80", "B:80", "C:80"), requestKey)

	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		seen[selectAddress(t, lb)] = true
	}
	if len(seen) != 3 {
		t.Fatalf("expected empty requests to spread over all 3 instances, got %v", seen)
	}
}

func TestConsistentHash_WithReplicas(t *testing.T) {
	tests := []struct {
		name     string
		replicas int
	}{
		{name: "single virtual node", replicas: 1},
		{name: "non-positive falls back to default", replicas: 0},
		{name: "explicit default", replicas: balancer.DefaultReplicas},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lb := balancer.NewConsistentHash(newEndpointer(t, "A:80", "B:80", "C:80"), requestKey,
				balancer.WithReplicas(tt.replicas))

			first := routeKey(t, lb, "tenant-7")
			for i := 0; i < 20; i++ {
				if again := routeKey(t, lb, "tenant-7"); again != first {
					t.Fatalf("key moved from %s to %s", first, again)
				}
			}
		})
	}
}

func TestConsistentHash_PropagatesSourceError(t *testing.T) {
	source := newEndpointer(t, "A:80")
	if err := source.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lb := balancer.NewConsistentHash(source, requestKey)
	if _, err := lb.Pick(context.Background(), "tenant-1"); err == nil {
		t.Fatal("expected the closed endpointer error to propagate")
	}
}
