package balancer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/balancer"
)

func selectAddress(t *testing.T, lb sd.Balancer) string {
	t.Helper()
	selected, err := lb.Endpoint()
	if err != nil {
		t.Fatalf("Endpoint() error: %v", err)
	}
	resp, err := selected(context.Background(), nil)
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	return resp.(string)
}

func TestWeightedRandom_NilWeightFunctionPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic on a nil weight function")
		}
	}()
	balancer.NewWeightedRandom(newEndpointer(t, "A:80"), nil)
}

func TestWeightedRandom_NoEndpoints(t *testing.T) {
	lb := balancer.NewWeightedRandom(newEndpointer(t), func(string) int { return 1 })
	if _, err := lb.Endpoint(); !errors.Is(err, sd.ErrNoEndpoints) {
		t.Fatalf("Endpoint() error = %v, want ErrNoEndpoints", err)
	}
}

func TestWeightedRandom_SkipsNonPositiveWeights(t *testing.T) {
	lb := balancer.NewWeightedRandom(newEndpointer(t, "drained:80", "live:80"), func(instance string) int {
		if instance == "drained:80" {
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
	lb := balancer.NewWeightedRandom(newEndpointer(t, "bad:80", "good:80"), func(instance string) int {
		if instance == "bad:80" {
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
	lb := balancer.NewWeightedRandom(newEndpointer(t, "A:80", "B:80"), func(string) int { return 0 })
	if _, err := lb.Endpoint(); !errors.Is(err, sd.ErrNoEndpoints) {
		t.Fatalf("Endpoint() error = %v, want ErrNoEndpoints", err)
	}
}

// With weights 1:9 over 2000 draws the heavy instance lands within [1700,1900]
// at more than six standard deviations, so this band tolerates jitter while
// still failing if the weights are ignored.
func TestWeightedRandom_DistributesProportionally(t *testing.T) {
	lb := balancer.NewWeightedRandom(newEndpointer(t, "heavy:80", "light:80"), func(instance string) int {
		if instance == "heavy:80" {
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
	lb := balancer.NewWeightedRandom(newEndpointer(t, "A:80", "B:80", "C:80"), func(string) int { return 1 })

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

	lb := balancer.NewWeightedRandom(source, func(string) int { return 1 })
	if _, err := lb.Endpoint(); err == nil {
		t.Fatal("expected the closed endpointer error to propagate")
	}
}
