package balancer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/balancer"
)

func TestRandom_NoEndpoints(t *testing.T) {
	lb := balancer.NewRandom(newEndpointer(t))
	if _, err := lb.Pick(context.Background(), nil); !errors.Is(err, sd.ErrNoEndpoints) {
		t.Fatalf("Endpoint() error = %v, want ErrNoEndpoints", err)
	}
}

func TestRandom_SingleEndpoint(t *testing.T) {
	lb := balancer.NewRandom(newEndpointer(t, "only:80"))

	for i := 0; i < 5; i++ {
		resp := callPicked(t, pick(t, lb, nil), nil)
		if resp != "only:80" {
			t.Fatalf("got %v, want only:80", resp)
		}
	}
}

func TestRandom_SelectsOnlyKnownEndpoints(t *testing.T) {
	lb := balancer.NewRandom(newEndpointer(t, "A:80", "B:80", "C:80"))

	known := map[string]bool{"A:80": true, "B:80": true, "C:80": true}
	for i := 0; i < 50; i++ {
		resp := callPicked(t, pick(t, lb, nil), nil)
		if !known[resp.(string)] {
			t.Fatalf("selected unknown endpoint %v", resp)
		}
	}
}

// A uniform draw over three instances misses one across 200 attempts with
// probability (2/3)^200, so a failure here means selection is not random.
func TestRandom_ReachesEveryEndpoint(t *testing.T) {
	lb := balancer.NewRandom(newEndpointer(t, "A:80", "B:80", "C:80"))

	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		resp := callPicked(t, pick(t, lb, nil), nil)
		seen[resp.(string)] = true
	}
	if len(seen) != 3 {
		t.Fatalf("expected all 3 endpoints to be selected, got %v", seen)
	}
}

func TestRandom_PropagatesSourceError(t *testing.T) {
	source := newEndpointer(t, "A:80")
	if err := source.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lb := balancer.NewRandom(source)
	if _, err := lb.Pick(context.Background(), nil); err == nil {
		t.Fatal("expected the closed endpointer error to propagate")
	}
}
