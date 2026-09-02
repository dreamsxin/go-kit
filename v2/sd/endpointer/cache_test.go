package endpointer

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"log/slog"
)

type nopCloser struct{ closed bool }

func (n *nopCloser) Close() error { n.closed = true; return nil }

func makeFactory(instances map[string]endpoint.Endpoint) Factory {
	return func(instance sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		if ep, ok := instances[instance.Address]; ok {
			return ep, &nopCloser{}, nil
		}
		return nil, nil, errors.New("unknown instance: " + instance.Address)
	}
}

func TestCacheUpdateAndEndpoints(t *testing.T) {
	ep1 := func(context.Context, any) (any, error) { return "svc1", nil }
	ep2 := func(context.Context, any) (any, error) { return "svc2", nil }
	cache := NewCache(makeFactory(map[string]endpoint.Endpoint{
		"host1:8080": ep1,
		"host2:8080": ep2,
	}), slog.New(slog.DiscardHandler), Options{})

	cache.Update(sd.Event{Instances: sd.Addresses("host1:8080", "host2:8080")})
	endpoints, err := cache.Endpoints()
	if err != nil || len(endpoints) != 2 {
		t.Fatalf("Endpoints = %v, %v; want two endpoints", endpoints, err)
	}
	endpoints[0] = nil
	again, err := cache.Endpoints()
	if err != nil || again[0] == nil {
		t.Fatalf("Endpoints exposed internal slice: endpoints=%v err=%v", again, err)
	}
}

func TestCacheUpdateRemovesOld(t *testing.T) {
	closerA := &nopCloser{}
	factoryCalls := 0
	cache := NewCache(func(instance sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		factoryCalls++
		if instance.Address == "A" {
			return endpoint.Nop, closerA, nil
		}
		return endpoint.Nop, &nopCloser{}, nil
	}, slog.New(slog.DiscardHandler), Options{})

	cache.Update(sd.Event{Instances: sd.Addresses("A")})
	cache.Update(sd.Event{Instances: sd.Addresses("B")})
	if factoryCalls != 2 || !closerA.closed {
		t.Fatalf("factory calls=%d closerA.closed=%v", factoryCalls, closerA.closed)
	}
}

func TestCacheReusesSameInstance(t *testing.T) {
	factoryCalls := 0
	cache := NewCache(func(sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		factoryCalls++
		return endpoint.Nop, &nopCloser{}, nil
	}, slog.New(slog.DiscardHandler), Options{})

	cache.Update(sd.Event{Instances: sd.Addresses("host:80")})
	cache.Update(sd.Event{Instances: sd.Addresses("host:80")})
	if factoryCalls != 1 {
		t.Fatalf("factory calls = %d, want 1", factoryCalls)
	}
}

func TestCacheDoesNotMutateInstances(t *testing.T) {
	instances := sd.Addresses("b:80", "a:80")
	cache := NewCache(func(sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		return endpoint.Nop, nil, nil
	}, slog.New(slog.DiscardHandler), Options{})

	cache.Update(sd.Event{Instances: instances})
	if instances[0].Address != "b:80" || instances[1].Address != "a:80" {
		t.Fatalf("Update mutated caller instances: %v", instances)
	}
}

// A relabel must not tear down a working connection: the endpoint is reused and
// only the published labels change.
func TestCacheRelabelReusesEndpoint(t *testing.T) {
	factoryCalls := 0
	cache := NewCache(func(sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		factoryCalls++
		return endpoint.Nop, &nopCloser{}, nil
	}, slog.New(slog.DiscardHandler), Options{})

	cache.Update(sd.Event{Instances: []sd.Instance{
		{Address: "svc:80", Metadata: map[string]any{"zone": "north"}},
	}})
	cache.Update(sd.Event{Instances: []sd.Instance{
		{Address: "svc:80", Metadata: map[string]any{"zone": "south"}},
	}})

	if factoryCalls != 1 {
		t.Fatalf("factory called %d times, want 1", factoryCalls)
	}
	instances, err := cache.InstanceEndpoints()
	if err != nil {
		t.Fatalf("InstanceEndpoints: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("got %d instances, want 1", len(instances))
	}
	if zone, _ := sd.MetadataString(instances[0].Metadata(), "zone"); zone != "south" {
		t.Fatalf("published zone = %q, want south", zone)
	}
}

// One closer is held per address, so a snapshot repeating an address must not
// build a second endpoint whose closer would then be unreachable.
func TestCacheSkipsDuplicateAddresses(t *testing.T) {
	factoryCalls := 0
	cache := NewCache(func(sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		factoryCalls++
		return endpoint.Nop, &nopCloser{}, nil
	}, slog.New(slog.DiscardHandler), Options{})

	cache.Update(sd.Event{Instances: sd.Addresses("svc:80", "svc:80")})

	if factoryCalls != 1 {
		t.Fatalf("factory called %d times, want 1", factoryCalls)
	}
	instances, err := cache.InstanceEndpoints()
	if err != nil {
		t.Fatalf("InstanceEndpoints: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("got %d instances, want the duplicate collapsed", len(instances))
	}
}

func TestCacheCarriesMetadataToInstanceEndpoints(t *testing.T) {
	cache := NewCache(func(sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		return endpoint.Nop, nil, nil
	}, slog.New(slog.DiscardHandler), Options{})

	cache.Update(sd.Event{Instances: []sd.Instance{
		{Address: "svc:80", Metadata: map[string]any{"zone": "north", "weight": "10"}},
	}})

	instances, err := cache.InstanceEndpoints()
	if err != nil {
		t.Fatalf("InstanceEndpoints: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("got %d instances, want 1", len(instances))
	}
	if got := instances[0].Address(); got != "svc:80" {
		t.Fatalf("Address() = %q, want svc:80", got)
	}
	if zone, ok := sd.MetadataString(instances[0].Metadata(), "zone"); !ok || zone != "north" {
		t.Fatalf("zone = %q, %v; want north, true", zone, ok)
	}
	if weight, ok := sd.MetadataInt(instances[0].Metadata(), "weight"); !ok || weight != 10 {
		t.Fatalf("weight = %d, %v; want 10, true", weight, ok)
	}
}

func TestCacheErrorEventWithoutInvalidation(t *testing.T) {
	cache := NewCache(makeFactory(map[string]endpoint.Endpoint{"h:1": endpoint.Nop}), slog.New(slog.DiscardHandler), Options{})
	cache.Update(sd.Event{Instances: sd.Addresses("h:1")})
	cache.Update(sd.Event{Err: errors.New("consul down")})

	endpoints, err := cache.Endpoints()
	if err != nil || len(endpoints) != 1 {
		t.Fatalf("Endpoints = %v, %v; want cached endpoint", endpoints, err)
	}
}

func TestCacheErrorEventWithInvalidation(t *testing.T) {
	timeout := 20 * time.Millisecond
	cache := NewCache(makeFactory(map[string]endpoint.Endpoint{"h:1": endpoint.Nop}), slog.New(slog.DiscardHandler), Options{
		InvalidateOnError: true,
		InvalidateTimeout: timeout,
	})
	cache.Update(sd.Event{Instances: sd.Addresses("h:1")})
	cache.Update(sd.Event{Err: errors.New("sd error")})

	if endpoints, err := cache.Endpoints(); err != nil || len(endpoints) != 1 {
		t.Fatalf("before deadline: endpoints=%v err=%v", endpoints, err)
	}
	time.Sleep(timeout + 20*time.Millisecond)
	if endpoints, err := cache.Endpoints(); err == nil || len(endpoints) != 0 {
		t.Fatalf("after deadline: endpoints=%v err=%v", endpoints, err)
	}
}

func TestCacheEmptyUpdate(t *testing.T) {
	cache := NewCache(makeFactory(map[string]endpoint.Endpoint{"h:1": endpoint.Nop}), slog.New(slog.DiscardHandler), Options{})
	cache.Update(sd.Event{Instances: sd.Addresses("h:1")})
	cache.Update(sd.Event{})
	if endpoints, err := cache.Endpoints(); err != nil || len(endpoints) != 0 {
		t.Fatalf("Endpoints = %v, %v; want empty cache", endpoints, err)
	}
}

func TestCacheCloseReleasesResourcesAndRejectsUpdates(t *testing.T) {
	closeErr := errors.New("close failed")
	closed := 0
	factoryCalls := 0
	cache := NewCache(func(sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		factoryCalls++
		return endpoint.Nop, closerFunc(func() error {
			closed++
			if closed == 1 {
				return closeErr
			}
			return nil
		}), nil
	}, slog.New(slog.DiscardHandler), Options{})

	cache.Update(sd.Event{Instances: sd.Addresses("a:80", "b:80")})
	if err := cache.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("Close error = %v, want joined close error", err)
	}
	if closed != 2 {
		t.Fatalf("closed resources = %d, want 2", closed)
	}
	if err := cache.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := cache.Endpoints(); !errors.Is(err, ErrCacheClosed) {
		t.Fatalf("Endpoints error = %v, want ErrCacheClosed", err)
	}
	cache.Update(sd.Event{Instances: sd.Addresses("c:80")})
	if factoryCalls != 2 {
		t.Fatalf("factory called after Close: %d calls", factoryCalls)
	}
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }
