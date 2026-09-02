// Package main demonstrates the service-discovery (sd) components
// without any external dependency (no Consul, no network):
//
//   - sd/instance.Cache        — in-memory Instancer for testing
//   - sd/endpointer            — wires Instancer → EndpointCache
//   - sd/endpointer.Prefer — metadata filtering with fallback
//   - sd/selector              — pick an instance without building endpoints
//   - sd/balancer              — lock-free RoundRobin, weighted random
//   - sd/retry                 — Retry, WithCallback
//   - sd/client.NewEndpoint    — one-liner that wires everything together
//   - endpointer.InvalidateOnError — cache invalidation on SD errors
//
// Run:
//
//	go run ./examples/sd
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
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

type transientError struct {
	error
}

func (transientError) Retryable() bool { return true }

// ── Factory helper ────────────────────────────────────────────────────────────

// instanceFactory returns an Endpoint that echoes the instance address.
func instanceFactory(instance sd.Instance) (endpoint.Endpoint, io.Closer, error) {
	address := instance.Address
	ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		return address, nil
	})
	return ep, io.NopCloser(nil), nil
}

var factory = endpointer.Factory(instanceFactory)

func invoke(lb sd.Balancer, ctx context.Context, request any) (any, error) {
	picked, err := lb.Pick(ctx, request)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	response, err := picked.Endpoint(ctx, request)
	if picked.Done != nil {
		picked.Done(sd.Outcome{Err: err, Latency: time.Since(started)})
	}
	return response, err
}

// ── Demo 1: instance.Cache + Endpointer + RoundRobin ─────────────────────────

func demo1_RoundRobin(logger *slog.Logger) {
	fmt.Println("=== 1. instance.Cache + Endpointer + RoundRobin ===")

	cache := instance.NewCache()
	ep := endpointer.NewEndpointer(cache, factory, logger)
	defer ep.Close() //nolint:errcheck
	lb := balancer.NewRoundRobin(ep)

	// No instances yet
	_, err := lb.Pick(context.Background(), nil)
	fmt.Printf("  no instances: %v\n", err) // ErrNoEndpoints

	// Register two instances
	cache.Update(sd.Event{Instances: sd.Addresses("host-A:8080", "host-B:8080")})
	time.Sleep(10 * time.Millisecond) // let the goroutine process

	fmt.Println("  round-robin over 4 calls:")
	for i := 0; i < 4; i++ {
		resp, _ := invoke(lb, context.Background(), nil)
		fmt.Printf("    call %d → %s\n", i+1, resp)
	}

	// Remove one instance
	cache.Update(sd.Event{Instances: sd.Addresses("host-A:8080")})
	time.Sleep(10 * time.Millisecond)

	resp, _ := invoke(lb, context.Background(), nil)
	fmt.Printf("  after removing host-B: %s\n", resp)
}

// ── Demo 2: retry.Retry ───────────────────────────────────────────────────────

func demo2_Retry(logger *slog.Logger) {
	fmt.Println("\n=== 2. retry.Retry (max 3 attempts) ===")

	attempts := 0
	flakyFactory := endpointer.Factory(func(instance sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			attempts++
			if attempts < 3 {
				return nil, transientError{fmt.Errorf("attempt %d failed", attempts)}
			}
			return fmt.Sprintf("success on attempt %d", attempts), nil
		})
		return ep, io.NopCloser(nil), nil
	})

	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: sd.Addresses("svc:80")})
	time.Sleep(10 * time.Millisecond)

	ep := endpointer.NewEndpointer(cache, flakyFactory, logger)
	defer ep.Close() //nolint:errcheck
	lb := balancer.NewRoundRobin(ep)
	retryEp := retry.Retry(5, time.Second, lb)

	resp, err := retryEp(context.Background(), nil)
	fmt.Printf("  result: %v, err: %v\n", resp, err)
}

// ── Demo 3: retry.WithCallback ────────────────────────────────────────────────

func demo3_RetryWithCallback(logger *slog.Logger) {
	fmt.Println("\n=== 3. retry.WithCallback ===")

	var sentinelErr = errors.New("non-retryable")
	callCount := 0

	flakyFactory := endpointer.Factory(func(instance sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			callCount++
			switch callCount {
			case 1:
				return nil, transientError{errors.New("transient error")}
			case 2:
				return nil, sentinelErr // non-retryable
			default:
				return "ok", nil
			}
		})
		return ep, io.NopCloser(nil), nil
	})

	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: sd.Addresses("svc:80")})
	time.Sleep(10 * time.Millisecond)

	ep := endpointer.NewEndpointer(cache, flakyFactory, logger)
	defer ep.Close() //nolint:errcheck
	lb := balancer.NewRoundRobin(ep)

	retryEp := retry.WithCallback(time.Second, lb,
		func(n int, err error) (keepTrying bool, replacement error) {
			if errors.Is(err, sentinelErr) {
				fmt.Printf("  attempt %d: non-retryable error, stopping\n", n)
				return false, err
			}
			fmt.Printf("  attempt %d: transient error, retrying\n", n)
			return true, nil
		},
	)

	_, err := retryEp(context.Background(), nil)
	fmt.Printf("  final error: %v\n", err)
}

// ── Demo 4: sd/client.NewEndpoint (one-liner) ────────────────────────────────

func demo4_NewEndpoint(logger *slog.Logger) {
	fmt.Println("\n=== 4. sd/client.NewEndpoint (one-liner) ===")

	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: sd.Addresses("svc1:80", "svc2:80", "svc3:80")})
	time.Sleep(10 * time.Millisecond)

	ep, closer, err := sdclient.NewEndpoint(cache, factory, logger,
		sdclient.WithMaxAttempts(3),
		sdclient.WithTimeout(500*time.Millisecond),
	)
	if err != nil {
		fmt.Printf("  construct endpoint: %v\n", err)
		return
	}
	defer closer.Close() //nolint:errcheck

	fmt.Println("  5 calls via sd/client.NewEndpoint:")
	for i := 0; i < 5; i++ {
		resp, err := ep(context.Background(), nil)
		fmt.Printf("    call %d → %v (err=%v)\n", i+1, resp, err)
	}
}

// ── Demo 5: InvalidateOnError ─────────────────────────────────────────────────

func demo5_InvalidateOnError(logger *slog.Logger) {
	fmt.Println("\n=== 5. endpointer.InvalidateOnError ===")

	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: sd.Addresses("svc:80")})
	time.Sleep(10 * time.Millisecond)

	ep := endpointer.NewEndpointer(cache, factory, logger,
		endpointer.InvalidateOnError(50*time.Millisecond),
	)
	defer ep.Close() //nolint:errcheck
	lb := balancer.NewRoundRobin(ep)

	// Healthy call
	picked, err := lb.Pick(context.Background(), nil)
	fmt.Printf("  before error: endpoint=%v err=%v\n", picked.Endpoint != nil, err)

	// Simulate SD error
	cache.Update(sd.Event{Err: errors.New("consul down")})
	time.Sleep(10 * time.Millisecond)

	// Within grace period — still returns cached endpoints
	picked, err = lb.Pick(context.Background(), nil)
	fmt.Printf("  during grace period: endpoint=%v err=%v\n", picked.Endpoint != nil, err)

	// After grace period — cache is cleared
	time.Sleep(80 * time.Millisecond)
	_, err = lb.Pick(context.Background(), nil)
	fmt.Printf("  after grace period: err=%v\n", err)
	if errors.Is(err, sd.ErrNoEndpoints) || err != nil {
		fmt.Println("  cache invalidated as expected")
	}
}

// ── Demo 6: metadata-driven routing ───────────────────────────────────────────

// demo6_Metadata shows the two things registration labels buy you: filtering the
// set (zone affinity) and weighting the pick, without a new balancer for either.
func demo6_Metadata(logger *slog.Logger) {
	fmt.Println("\n=== 6. instance metadata: PreferSubset + weighted pick ===")

	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: []sd.Instance{
		{Address: "local-A:80", Metadata: map[string]any{"zone": "z1", "weight": 3}},
		{Address: "local-B:80", Metadata: map[string]any{"zone": "z1", "weight": 1}},
		{Address: "remote-C:80", Metadata: map[string]any{"zone": "z2", "weight": 9}},
	}})
	time.Sleep(10 * time.Millisecond)

	set := endpointer.NewEndpointer(cache, factory, logger)
	defer set.Close() //nolint:errcheck

	// Prefer the local zone, but fall back to the whole set when it empties out.
	local := endpointer.Prefer(set, sd.MetadataEquals("zone", "z1"))
	lb := balancer.NewWeightedRandom(local, balancer.MetadataWeight(balancer.DefaultWeightKey, 1))

	seen := map[string]int{}
	for i := 0; i < 100; i++ {
		resp, err := invoke(lb, context.Background(), nil)
		if err != nil {
			fmt.Printf("  select: %v\n", err)
			return
		}
		seen[resp.(string)]++
	}
	fmt.Printf("  100 calls, zone z1 only, weights 3:1 → %v\n", seen)

	// Drain the local zone; the preferred subset is empty, so the fallback keeps
	// the call alive on the remote instance.
	cache.Update(sd.Event{Instances: []sd.Instance{
		{Address: "remote-C:80", Metadata: map[string]any{"zone": "z2", "weight": 9}},
	}})
	time.Sleep(10 * time.Millisecond)

	resp, err := invoke(lb, context.Background(), nil)
	if err != nil {
		fmt.Printf("  after draining z1: %v\n", err)
		return
	}
	fmt.Printf("  after draining z1, fallback → %v\n", resp)
}

// ── Demo 7: selector — picking an instance without building endpoints ─────────

// demo7_Selector is the assembly for callers that only need an address: a proxy
// that dials the instance itself, or an API that answers "where should I
// connect?". No factory runs, so nothing is connected for an instance nobody
// calls.
func demo7_Selector() {
	fmt.Println("\n=== 7. sd/selector: choose an instance, build no endpoint ===")

	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: []sd.Instance{
		{Address: "local-A:80", Metadata: map[string]any{"zone": "z1"}},
		{Address: "local-B:80", Metadata: map[string]any{"zone": "z1"}},
		{Address: "remote-C:80", Metadata: map[string]any{"zone": "z2"}},
	}})

	instances := selector.Subscribe(cache)
	defer instances.Close() //nolint:errcheck

	local := selector.Filter(instances, sd.MetadataEquals("zone", "z1"))
	pick := selector.New(local, selector.RoundRobin())

	for i := 0; i < 3; i++ {
		chosen, err := pick.Select(context.Background(), nil)
		if err != nil {
			fmt.Printf("  select: %v\n", err)
			return
		}
		fmt.Printf("  call %d → %s (zone %v)\n", i+1, chosen.Address, chosen.Metadata["zone"])
	}

	// A score table the caller owns: the highest score wins, and refusing to
	// score is a hard filter. This is where a load report pushed by the
	// instances belongs — stale by a reporting interval, and never in the
	// registry.
	load := map[string]float64{"local-A:80": 0.9, "local-B:80": 0.2, "remote-C:80": 0.5}
	byLoad := selector.New(instances, selector.Scored(func(item sd.Instance) (float64, bool) {
		score, known := load[item.Address]
		return -score, known // lower load, higher score
	}))
	chosen, err := byLoad.Select(context.Background(), nil)
	if err != nil {
		fmt.Printf("  select by load: %v\n", err)
		return
	}
	fmt.Printf("  least loaded → %s\n", chosen.Address)
}

// ── Demo 8: feedback — measuring the calls you make ───────────────────────────

// demo8_Feedback closes the loop: the calls report their own outcome, and the
// next pick is made from those measurements. Nothing is published to the
// registry, so no consumer reads a stale number.
func demo8_Feedback(logger *slog.Logger) {
	fmt.Println("\n=== 8. sd/feedback: eject on measured failures, prune on scale-down ===")

	failing := endpointer.Factory(func(item sd.Instance) (endpoint.Endpoint, io.Closer, error) {
		address := item.Address
		ep := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
			if address == "bad:80" {
				return nil, errors.New("upstream refused the connection")
			}
			return address, nil
		})
		return ep, io.NopCloser(nil), nil
	})

	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: sd.Addresses("bad:80", "good-A:80", "good-B:80")})
	time.Sleep(10 * time.Millisecond)

	set := endpointer.NewEndpointer(cache, failing, logger)
	defer set.Close() //nolint:errcheck

	// One table serves every policy: it ejects, it scores, and it counts what is
	// in flight. Follow keeps it the size of the service rather than the size of
	// the deployment history.
	table := feedback.NewTable(feedback.WithAlpha(1))
	following := table.Follow(cache)
	defer following.Close() //nolint:errcheck

	healthy := table.Healthy(feedback.HealthPolicy{MaxErrorRate: 0.5, MinSamples: 1})
	lb := balancer.New(set, table.Wrap(selector.Filtered(selector.RoundRobin(), healthy)))
	defer lb.Close() //nolint:errcheck

	seen := map[string]int{}
	for i := 0; i < 30; i++ {
		response, err := invoke(lb, context.Background(), nil)
		if err != nil {
			seen["failed"]++
			continue
		}
		seen[response.(string)]++
	}
	fmt.Printf("  30 calls → %v\n", seen)
	fmt.Printf("  bad:80 error rate %.0f%%, ejected after its first failure\n",
		table.Stats(sd.Instance{Address: "bad:80"}).ErrorRate*100)

	// Scale down. Following discovery drops the measurements for the address
	// that left, which is what stops a long-running table from growing with
	// every rolling deployment.
	cache.Update(sd.Event{Instances: sd.Addresses("good-A:80", "good-B:80")})
	time.Sleep(10 * time.Millisecond)
	fmt.Printf("  after bad:80 leaves discovery, samples retained for it: %d\n",
		table.Stats(sd.Instance{Address: "bad:80"}).Samples)
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	logger := slog.New(slog.DiscardHandler)

	demo1_RoundRobin(logger)
	demo2_Retry(logger)
	demo3_RetryWithCallback(logger)
	demo4_NewEndpoint(logger)
	demo5_InvalidateOnError(logger)
	demo6_Metadata(logger)
	demo7_Selector()
	demo8_Feedback(logger)

	fmt.Println("\nDone.")
}
