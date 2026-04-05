package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/endpoint"
	kitlog "github.com/dreamsxin/go-kit/log"
	"github.com/dreamsxin/go-kit/sd"
	"github.com/dreamsxin/go-kit/sd/endpointer"
	"github.com/dreamsxin/go-kit/sd/endpointer/balancer"
	"github.com/dreamsxin/go-kit/sd/endpointer/executor"
	"github.com/dreamsxin/go-kit/sd/events"
	"github.com/dreamsxin/go-kit/sd/instance"
	"github.com/dreamsxin/go-kit/sd/interfaces"
)

var nopLogger = kitlog.NewNopLogger()

func TestInstanceFactory_ReturnsAddr(t *testing.T) {
	ep, closer, err := instanceFactory("host:8080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer closer.Close()

	resp, err := ep(context.Background(), nil)
	if err != nil {
		t.Fatalf("endpoint error: %v", err)
	}
	if resp != "host:8080" {
		t.Errorf("got %q, want %q", resp, "host:8080")
	}
}

func TestRoundRobin_NoInstances(t *testing.T) {
	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, factory, nopLogger)
	lb := balancer.NewRoundRobin(ep)

	_, err := lb.Endpoint()
	if err == nil {
		t.Error("expected error with no instances")
	}
}

func TestRoundRobin_DistributesLoad(t *testing.T) {
	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, factory, nopLogger)
	lb := balancer.NewRoundRobin(ep)

	cache.Update(events.Event{Instances: []string{"A:80", "B:80"}})
	time.Sleep(20 * time.Millisecond)

	seen := map[string]int{}
	for i := 0; i < 4; i++ {
		e, err := lb.Endpoint()
		if err != nil {
			t.Fatalf("Endpoint() error: %v", err)
		}
		resp, _ := e(context.Background(), nil)
		seen[resp.(string)]++
	}
	if seen["A:80"] != 2 || seen["B:80"] != 2 {
		t.Errorf("uneven distribution: %v", seen)
	}
}

func TestRoundRobin_RemoveInstance(t *testing.T) {
	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, factory, nopLogger)
	lb := balancer.NewRoundRobin(ep)

	cache.Update(events.Event{Instances: []string{"A:80", "B:80"}})
	time.Sleep(20 * time.Millisecond)

	cache.Update(events.Event{Instances: []string{"A:80"}})
	time.Sleep(20 * time.Millisecond)

	for i := 0; i < 3; i++ {
		e, err := lb.Endpoint()
		if err != nil {
			t.Fatalf("Endpoint() error: %v", err)
		}
		resp, _ := e(context.Background(), nil)
		if resp != "A:80" {
			t.Errorf("expected A:80, got %v", resp)
		}
	}
}

func TestRetry_SucceedsAfterFailures(t *testing.T) {
	attempts := 0
	flakyFactory := endpoint.Factory(func(addr string) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			attempts++
			if attempts < 3 {
				return nil, fmt.Errorf("attempt %d failed", attempts)
			}
			return "success", nil
		})
		return ep, io.NopCloser(nil), nil
	})

	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"svc:80"}})
	time.Sleep(20 * time.Millisecond)

	ep := endpointer.NewEndpointer(cache, flakyFactory, nopLogger)
	lb := balancer.NewRoundRobin(ep)
	retryEp := executor.Retry(5, time.Second, lb)

	resp, err := retryEp(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "success" {
		t.Errorf("got %v, want success", resp)
	}
}

func TestRetry_ExceedsMaxAttempts(t *testing.T) {
	alwaysFail := endpoint.Factory(func(addr string) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			return nil, errors.New("always fails")
		})
		return ep, io.NopCloser(nil), nil
	})

	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"svc:80"}})
	time.Sleep(20 * time.Millisecond)

	ep := endpointer.NewEndpointer(cache, alwaysFail, nopLogger)
	lb := balancer.NewRoundRobin(ep)
	retryEp := executor.Retry(3, time.Second, lb)

	_, err := retryEp(context.Background(), nil)
	if err == nil {
		t.Error("expected error after max retries")
	}
}

func TestRetryWithCallback_StopsOnNonRetryable(t *testing.T) {
	sentinel := errors.New("non-retryable")
	callCount := 0

	flakyFactory := endpoint.Factory(func(addr string) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			callCount++
			if callCount == 1 {
				return nil, errors.New("transient")
			}
			return nil, sentinel
		})
		return ep, io.NopCloser(nil), nil
	})

	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"svc:80"}})
	time.Sleep(20 * time.Millisecond)

	ep := endpointer.NewEndpointer(cache, flakyFactory, nopLogger)
	lb := balancer.NewRoundRobin(ep)

	retryEp := executor.RetryWithCallback(time.Second, lb,
		func(n int, err error) (bool, error) {
			if errors.Is(err, sentinel) {
				return false, err
			}
			return true, nil
		},
	)

	_, err := retryEp(context.Background(), nil)
	// RetryWithCallback wraps errors in RetryError; check Final field
	var retryErr executor.RetryError
	if errors.As(err, &retryErr) {
		if !errors.Is(retryErr.Final, sentinel) {
			t.Errorf("expected sentinel as Final error, got %v", retryErr.Final)
		}
	} else if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
	if callCount != 2 {
		t.Errorf("callCount: got %d, want 2", callCount)
	}
}

func TestNewEndpoint_RoundRobins(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"svc1:80", "svc2:80", "svc3:80"}})
	time.Sleep(20 * time.Millisecond)

	ep := sd.NewEndpoint(cache, factory, nopLogger,
		sd.WithMaxRetries(3),
		sd.WithTimeout(500*time.Millisecond),
	)

	seen := map[string]bool{}
	for i := 0; i < 6; i++ {
		resp, err := ep(context.Background(), nil)
		if err != nil {
			t.Fatalf("call %d error: %v", i+1, err)
		}
		seen[resp.(string)] = true
	}
	if len(seen) < 2 {
		t.Errorf("expected multiple instances to be hit, got: %v", seen)
	}
}

func TestInvalidateOnError_ClearsCache(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(events.Event{Instances: []string{"svc:80"}})
	time.Sleep(20 * time.Millisecond)

	ep := endpointer.NewEndpointer(cache, factory, nopLogger,
		endpoint.InvalidateOnError(50*time.Millisecond),
	)
	lb := balancer.NewRoundRobin(ep)

	// healthy
	_, err := lb.Endpoint()
	if err != nil {
		t.Fatalf("expected healthy endpoint, got: %v", err)
	}

	// inject SD error
	cache.Update(events.Event{Err: errors.New("consul down")})
	time.Sleep(10 * time.Millisecond)

	// within grace period — still cached
	_, err = lb.Endpoint()
	if err != nil {
		t.Logf("grace period: %v (may be ok depending on timing)", err)
	}

	// after grace period — cache cleared
	time.Sleep(80 * time.Millisecond)
	_, err = lb.Endpoint()
	if err == nil {
		t.Error("expected error after cache invalidation")
	}
	if !errors.Is(err, interfaces.ErrNoEndpoints) && err != nil {
		t.Logf("got expected error: %v", err)
	}
}
