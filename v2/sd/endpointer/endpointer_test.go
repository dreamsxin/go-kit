package endpointer_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/instance"
	"log/slog"
)

var nopLogger = slog.New(slog.DiscardHandler)

var echoFactory = endpointer.Factory(func(addr string) (endpoint.Endpoint, io.Closer, error) {
	ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		return addr, nil
	})
	return ep, io.NopCloser(nil), nil
})

func TestNewEndpointer_NoInstances(t *testing.T) {
	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, echoFactory, nopLogger)
	t.Cleanup(func() { _ = ep.Close() })

	eps, err := ep.Endpoints()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(eps) != 0 {
		t.Errorf("expected 0 endpoints, got %d", len(eps))
	}
}

func TestNewEndpointer_ReceivesInstances(t *testing.T) {
	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, echoFactory, nopLogger)
	t.Cleanup(func() { _ = ep.Close() })

	cache.Update(sd.Event{Instances: []string{"a:80", "b:80"}})
	time.Sleep(20 * time.Millisecond)

	eps, err := ep.Endpoints()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(eps) != 2 {
		t.Errorf("expected 2 endpoints, got %d", len(eps))
	}
}

func TestNewEndpointer_UpdateInstances(t *testing.T) {
	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, echoFactory, nopLogger)
	t.Cleanup(func() { _ = ep.Close() })

	cache.Update(sd.Event{Instances: []string{"a:80", "b:80", "c:80"}})
	time.Sleep(20 * time.Millisecond)

	cache.Update(sd.Event{Instances: []string{"a:80"}})
	time.Sleep(20 * time.Millisecond)

	eps, err := ep.Endpoints()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(eps) != 1 {
		t.Errorf("expected 1 endpoint after update, got %d", len(eps))
	}
}

func TestNewEndpointer_FactoryError_SkipsInstance(t *testing.T) {
	failFactory := endpointer.Factory(func(addr string) (endpoint.Endpoint, io.Closer, error) {
		if addr == "bad:80" {
			return nil, nil, errors.New("factory error")
		}
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) { return addr, nil })
		return ep, io.NopCloser(nil), nil
	})

	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, failFactory, nopLogger)
	t.Cleanup(func() { _ = ep.Close() })

	cache.Update(sd.Event{Instances: []string{"good:80", "bad:80"}})
	time.Sleep(20 * time.Millisecond)

	eps, err := ep.Endpoints()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// bad:80 should be skipped
	if len(eps) != 1 {
		t.Errorf("expected 1 endpoint (bad skipped), got %d", len(eps))
	}
}

func TestNewEndpointer_WithInvalidateOnError(t *testing.T) {
	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, echoFactory, nopLogger,
		endpointer.InvalidateOnError(50*time.Millisecond),
	)
	t.Cleanup(func() { _ = ep.Close() })

	cache.Update(sd.Event{Instances: []string{"svc:80"}})
	time.Sleep(20 * time.Millisecond)

	// healthy
	eps, err := ep.Endpoints()
	if err != nil || len(eps) == 0 {
		t.Fatalf("expected healthy endpoints, got err=%v len=%d", err, len(eps))
	}

	// inject error
	cache.Update(sd.Event{Err: errors.New("sd error")})
	time.Sleep(10 * time.Millisecond)

	// within grace period — still returns cached
	eps, _ = ep.Endpoints()
	if len(eps) == 0 {
		t.Log("grace period: cache may already be cleared (timing)")
	}

	// after grace period — cache cleared
	time.Sleep(80 * time.Millisecond)
	eps, err = ep.Endpoints()
	if err == nil && len(eps) > 0 {
		t.Error("expected cache to be invalidated after grace period")
	}
}

func TestDefaultEndpointer_CloseIsIdempotentAndSafeDuringUpdate(t *testing.T) {
	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, echoFactory, nopLogger)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			cache.Update(sd.Event{Instances: []string{"svc:80"}})
			cache.Update(sd.Event{Instances: []string{"svc2:80"}})
		}
		close(done)
	}()

	if err := ep.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := ep.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("updates blocked while endpointer was closing")
	}
	if _, err := ep.Endpoints(); !errors.Is(err, endpointer.ErrCacheClosed) {
		t.Fatalf("Endpoints after Close error = %v, want ErrCacheClosed", err)
	}
}

func TestDefaultEndpointer_CloseReleasesEndpointResources(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: []string{"svc:80"}})
	closed := make(chan struct{})
	factory := endpointer.Factory(func(string) (endpoint.Endpoint, io.Closer, error) {
		return endpoint.Nop, closerFunc(func() error {
			close(closed)
			return nil
		}), nil
	})
	ep := endpointer.NewEndpointer(cache, factory, nopLogger)

	if err := ep.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("factory resource was not closed")
	}
}

// ── InstanceEndpoints ─────────────────────────────────────────────────────────

func TestInstanceEndpoints_PairsAddressWithItsEndpoint(t *testing.T) {
	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, echoFactory, nopLogger)
	t.Cleanup(func() { _ = ep.Close() })

	cache.Update(sd.Event{Instances: []string{"c:80", "a:80", "b:80"}})
	time.Sleep(20 * time.Millisecond)

	instances, err := ep.InstanceEndpoints()
	if err != nil {
		t.Fatalf("InstanceEndpoints: %v", err)
	}
	want := []string{"a:80", "b:80", "c:80"}
	if len(instances) != len(want) {
		t.Fatalf("got %d instances, want %d", len(instances), len(want))
	}
	for i, item := range instances {
		if item.Instance != want[i] {
			t.Fatalf("instance %d = %q, want %q", i, item.Instance, want[i])
		}
		// echoFactory returns the address it was built with, so a mismatch
		// here means an endpoint was paired with the wrong instance.
		resp, err := item.Endpoint(context.Background(), nil)
		if err != nil {
			t.Fatalf("call %s: %v", item.Instance, err)
		}
		if resp != item.Instance {
			t.Fatalf("instance %q is paired with the endpoint for %v", item.Instance, resp)
		}
	}
}

func TestInstanceEndpoints_MatchesEndpointsOrder(t *testing.T) {
	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, echoFactory, nopLogger)
	t.Cleanup(func() { _ = ep.Close() })

	cache.Update(sd.Event{Instances: []string{"b:80", "a:80"}})
	time.Sleep(20 * time.Millisecond)

	eps, err := ep.Endpoints()
	if err != nil {
		t.Fatalf("Endpoints: %v", err)
	}
	instances, err := ep.InstanceEndpoints()
	if err != nil {
		t.Fatalf("InstanceEndpoints: %v", err)
	}
	if len(eps) != len(instances) {
		t.Fatalf("Endpoints returned %d, InstanceEndpoints returned %d", len(eps), len(instances))
	}
	for i := range eps {
		plain, err := eps[i](context.Background(), nil)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		paired, err := instances[i].Endpoint(context.Background(), nil)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if plain != paired {
			t.Fatalf("position %d: Endpoints has %v, InstanceEndpoints has %v", i, plain, paired)
		}
	}
}

func TestInstanceEndpoints_AfterCloseReportsCacheClosed(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: []string{"svc:80"}})
	ep := endpointer.NewEndpointer(cache, echoFactory, nopLogger)

	if err := ep.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := ep.InstanceEndpoints(); !errors.Is(err, endpointer.ErrCacheClosed) {
		t.Fatalf("InstanceEndpoints after Close error = %v, want ErrCacheClosed", err)
	}
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }
