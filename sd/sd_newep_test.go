package sd_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/endpoint"
	kitlog "github.com/dreamsxin/go-kit/log"
	"github.com/dreamsxin/go-kit/sd"
	"github.com/dreamsxin/go-kit/sd/events"
	"github.com/dreamsxin/go-kit/sd/instance"
)

func nopFactory(addr string) (endpoint.Endpoint, io.Closer, error) {
	ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		return addr, nil
	})
	return ep, io.NopCloser(nil), nil
}

func nopLogger() *kitlog.Logger { return kitlog.NewNopLogger() }

// ── NewEndpoint ───────────────────────────────────────────────────────────────

func TestNewEndpoint_CallsInstance(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"host:80"}})
	time.Sleep(10 * time.Millisecond)

	ep := sd.NewEndpoint(cache, endpoint.Factory(nopFactory), nopLogger())
	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "host:80" {
		t.Errorf("want 'host:80', got %v", resp)
	}
}

func TestNewEndpoint_RoundRobin(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"a:80", "b:80"}})
	time.Sleep(10 * time.Millisecond)

	ep := sd.NewEndpoint(cache, endpoint.Factory(nopFactory), nopLogger(),
		sd.WithMaxRetries(1),
		sd.WithTimeout(time.Second),
	)

	seen := map[string]int{}
	for i := 0; i < 4; i++ {
		resp, err := ep(context.Background(), nil)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		seen[resp.(string)]++
	}
	if seen["a:80"] == 0 || seen["b:80"] == 0 {
		t.Errorf("expected both instances to be called, got %v", seen)
	}
}

func TestNewEndpoint_WithOptions(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"svc:80"}})
	time.Sleep(10 * time.Millisecond)

	ep := sd.NewEndpoint(cache, endpoint.Factory(nopFactory), nopLogger(),
		sd.WithMaxRetries(2),
		sd.WithTimeout(500*time.Millisecond),
		sd.WithInvalidateOnError(5*time.Second),
	)
	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "svc:80" {
		t.Errorf("want 'svc:80', got %v", resp)
	}
}

// ── NewEndpointWithDefaults ───────────────────────────────────────────────────

func TestNewEndpointWithDefaults(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"default:80"}})
	time.Sleep(10 * time.Millisecond)

	ep := sd.NewEndpointWithDefaults(cache, endpoint.Factory(nopFactory), nopLogger())
	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "default:80" {
		t.Errorf("want 'default:80', got %v", resp)
	}
}

// ── WithMaxRetries(0) = unlimited ─────────────────────────────────────────────

func TestNewEndpoint_UnlimitedRetries(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"svc:80"}})
	time.Sleep(10 * time.Millisecond)

	// MaxRetries=0 → RetryAlways (until timeout)
	ep := sd.NewEndpoint(cache, endpoint.Factory(nopFactory), nopLogger(),
		sd.WithMaxRetries(0),
		sd.WithTimeout(200*time.Millisecond),
	)
	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "svc:80" {
		t.Errorf("want 'svc:80', got %v", resp)
	}
}
