package balancer_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/balancer"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/instance"
	"log/slog"
)

var nopLogger = slog.New(slog.DiscardHandler)

func echoFactory(instance sd.Instance) (endpoint.Endpoint, io.Closer, error) {
	address := instance.Address
	ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) { return address, nil })
	return ep, io.NopCloser(nil), nil
}

func newEndpointer(t *testing.T, addrs ...string) endpointer.InstanceEndpointer {
	t.Helper()
	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, endpointer.Factory(echoFactory), nopLogger)
	t.Cleanup(func() { _ = ep.Close() })
	if len(addrs) > 0 {
		cache.Update(sd.Event{Instances: sd.Addresses(addrs...)})
		time.Sleep(20 * time.Millisecond)
	}
	return ep
}

// ── RoundRobin ────────────────────────────────────────────────────────────────

func TestRoundRobin_NoEndpoints(t *testing.T) {
	ep := newEndpointer(t)
	lb := balancer.NewRoundRobin(ep)
	_, err := lb.Pick(context.Background(), nil)
	if err == nil {
		t.Error("expected error with no endpoints")
	}
}

func TestRoundRobin_SingleEndpoint(t *testing.T) {
	ep := newEndpointer(t, "only:80")
	lb := balancer.NewRoundRobin(ep)

	for i := 0; i < 3; i++ {
		resp := callPicked(t, pick(t, lb, nil), nil)
		if resp != "only:80" {
			t.Errorf("got %v, want only:80", resp)
		}
	}
}

func TestRoundRobin_DistributesEvenly(t *testing.T) {
	ep := newEndpointer(t, "A:80", "B:80")
	lb := balancer.NewRoundRobin(ep)

	counts := map[string]int{}
	for i := 0; i < 6; i++ {
		resp := callPicked(t, pick(t, lb, nil), nil)
		counts[resp.(string)]++
	}
	if counts["A:80"] != 3 || counts["B:80"] != 3 {
		t.Errorf("uneven distribution: %v", counts)
	}
}

func TestRoundRobin_ThreeEndpoints_Cycles(t *testing.T) {
	ep := newEndpointer(t, "A:80", "B:80", "C:80")
	lb := balancer.NewRoundRobin(ep)

	seen := map[string]bool{}
	for i := 0; i < 6; i++ {
		resp := callPicked(t, pick(t, lb, nil), nil)
		seen[resp.(string)] = true
	}
	if len(seen) != 3 {
		t.Errorf("expected all 3 endpoints to be hit, got: %v", seen)
	}
}
