package balancer_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/balancer"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/instance"
)

func TestWeightedRandom_NilWeightFunctionPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic on a nil weight function")
		}
	}()
	balancer.NewWeightedRandom(newEndpointer(t, "A:80"), nil)
}

func TestWeightedRandom_NoEndpoints(t *testing.T) {
	lb := balancer.NewWeightedRandom(newEndpointer(t), func(sd.Instance) int { return 1 })
	if _, err := lb.Pick(context.Background(), nil); !errors.Is(err, sd.ErrNoEndpoints) {
		t.Fatalf("Endpoint() error = %v, want ErrNoEndpoints", err)
	}
}

func TestWeightedRandom_SkipsNonPositiveWeights(t *testing.T) {
	lb := balancer.NewWeightedRandom(newEndpointer(t, "drained:80", "live:80"), func(instance sd.Instance) int {
		if instance.Address == "drained:80" {
			return 0
		}
		return 5
	})

	for i := 0; i < 50; i++ {
		if address := selectAddress(t, lb); address != "live:80" {
			t.Fatalf("selected %s, want live:80", address)
		}
	}
}

func TestWeightedRandom_NegativeWeightIsExcluded(t *testing.T) {
	lb := balancer.NewWeightedRandom(newEndpointer(t, "bad:80", "good:80"), func(instance sd.Instance) int {
		if instance.Address == "bad:80" {
			return -10
		}
		return 1
	})

	for i := 0; i < 50; i++ {
		if address := selectAddress(t, lb); address != "good:80" {
			t.Fatalf("selected %s, want good:80", address)
		}
	}
}

// Endpoints exist but none is selectable, which the caller must be able to
// distinguish from a successful selection.
func TestWeightedRandom_AllWeightsZero(t *testing.T) {
	lb := balancer.NewWeightedRandom(newEndpointer(t, "A:80", "B:80"), func(sd.Instance) int { return 0 })
	if _, err := lb.Pick(context.Background(), nil); !errors.Is(err, sd.ErrNoEndpoints) {
		t.Fatalf("Endpoint() error = %v, want ErrNoEndpoints", err)
	}
}

// With weights 1:9 over 2000 draws the heavy instance lands within [1700,1900]
// at more than six standard deviations, so this band tolerates jitter while
// still failing if the weights are ignored.
func TestWeightedRandom_DistributesProportionally(t *testing.T) {
	lb := balancer.NewWeightedRandom(newEndpointer(t, "heavy:80", "light:80"), func(instance sd.Instance) int {
		if instance.Address == "heavy:80" {
			return 9
		}
		return 1
	})

	counts := map[string]int{}
	for i := 0; i < 2000; i++ {
		counts[selectAddress(t, lb)]++
	}
	if counts["heavy:80"] < 1700 || counts["heavy:80"] > 1900 {
		t.Fatalf("heavy instance selected %d times, want roughly 1800: %v", counts["heavy:80"], counts)
	}
	if counts["light:80"] == 0 {
		t.Fatalf("light instance was never selected: %v", counts)
	}
}

func TestWeightedRandom_EqualWeightsReachEveryEndpoint(t *testing.T) {
	lb := balancer.NewWeightedRandom(newEndpointer(t, "A:80", "B:80", "C:80"), func(sd.Instance) int { return 1 })

	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		seen[selectAddress(t, lb)] = true
	}
	if len(seen) != 3 {
		t.Fatalf("expected all 3 endpoints to be selected, got %v", seen)
	}
}

func TestWeightedRandom_PropagatesSourceError(t *testing.T) {
	source := newEndpointer(t, "A:80")
	if err := source.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lb := balancer.NewWeightedRandom(source, func(sd.Instance) int { return 1 })
	if _, err := lb.Pick(context.Background(), nil); err == nil {
		t.Fatal("expected the closed endpointer error to propagate")
	}
}

// ── MetadataWeight ────────────────────────────────────────────────────────────

func TestMetadataWeight(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		fallback int
		metadata map[string]any
		want     int
	}{
		{name: "reads the registry string", key: "weight", fallback: 1, metadata: map[string]any{"weight": "10"}, want: 10},
		{name: "reads a typed value", key: "weight", fallback: 1, metadata: map[string]any{"weight": 7}, want: 7},
		{name: "empty key uses the default label", key: "", fallback: 1, metadata: map[string]any{"weight": "3"}, want: 3},
		{name: "custom key", key: "capacity", fallback: 1, metadata: map[string]any{"capacity": "5"}, want: 5},
		{name: "absent label falls back", key: "weight", fallback: 4, metadata: map[string]any{"zone": "north"}, want: 4},
		{name: "unparsable label falls back", key: "weight", fallback: 4, metadata: map[string]any{"weight": "heavy"}, want: 4},
		{name: "no metadata falls back", key: "weight", fallback: 2, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			weight := balancer.MetadataWeight(tt.key, tt.fallback)
			if got := weight(sd.Instance{Address: "svc:80", Metadata: tt.metadata}); got != tt.want {
				t.Fatalf("weight = %d, want %d", got, tt.want)
			}
		})
	}
}

// End to end: an instance that reported weight 0 at registration is drained
// without service discovery having to withdraw it.
func TestWeightedRandom_HonoursRegisteredWeights(t *testing.T) {
	cache := instance.NewCache()
	set := endpointer.NewEndpointer(cache, endpointer.Factory(echoFactory), nopLogger)
	t.Cleanup(func() { _ = set.Close() })
	cache.Update(sd.Event{Instances: []sd.Instance{
		{Address: "drained:80", Metadata: map[string]any{"weight": "0"}},
		{Address: "live:80", Metadata: map[string]any{"weight": "5"}},
	}})
	time.Sleep(20 * time.Millisecond)

	lb := balancer.NewWeightedRandom(set, balancer.MetadataWeight(balancer.DefaultWeightKey, 1))
	for i := 0; i < 50; i++ {
		if address := selectAddress(t, lb); address != "live:80" {
			t.Fatalf("selected %s, want live:80", address)
		}
	}
}
