package balancer_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/endpoint"
	kitlog "github.com/dreamsxin/go-kit/log"
	"github.com/dreamsxin/go-kit/sd/endpointer"
	"github.com/dreamsxin/go-kit/sd/endpointer/balancer"
	"github.com/dreamsxin/go-kit/sd/events"
	"github.com/dreamsxin/go-kit/sd/instance"
)

var nopLogger = kitlog.NewNopLogger()

func echoFactory(addr string) (endpoint.Endpoint, io.Closer, error) {
	ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) { return addr, nil })
	return ep, io.NopCloser(nil), nil
}

func newEndpointer(t *testing.T, addrs ...string) endpointer.Endpointer {
	t.Helper()
	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, endpoint.Factory(echoFactory), nopLogger)
	if len(addrs) > 0 {
		cache.Update(events.Event{Instances: addrs})
		time.Sleep(20 * time.Millisecond)
	}
	return ep
}

// ── RoundRobin ────────────────────────────────────────────────────────────────

func TestRoundRobin_NoEndpoints(t *testing.T) {
	ep := newEndpointer(t)
	lb := balancer.NewRoundRobin(ep)
	_, err := lb.Endpoint()
	if err == nil {
		t.Error("expected error with no endpoints")
	}
}

func TestRoundRobin_SingleEndpoint(t *testing.T) {
	ep := newEndpointer(t, "only:80")
	lb := balancer.NewRoundRobin(ep)

	for i := 0; i < 3; i++ {
		e, err := lb.Endpoint()
		if err != nil {
			t.Fatalf("Endpoint() error: %v", err)
		}
		resp, _ := e(context.Background(), nil)
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
		e, err := lb.Endpoint()
		if err != nil {
			t.Fatalf("Endpoint() error: %v", err)
		}
		resp, _ := e(context.Background(), nil)
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
		e, err := lb.Endpoint()
		if err != nil {
			t.Fatalf("Endpoint() error: %v", err)
		}
		resp, _ := e(context.Background(), nil)
		seen[resp.(string)] = true
	}
	if len(seen) != 3 {
		t.Errorf("expected all 3 endpoints to be hit, got: %v", seen)
	}
}
