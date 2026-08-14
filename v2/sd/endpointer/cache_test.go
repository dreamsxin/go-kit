package endpointer

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	kitlog "github.com/dreamsxin/go-kit/v2/log"
	"github.com/dreamsxin/go-kit/v2/sd"
)

type nopCloser struct{ closed bool }

func (n *nopCloser) Close() error { n.closed = true; return nil }

func makeFactory(instances map[string]endpoint.Endpoint) Factory {
	return func(instance string) (endpoint.Endpoint, io.Closer, error) {
		if ep, ok := instances[instance]; ok {
			return ep, &nopCloser{}, nil
		}
		return nil, nil, errors.New("unknown instance: " + instance)
	}
}

func TestCacheUpdateAndEndpoints(t *testing.T) {
	ep1 := func(context.Context, any) (any, error) { return "svc1", nil }
	ep2 := func(context.Context, any) (any, error) { return "svc2", nil }
	cache := NewCache(makeFactory(map[string]endpoint.Endpoint{
		"host1:8080": ep1,
		"host2:8080": ep2,
	}), kitlog.NewNopLogger(), Options{})

	cache.Update(sd.Event{Instances: []string{"host1:8080", "host2:8080"}})
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
	cache := NewCache(func(instance string) (endpoint.Endpoint, io.Closer, error) {
		factoryCalls++
		if instance == "A" {
			return endpoint.Nop, closerA, nil
		}
		return endpoint.Nop, &nopCloser{}, nil
	}, kitlog.NewNopLogger(), Options{})

	cache.Update(sd.Event{Instances: []string{"A"}})
	cache.Update(sd.Event{Instances: []string{"B"}})
	if factoryCalls != 2 || !closerA.closed {
		t.Fatalf("factory calls=%d closerA.closed=%v", factoryCalls, closerA.closed)
	}
}

func TestCacheReusesSameInstance(t *testing.T) {
	factoryCalls := 0
	cache := NewCache(func(string) (endpoint.Endpoint, io.Closer, error) {
		factoryCalls++
		return endpoint.Nop, &nopCloser{}, nil
	}, kitlog.NewNopLogger(), Options{})

	cache.Update(sd.Event{Instances: []string{"host:80"}})
	cache.Update(sd.Event{Instances: []string{"host:80"}})
	if factoryCalls != 1 {
		t.Fatalf("factory calls = %d, want 1", factoryCalls)
	}
}

func TestCacheDoesNotMutateInstances(t *testing.T) {
	instances := []string{"b:80", "a:80"}
	cache := NewCache(func(string) (endpoint.Endpoint, io.Closer, error) {
		return endpoint.Nop, nil, nil
	}, kitlog.NewNopLogger(), Options{})

	cache.Update(sd.Event{Instances: instances})
	if instances[0] != "b:80" || instances[1] != "a:80" {
		t.Fatalf("Update mutated caller instances: %v", instances)
	}
}

func TestCacheErrorEventWithoutInvalidation(t *testing.T) {
	cache := NewCache(makeFactory(map[string]endpoint.Endpoint{"h:1": endpoint.Nop}), kitlog.NewNopLogger(), Options{})
	cache.Update(sd.Event{Instances: []string{"h:1"}})
	cache.Update(sd.Event{Err: errors.New("consul down")})

	endpoints, err := cache.Endpoints()
	if err != nil || len(endpoints) != 1 {
		t.Fatalf("Endpoints = %v, %v; want cached endpoint", endpoints, err)
	}
}

func TestCacheErrorEventWithInvalidation(t *testing.T) {
	timeout := 20 * time.Millisecond
	cache := NewCache(makeFactory(map[string]endpoint.Endpoint{"h:1": endpoint.Nop}), kitlog.NewNopLogger(), Options{
		InvalidateOnError: true,
		InvalidateTimeout: timeout,
	})
	cache.Update(sd.Event{Instances: []string{"h:1"}})
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
	cache := NewCache(makeFactory(map[string]endpoint.Endpoint{"h:1": endpoint.Nop}), kitlog.NewNopLogger(), Options{})
	cache.Update(sd.Event{Instances: []string{"h:1"}})
	cache.Update(sd.Event{})
	if endpoints, err := cache.Endpoints(); err != nil || len(endpoints) != 0 {
		t.Fatalf("Endpoints = %v, %v; want empty cache", endpoints, err)
	}
}

func TestCacheCloseReleasesResourcesAndRejectsUpdates(t *testing.T) {
	closeErr := errors.New("close failed")
	closed := 0
	factoryCalls := 0
	cache := NewCache(func(string) (endpoint.Endpoint, io.Closer, error) {
		factoryCalls++
		return endpoint.Nop, closerFunc(func() error {
			closed++
			if closed == 1 {
				return closeErr
			}
			return nil
		}), nil
	}, kitlog.NewNopLogger(), Options{})

	cache.Update(sd.Event{Instances: []string{"a:80", "b:80"}})
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
	cache.Update(sd.Event{Instances: []string{"c:80"}})
	if factoryCalls != 2 {
		t.Fatalf("factory called after Close: %d calls", factoryCalls)
	}
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }
