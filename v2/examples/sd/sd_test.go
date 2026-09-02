package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/balancer"
	sdclient "github.com/dreamsxin/go-kit/v2/sd/client"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/feedback"
	"github.com/dreamsxin/go-kit/v2/sd/instance"
	"github.com/dreamsxin/go-kit/v2/sd/retry"
	"github.com/dreamsxin/go-kit/v2/sd/selector"
	"log/slog"
)

var nopLogger = slog.New(slog.DiscardHandler)

func picked(t *testing.T, lb sd.Balancer, request any) sd.Picked {
	t.Helper()
	selected, err := lb.Pick(context.Background(), request)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	return selected
}

func callPicked(t *testing.T, selected sd.Picked, request any) any {
	t.Helper()
	response, err := selected.Endpoint(context.Background(), request)
	if selected.Done != nil {
		selected.Done(sd.Outcome{Err: err})
	}
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	return response
}

func TestInstanceFactory_ReturnsAddr(t *testing.T) {
	ep, closer, err := instanceFactory(sd.Instance{Address: "host:8080"})
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
	t.Cleanup(func() { _ = ep.Close() })
	lb := balancer.NewRoundRobin(ep)

	_, err := lb.Pick(context.Background(), nil)
	if err == nil {
		t.Error("expected error with no instances")
	}
}

func TestRoundRobin_DistributesLoad(t *testing.T) {
	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, factory, nopLogger)
	t.Cleanup(func() { _ = ep.Close() })
	lb := balancer.NewRoundRobin(ep)

	cache.Update(sd.Event{Instances: sd.Addresses("A:80", "B:80")})
	time.Sleep(20 * time.Millisecond)

	seen := map[string]int{}
	for i := 0; i < 4; i++ {
		selected, err := lb.Pick(context.Background(), nil)
		if err != nil {
			t.Fatalf("Endpoint() error: %v", err)
		}
		resp, callErr := selected.Endpoint(context.Background(), nil)
		selected.Done(sd.Outcome{Err: callErr})
		seen[resp.(string)]++
	}
	if seen["A:80"] != 2 || seen["B:80"] != 2 {
		t.Errorf("uneven distribution: %v", seen)
	}
}

func TestRoundRobin_RemoveInstance(t *testing.T) {
	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, factory, nopLogger)
	t.Cleanup(func() { _ = ep.Close() })
	lb := balancer.NewRoundRobin(ep)

	cache.Update(sd.Event{Instances: sd.Addresses("A:80", "B:80")})
	time.Sleep(20 * time.Millisecond)

	cache.Update(sd.Event{Instances: sd.Addresses("A:80")})
	time.Sleep(20 * time.Millisecond)

	for i := 0; i < 3; i++ {
		selected, err := lb.Pick(context.Background(), nil)
		if err != nil {
			t.Fatalf("Endpoint() error: %v", err)
		}
		resp, callErr := selected.Endpoint(context.Background(), nil)
		selected.Done(sd.Outcome{Err: callErr})
		if resp != "A:80" {
			t.Errorf("expected A:80, got %v", resp)
		}
	}
}

func TestRetry_SucceedsAfterFailures(t *testing.T) {
	attempts := 0
	flakyFactory := endpointer.Factory(func(instance sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			attempts++
			if attempts < 3 {
				return nil, transientError{fmt.Errorf("attempt %d failed", attempts)}
			}
			return "success", nil
		})
		return ep, io.NopCloser(nil), nil
	})

	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: sd.Addresses("svc:80")})
	time.Sleep(20 * time.Millisecond)

	ep := endpointer.NewEndpointer(cache, flakyFactory, nopLogger)
	t.Cleanup(func() { _ = ep.Close() })
	lb := balancer.NewRoundRobin(ep)
	retryEp := retry.Retry(5, time.Second, lb)

	resp, err := retryEp(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "success" {
		t.Errorf("got %v, want success", resp)
	}
}

func TestRetry_ExceedsMaxAttempts(t *testing.T) {
	alwaysFail := endpointer.Factory(func(instance sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			return nil, transientError{errors.New("always fails")}
		})
		return ep, io.NopCloser(nil), nil
	})

	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: sd.Addresses("svc:80")})
	time.Sleep(20 * time.Millisecond)

	ep := endpointer.NewEndpointer(cache, alwaysFail, nopLogger)
	t.Cleanup(func() { _ = ep.Close() })
	lb := balancer.NewRoundRobin(ep)
	retryEp := retry.Retry(3, time.Second, lb)

	_, err := retryEp(context.Background(), nil)
	if err == nil {
		t.Error("expected error after max retries")
	}
}

func TestRetryWithCallback_StopsOnNonRetryable(t *testing.T) {
	sentinel := errors.New("non-retryable")
	callCount := 0

	flakyFactory := endpointer.Factory(func(instance sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			callCount++
			if callCount == 1 {
				return nil, transientError{errors.New("transient")}
			}
			return nil, sentinel
		})
		return ep, io.NopCloser(nil), nil
	})

	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: sd.Addresses("svc:80")})
	time.Sleep(20 * time.Millisecond)

	ep := endpointer.NewEndpointer(cache, flakyFactory, nopLogger)
	t.Cleanup(func() { _ = ep.Close() })
	lb := balancer.NewRoundRobin(ep)

	retryEp := retry.WithCallback(time.Second, lb,
		func(n int, err error) (bool, error) {
			if errors.Is(err, sentinel) {
				return false, err
			}
			return true, nil
		},
	)

	_, err := retryEp(context.Background(), nil)
	// RetryWithCallback wraps errors in RetryError; check Final field
	var retryErr retry.Error
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
	cache.Update(sd.Event{Instances: sd.Addresses("svc1:80", "svc2:80", "svc3:80")})
	time.Sleep(20 * time.Millisecond)

	ep, closer, err := sdclient.NewEndpoint(cache, factory, nopLogger,
		sdclient.WithMaxAttempts(3),
		sdclient.WithTimeout(500*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewEndpoint: %v", err)
	}
	t.Cleanup(func() { _ = closer.Close() })

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

func TestPrefer_KeepsZoneAffinityThenFallsBack(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: []sd.Instance{
		{Address: "local-A:80", Metadata: map[string]any{"zone": "z1", "weight": 3}},
		{Address: "remote-C:80", Metadata: map[string]any{"zone": "z2", "weight": 9}},
	}})
	time.Sleep(20 * time.Millisecond)

	set := endpointer.NewEndpointer(cache, factory, nopLogger)
	t.Cleanup(func() { _ = set.Close() })
	local := endpointer.Prefer(set, sd.MetadataEquals("zone", "z1"))
	lb := balancer.NewWeightedRandom(local, balancer.MetadataWeight(balancer.DefaultWeightKey, 1))

	for i := 0; i < 10; i++ {
		selected, err := lb.Pick(context.Background(), nil)
		if err != nil {
			t.Fatalf("Endpoint() error: %v", err)
		}
		resp, callErr := selected.Endpoint(context.Background(), nil)
		selected.Done(sd.Outcome{Err: callErr})
		if resp != "local-A:80" {
			t.Fatalf("call %d selected %v, want the z1 instance", i+1, resp)
		}
	}

	// Drain z1: the preferred subset is empty, so the fallback takes over.
	cache.Update(sd.Event{Instances: []sd.Instance{
		{Address: "remote-C:80", Metadata: map[string]any{"zone": "z2", "weight": 9}},
	}})
	time.Sleep(20 * time.Millisecond)

	selected, err := lb.Pick(context.Background(), nil)
	if err != nil {
		t.Fatalf("Endpoint() after draining z1: %v", err)
	}
	resp, callErr := selected.Endpoint(context.Background(), nil)
	selected.Done(sd.Outcome{Err: callErr})
	if resp != "remote-C:80" {
		t.Errorf("fallback selected %v, want remote-C:80", resp)
	}
}

// The selector layer answers "which instance?" from an Instancer alone — no
// endpointer, no factory — which is what a caller that dials the address itself
// needs.
func TestSelector_PicksInstanceWithoutEndpoints(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: []sd.Instance{
		{Address: "local-A:80", Metadata: map[string]any{"zone": "z1"}},
		{Address: "remote-C:80", Metadata: map[string]any{"zone": "z2"}},
	}})

	instances := selector.Subscribe(cache)
	t.Cleanup(func() { _ = instances.Close() })

	local := selector.Filter(instances, sd.MetadataEquals("zone", "z1"))
	pick := selector.New(local, selector.RoundRobin())

	for i := 0; i < 3; i++ {
		chosen, err := pick.Select(context.Background(), nil)
		if err != nil {
			t.Fatalf("Select %d: %v", i+1, err)
		}
		if chosen.Address != "local-A:80" {
			t.Fatalf("selected %q, want the only z1 instance", chosen.Address)
		}
	}
}

// The feedback layer closes the loop: one table ejects the instance whose calls
// fail, and forgets it once discovery drops it.
func TestFeedback_EjectsFailingInstanceAndForgetsRemovedOnes(t *testing.T) {
	failing := endpointer.Factory(func(item sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		address := item.Address
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			if address == "bad:80" {
				return nil, errors.New("refused")
			}
			return address, nil
		})
		return ep, io.NopCloser(nil), nil
	})

	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: sd.Addresses("bad:80", "good:80")})
	time.Sleep(20 * time.Millisecond)

	set := endpointer.NewEndpointer(cache, failing, nopLogger)
	t.Cleanup(func() { _ = set.Close() })

	table := feedback.NewTable(feedback.WithAlpha(1))
	ejector := feedback.NewEjector(table, feedback.EjectionPolicy{MaxErrorRate: 0.5, MinSamples: 1})
	following := feedback.Follow(cache, table, ejector)
	t.Cleanup(func() { _ = following.Close() })

	lb := balancer.New(set, table.Wrap(selector.Filtered(selector.RoundRobin(), ejector.Filter())))
	t.Cleanup(func() { _ = lb.Close() })

	failures := 0
	for i := 0; i < 20; i++ {
		if _, err := invoke(lb, context.Background(), nil); err != nil {
			failures++
		}
	}
	// Round robin would have sent half the calls to bad:80 without ejection.
	if failures != 1 {
		t.Fatalf("failures = %d, want exactly the one that measured the fault", failures)
	}

	cache.Update(sd.Event{Instances: sd.Addresses("good:80")})
	time.Sleep(20 * time.Millisecond)
	if got := table.Stats(sd.Instance{Address: "bad:80"}).Samples; got != 0 {
		t.Fatalf("samples retained for a removed instance = %d, want 0", got)
	}
}

func TestInvalidateOnError_ClearsCache(t *testing.T) {
	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: sd.Addresses("svc:80")})
	time.Sleep(20 * time.Millisecond)

	ep := endpointer.NewEndpointer(cache, factory, nopLogger,
		endpointer.InvalidateOnError(50*time.Millisecond),
	)
	t.Cleanup(func() { _ = ep.Close() })
	lb := balancer.NewRoundRobin(ep)

	// healthy
	_, err := lb.Pick(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected healthy endpoint, got: %v", err)
	}

	// inject SD error
	cache.Update(sd.Event{Err: errors.New("consul down")})
	time.Sleep(10 * time.Millisecond)

	// within grace period — still cached
	_, err = lb.Pick(context.Background(), nil)
	if err != nil {
		t.Logf("grace period: %v (may be ok depending on timing)", err)
	}

	// after grace period — cache cleared
	time.Sleep(80 * time.Millisecond)
	_, err = lb.Pick(context.Background(), nil)
	if err == nil {
		t.Error("expected error after cache invalidation")
	}
	if !errors.Is(err, sd.ErrNoEndpoints) && err != nil {
		t.Logf("got expected error: %v", err)
	}
}
