package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/balancer"
	sdclient "github.com/dreamsxin/go-kit/v2/sd/client"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
	"github.com/dreamsxin/go-kit/v2/sd/feedback"
	"github.com/dreamsxin/go-kit/v2/sd/health"
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
		chosen, done, err := pick.Select(context.Background(), nil)
		if err != nil {
			t.Fatalf("Select %d: %v", i+1, err)
		}
		done(sd.Outcome{})
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

// eventually polls until condition holds, so the health tests do not depend on
// how long a probe round takes.
func eventually(t *testing.T, condition func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func published(t *testing.T, source selector.Source) []string {
	t.Helper()
	items, err := source.Instances()
	if err != nil {
		return nil
	}
	list := make([]string, 0, len(items))
	for _, item := range items {
		list = append(list, item.Address)
	}
	sort.Strings(list)
	return list
}

// Active probing covers what passive feedback cannot: an instance nothing has
// called is never measured, so nothing can eject it.
func TestHealth_DropsUnreachableThenFailsOpen(t *testing.T) {
	var mu sync.Mutex
	down := map[string]bool{"unreachable:80": true}
	probe := health.Probe(func(_ context.Context, target sd.Instance) error {
		mu.Lock()
		defer mu.Unlock()
		if down[target.Address] {
			return errors.New("connection refused")
		}
		return nil
	})

	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: sd.Addresses("serving-A:80", "serving-B:80", "unreachable:80")})

	checked := health.Check(cache, probe,
		health.WithInterval(5*time.Millisecond),
		health.WithUnhealthyThreshold(1),
		health.WithLogger(nopLogger))
	t.Cleanup(func() { _ = checked.Close() })

	instances := selector.Subscribe(checked)
	t.Cleanup(func() { _ = instances.Close() })

	if !eventually(t, func() bool { return len(published(t, instances)) == 2 }) {
		t.Fatalf("unreachable instance was never withdrawn: %v", published(t, instances))
	}
	if got := published(t, instances); got[0] != "serving-A:80" || got[1] != "serving-B:80" {
		t.Errorf("published %v, want only the reachable pair", got)
	}

	// A probe failing for everything is far more likely broken than the whole
	// service being down, so the unchecked set is published rather than none.
	mu.Lock()
	down["serving-A:80"] = true
	down["serving-B:80"] = true
	mu.Unlock()

	if !eventually(t, func() bool { return len(published(t, instances)) == 3 }) {
		t.Fatalf("no fail-open when every probe failed: %v", published(t, instances))
	}
}

// A ranker answers "which N", for a caller that dials the instance itself.
func TestRanker_ShortlistsScoredInstancesOnly(t *testing.T) {
	pool := selector.Static(sd.Addresses("edge-A:443", "edge-B:443", "edge-C:443", "edge-D:443")...)
	score := map[string]float64{"edge-A:443": 0.2, "edge-B:443": 0.9, "edge-C:443": 0.5}
	rank := selector.NewRanker(pool, func(_ context.Context, _ any, item sd.Instance) (float64, bool) {
		value, known := score[item.Address]
		return value, known
	})

	best, err := rank.Rank(context.Background(), nil, 2)
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	if len(best) != 2 || best[0].Address != "edge-B:443" || best[1].Address != "edge-C:443" {
		t.Fatalf("best 2 = %v, want edge-B then edge-C", best)
	}

	every, err := rank.Rank(context.Background(), nil, 0)
	if err != nil {
		t.Fatalf("Rank(0): %v", err)
	}
	if len(every) != 3 {
		t.Fatalf("n <= 0 returned %d candidates, want the 3 that have a score", len(every))
	}
	for _, item := range every {
		if item.Address == "edge-D:443" {
			t.Error("an instance that refused a score became a candidate")
		}
	}
}

// Without a ramp, a new instance looks best to every load-aware strategy — it
// has no in-flight requests and no latency history — and takes full traffic
// while it is still cold.
func TestSlowStart_RampsRecentlySeenInstances(t *testing.T) {
	const window = time.Minute
	seen := map[string]time.Time{
		"warm:80":    time.Now().Add(-window),
		"warming:80": time.Now().Add(-window / 4),
		"cold:80":    time.Now(),
	}
	first := selector.FirstSeenFunc(func(item sd.Instance) (time.Time, bool) {
		at, known := seen[item.Address]
		return at, known
	})
	weight := selector.SlowStart(selector.MetadataWeight("weight", 10), first, window)

	for address, want := range map[string]int{
		"warm:80":    10,
		"warming:80": 2,
		"cold:80":    1,
		"unknown:80": 1, // never seen, so treated as brand new
	} {
		if got := weight(sd.Instance{Address: address}); got != want {
			t.Errorf("%s weight = %d, want %d", address, got, want)
		}
	}

	// Zero means "never pick me"; ramping it would contradict the operator.
	drained := sd.Instance{Address: "drained:80", Metadata: map[string]any{"weight": 0}}
	if got := weight(drained); got != 0 {
		t.Errorf("drained weight = %d, want 0", got)
	}
}

// Draining is a property of the registration, not something measured, so it
// lives in metadata and is read by a filter.
func TestDraining_WithheldFromNewWorkAndReturnsOnReady(t *testing.T) {
	pool := func(state string) []sd.Instance {
		return []sd.Instance{
			{Address: "ready-A:80", Metadata: map[string]any{sd.StateKey: sd.StateReady}},
			{Address: "leaving:80", Metadata: map[string]any{sd.StateKey: state}},
		}
	}

	count := func(state string) map[string]int {
		pick := selector.New(selector.Static(pool(state)...),
			selector.Filtered(selector.RoundRobin(), sd.Keep(sd.Serving())))
		seen := map[string]int{}
		for i := 0; i < 4; i++ {
			chosen, done, err := pick.Select(context.Background(), nil)
			if err != nil {
				t.Fatalf("Select %d: %v", i+1, err)
			}
			done(sd.Outcome{})
			seen[chosen.Address]++
		}
		return seen
	}

	draining := count(sd.StateDraining)
	if draining["leaving:80"] != 0 {
		t.Errorf("a draining instance took %d new calls, want 0", draining["leaving:80"])
	}
	if draining["ready-A:80"] != 4 {
		t.Errorf("ready-A:80 served %d of 4 calls", draining["ready-A:80"])
	}

	ready := count(sd.StateReady)
	if ready["leaving:80"] == 0 {
		t.Error("flipping the label back did not bring the instance back into rotation")
	}
}
