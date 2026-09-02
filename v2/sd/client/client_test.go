package client_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/balancer"
	sdclient "github.com/dreamsxin/go-kit/v2/sd/client"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/instance"
	"log/slog"
)

func nopFactory(instance sd.Instance) (endpoint.Endpoint, io.Closer, error) {
	address := instance.Address
	ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		return address, nil
	})
	return ep, io.NopCloser(nil), nil
}

func nopLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func newTestEndpoint(t *testing.T, cache *instance.Cache, opts ...sdclient.Option) endpoint.Endpoint {
	t.Helper()
	ep, closer, err := sdclient.NewEndpoint(cache, endpointer.Factory(nopFactory), nopLogger(), opts...)
	if err != nil {
		t.Fatalf("NewEndpoint: %v", err)
	}
	t.Cleanup(func() {
		if err := closer.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return ep
}

// ── NewEndpoint ───────────────────────────────────────────────────────────────

func TestNewEndpoint_CallsInstance(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: sd.Addresses("host:80")})
	time.Sleep(10 * time.Millisecond)

	ep := newTestEndpoint(t, cache)
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
	cache.Update(sd.Event{Instances: sd.Addresses("a:80", "b:80")})
	time.Sleep(10 * time.Millisecond)

	ep := newTestEndpoint(t, cache,
		sdclient.WithMaxAttempts(1),
		sdclient.WithTimeout(time.Second),
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
	cache.Update(sd.Event{Instances: sd.Addresses("svc:80")})
	time.Sleep(10 * time.Millisecond)

	ep := newTestEndpoint(t, cache,
		sdclient.WithMaxAttempts(2),
		sdclient.WithTimeout(500*time.Millisecond),
		sdclient.WithInvalidateOnError(5*time.Second),
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
	cache.Update(sd.Event{Instances: sd.Addresses("default:80")})
	time.Sleep(10 * time.Millisecond)

	ep, closer, err := sdclient.NewEndpointWithDefaults(cache, endpointer.Factory(nopFactory), nopLogger())
	if err != nil {
		t.Fatalf("NewEndpointWithDefaults: %v", err)
	}
	t.Cleanup(func() { _ = closer.Close() })
	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "default:80" {
		t.Errorf("want 'default:80', got %v", resp)
	}
}

func TestNewEndpoint_RejectsInvalidConfiguration(t *testing.T) {
	cache := instance.NewCache()
	tests := []struct {
		name    string
		src     *instance.Cache
		factory endpointer.Factory
		logger  *slog.Logger
		opts    []sdclient.Option
		want    string
	}{
		{name: "nil instancer", factory: nopFactory, logger: nopLogger(), want: "instancer is nil"},
		{name: "nil factory", src: cache, logger: nopLogger(), want: "factory is nil"},
		{name: "attempts", src: cache, factory: nopFactory, logger: nopLogger(), opts: []sdclient.Option{sdclient.WithMaxAttempts(0)}, want: "max attempts"},
		{name: "timeout", src: cache, factory: nopFactory, logger: nopLogger(), opts: []sdclient.Option{sdclient.WithTimeout(0)}, want: "timeout"},
		{name: "invalidation", src: cache, factory: nopFactory, logger: nopLogger(), opts: []sdclient.Option{sdclient.WithInvalidateOnError(-time.Second)}, want: "invalidate-on-error"},
		{name: "nil option", src: cache, factory: nopFactory, logger: nopLogger(), opts: []sdclient.Option{nil}, want: "option 0 is nil"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, closer, err := sdclient.NewEndpoint(tt.src, tt.factory, tt.logger, tt.opts...)
			if closer != nil {
				t.Fatal("invalid construction returned a closer")
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestNewEndpoint_NilLoggerFallsBackToDefault(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: sd.Addresses("svc:80")})
	time.Sleep(10 * time.Millisecond)

	ep, closer, err := sdclient.NewEndpoint(cache, endpointer.Factory(nopFactory), nil)
	if err != nil {
		t.Fatalf("NewEndpoint with nil logger: %v", err)
	}
	t.Cleanup(func() { _ = closer.Close() })
	if _, err := ep(context.Background(), nil); err != nil {
		t.Fatalf("call with default logger: %v", err)
	}
}

func TestNewEndpoint_CloserReleasesFactoryResources(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: sd.Addresses("svc:80")})
	closed := false
	factory := endpointer.Factory(func(sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		return endpoint.Nop, closerFunc(func() error {
			closed = true
			return nil
		}), nil
	})

	ep, closer, err := sdclient.NewEndpoint(cache, factory, nopLogger())
	if err != nil {
		t.Fatalf("NewEndpoint: %v", err)
	}
	if err := closer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !closed {
		t.Fatal("factory resource was not closed")
	}
	_, err = ep(context.Background(), nil)
	if !errors.Is(err, endpointer.ErrCacheClosed) {
		t.Fatalf("call after Close error = %v, want ErrCacheClosed", err)
	}
}

// ── WithBalancer ──────────────────────────────────────────────────────────────

func TestNewEndpoint_WithBalancerReplacesRoundRobin(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: sd.Addresses("a:80", "b:80", "c:80")})
	time.Sleep(10 * time.Millisecond)

	// Weighting every instance but one to zero proves the supplied balancer is
	// the one making the decision, not the default round robin.
	ep := newTestEndpoint(t, cache,
		sdclient.WithBalancer(func(set endpointer.InstanceEndpointer) sd.Balancer {
			return balancer.NewWeightedRandom(set, func(instance sd.Instance) int {
				if instance.Address == "b:80" {
					return 1
				}
				return 0
			})
		}),
	)

	for i := 0; i < 10; i++ {
		resp, err := ep(context.Background(), nil)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if resp != "b:80" {
			t.Fatalf("call %d selected %v, want b:80", i, resp)
		}
	}
}

// The consistent-hash strategy only works end to end if retry prefers the
// request-aware contract, so this also covers the sd/retry wiring.
func TestNewEndpoint_WithConsistentHashRoutesByKey(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: sd.Addresses("a:80", "b:80", "c:80")})
	time.Sleep(10 * time.Millisecond)

	ep := newTestEndpoint(t, cache,
		sdclient.WithBalancer(func(set endpointer.InstanceEndpointer) sd.Balancer {
			return balancer.NewConsistentHash(set, func(_ context.Context, request any) string {
				key, _ := request.(string)
				return key
			})
		}),
	)

	first, err := ep(context.Background(), "tenant-42")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	for i := 0; i < 20; i++ {
		again, err := ep(context.Background(), "tenant-42")
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if again != first {
			t.Fatalf("key moved from %v to %v", first, again)
		}
	}
}

func TestNewEndpoint_RejectsNilBalancerFactory(t *testing.T) {
	cache := instance.NewCache()
	_, closer, err := sdclient.NewEndpoint(cache, endpointer.Factory(nopFactory), nopLogger(),
		sdclient.WithBalancer(nil))
	if closer != nil {
		t.Fatal("invalid construction returned a closer")
	}
	if err == nil || !strings.Contains(err.Error(), "balancer factory is nil") {
		t.Fatalf("error = %v, want a nil balancer factory error", err)
	}
}

func TestNewEndpoint_RejectsBalancerFactoryReturningNil(t *testing.T) {
	cache := instance.NewCache()
	_, closer, err := sdclient.NewEndpoint(cache, endpointer.Factory(nopFactory), nopLogger(),
		sdclient.WithBalancer(func(endpointer.InstanceEndpointer) sd.Balancer { return nil }))
	if closer != nil {
		t.Fatal("invalid construction returned a closer")
	}
	if err == nil || !strings.Contains(err.Error(), "balancer factory returned nil") {
		t.Fatalf("error = %v, want a nil balancer error", err)
	}
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }
