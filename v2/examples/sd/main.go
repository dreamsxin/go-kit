// Package main demonstrates the service-discovery (sd) components
// without any external dependency (no Consul, no network):
//
//   - sd/instance.Cache        — in-memory Instancer for testing
//   - sd/endpointer            — wires Instancer → EndpointCache
//   - sd/endpointer.Prefer     — metadata filtering with fallback
//   - sd/selector              — pick an instance without building endpoints
//   - sd/selector.Ranker       — a shortlist instead of one instance
//   - sd/selector.SlowStart    — ramp a newly seen instance up
//   - sd/balancer              — lock-free RoundRobin, weighted random
//   - sd/retry                 — Retry, WithCallback
//   - sd/feedback              — measured outcomes, outlier ejection, pruning
//   - sd/health                — active probing as an Instancer decorator
//   - sd/client.NewEndpoint    — one-liner that wires everything together
//   - endpointer.InvalidateOnError — cache invalidation on SD errors
//   - draining instances       — registered, but kept out of new work
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
	"sort"
	"sync"
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
		chosen, done, err := pick.Select(context.Background(), nil)
		if err != nil {
			fmt.Printf("  select: %v\n", err)
			return
		}
		// Every selection reports its result, even here where the strategy
		// keeps no state: it is the same callback a feedback table needs to
		// release the call it just counted.
		done(sd.Outcome{})
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
	chosen, done, err := byLoad.Select(context.Background(), nil)
	if err != nil {
		fmt.Printf("  select by load: %v\n", err)
		return
	}
	done(sd.Outcome{})
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
	// the deployment history, and drives the ejector's state with the same
	// subscription.
	table := feedback.NewTable(feedback.WithAlpha(1))
	ejector := feedback.NewEjector(table, feedback.EjectionPolicy{
		MaxErrorRate: 0.5,
		MinSamples:   1,
		BaseDuration: 50 * time.Millisecond,
	})
	following := feedback.Follow(cache, table, ejector)
	defer following.Close() //nolint:errcheck

	lb := balancer.New(set, table.Wrap(selector.Filtered(selector.RoundRobin(), ejector.Filter())))
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

	// Ejection expires. Without that, an instance receiving no traffic produces
	// no new measurements, so nothing would ever clear the ones that ejected it.
	time.Sleep(60 * time.Millisecond)
	if _, err := invoke(lb, context.Background(), nil); err != nil {
		fmt.Println("  after the ejection window: bad:80 was tried again and failed again")
	}

	// Scale down. Following discovery drops the measurements for the address
	// that left, which is what stops a long-running table from growing with
	// every rolling deployment.
	cache.Update(sd.Event{Instances: sd.Addresses("good-A:80", "good-B:80")})
	time.Sleep(10 * time.Millisecond)
	fmt.Printf("  after bad:80 leaves discovery, samples retained for it: %d\n",
		table.Stats(sd.Instance{Address: "bad:80"}).Samples)
}

// ── main ──────────────────────────────────────────────────────────────────────

// addresses reports what a source currently offers, sorted so the output does
// not depend on publication order.
func addresses(source selector.Source) []string {
	items, err := source.Instances()
	if err != nil {
		return []string{"error: " + err.Error()}
	}
	list := ordered(items)
	sort.Strings(list)
	return list
}

// ordered reports addresses in the order given, which is the point when the
// order is the answer.
func ordered(items []sd.Instance) []string {
	list := make([]string, 0, len(items))
	for _, item := range items {
		list = append(list, item.Address)
	}
	return list
}

// ── Demo 9: sd/health — probing what no request has touched ───────────────────

// demo9_ActiveHealth covers the gap passive feedback cannot: an instance that
// receives no traffic is never measured, so nothing can eject it. Probing asks
// directly, and because it decorates the instancer, no layer downstream changes.
func demo9_ActiveHealth(logger *slog.Logger) {
	fmt.Println("\n=== 9. sd/health: probe what no request has touched ===")

	// Probes run on their own goroutines, so the switch a test or an operator
	// flips has to be guarded.
	var mu sync.Mutex
	down := map[string]bool{"unreachable:80": true}
	fail := func(addresses ...string) {
		mu.Lock()
		defer mu.Unlock()
		for _, address := range addresses {
			down[address] = true
		}
	}
	probe := health.Probe(func(_ context.Context, target sd.Instance) error {
		mu.Lock()
		refused := down[target.Address]
		mu.Unlock()
		if refused {
			return errors.New("connection refused")
		}
		return nil
	})

	cache := instance.NewCache()
	cache.Update(sd.Event{Instances: sd.Addresses("serving-A:80", "serving-B:80", "unreachable:80")})

	checked := health.Check(cache, probe,
		health.WithInterval(10*time.Millisecond),
		health.WithUnhealthyThreshold(1),
		health.WithLogger(logger))
	defer checked.Close() //nolint:errcheck

	// The checker republishes through its own cache, so everything downstream
	// still sees a plain sd.Instancer.
	instances := selector.Subscribe(checked)
	defer instances.Close() //nolint:errcheck

	time.Sleep(50 * time.Millisecond)
	fmt.Printf("  after probing: %v\n", addresses(instances))

	// A probe that fails for every instance is far more likely to be broken
	// than to mean the whole service is down, so the unchecked set is published
	// rather than an empty one: publishing nothing turns a monitoring fault
	// into an outage.
	fail("serving-A:80", "serving-B:80")
	time.Sleep(50 * time.Millisecond)
	fmt.Printf("  every probe failing: %v — fail open, not black hole\n", addresses(instances))
}

// ── Demo 10: selector.Ranker — answering "which N", not "which one" ───────────

// demo10_Ranker is the shape a routing service needs: the caller asks where to
// connect and dials the instance itself, so it wants candidates to fail over
// through rather than one address it has to come back for.
func demo10_Ranker() {
	fmt.Println("\n=== 10. selector.Ranker: a shortlist for a caller that dials itself ===")

	pool := selector.Static(sd.Addresses("edge-A:443", "edge-B:443", "edge-C:443", "edge-D:443")...)
	score := map[string]float64{"edge-A:443": 0.2, "edge-B:443": 0.9, "edge-C:443": 0.5}
	rank := selector.NewRanker(pool, func(item sd.Instance) (float64, bool) {
		value, known := score[item.Address]
		return value, known // edge-D refuses a score, so it is not a candidate
	})

	// Nil request: this shortlist does not depend on who is asking. The
	// parameter is there for the cases where it does — a tenant pinned to a
	// region, a job class with its own pool.
	best, err := rank.Rank(context.Background(), nil, 2)
	if err != nil {
		fmt.Printf("  rank: %v\n", err)
		return
	}
	fmt.Printf("  best 2 of 4 → %v\n", ordered(best))

	every, err := rank.Rank(context.Background(), nil, 0)
	if err != nil {
		fmt.Printf("  rank: %v\n", err)
		return
	}
	fmt.Printf("  n <= 0 → %v, edge-D:443 never appears\n", ordered(every))
}

// ── Demo 11: selector.SlowStart — a cold instance wins every comparison ───────

// demo11_SlowStart shows why weights need a ramp. A new instance has no
// in-flight requests and no latency history, so least request and scored
// selection both rate it best and hand it full traffic while its caches,
// pools and JIT are still cold.
func demo11_SlowStart() {
	fmt.Println("\n=== 11. selector.SlowStart: ramp a cold instance up ===")

	const window = time.Minute
	seen := map[string]time.Time{
		"warm:80":    time.Now().Add(-window),
		"warming:80": time.Now().Add(-window / 4),
		"cold:80":    time.Now(),
	}
	// In a real assembly this is table.FirstSeen(), so the ramp survives the
	// strategy being rebuilt.
	first := selector.FirstSeenFunc(func(item sd.Instance) (time.Time, bool) {
		at, known := seen[item.Address]
		return at, known
	})
	weight := selector.SlowStart(selector.MetadataWeight("weight", 10), first, window)

	for _, address := range []string{"warm:80", "warming:80", "cold:80", "unknown:80"} {
		fmt.Printf("  %-12s weight %2d of 10\n", address, weight(sd.Instance{Address: address}))
	}

	// Zero is left alone: it means "never pick me", and ramping it up would
	// contradict the operator who set it.
	drained := sd.Instance{Address: "drained:80", Metadata: map[string]any{"weight": 0}}
	fmt.Printf("  %-12s weight %2d — zero is not ramped\n", drained.Address, weight(drained))
}

// ── Demo 12: draining — still registered, no longer taking new work ───────────

// demo12_Draining uses the state label. Draining is a property of the
// registration rather than something measured, so it belongs in metadata and is
// read by a filter — a shutting-down instance is healthy, it is just leaving.
func demo12_Draining() {
	fmt.Println("\n=== 12. draining: registered, but not for new work ===")

	serving := func(state string) []sd.Instance {
		return []sd.Instance{
			{Address: "ready-A:80", Metadata: map[string]any{sd.StateKey: sd.StateReady}},
			{Address: "ready-B:80", Metadata: map[string]any{sd.StateKey: sd.StateReady}},
			{Address: "leaving:80", Metadata: map[string]any{sd.StateKey: state}},
		}
	}

	count := func(pool []sd.Instance) map[string]int {
		pick := selector.New(selector.Static(pool...),
			selector.Filtered(selector.RoundRobin(), sd.Keep(sd.Serving())))
		seen := map[string]int{}
		for i := 0; i < 6; i++ {
			chosen, done, err := pick.Select(context.Background(), nil)
			if err != nil {
				seen["no endpoint"]++
				continue
			}
			done(sd.Outcome{})
			seen[chosen.Address]++
		}
		return seen
	}

	fmt.Printf("  draining → %v\n", count(serving(sd.StateDraining)))
	// Calls already in flight to a draining instance still finish; only new
	// work is withheld. Flipping the label back is all it takes to return.
	fmt.Printf("  ready    → %v\n", count(serving(sd.StateReady)))
}

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
	demo9_ActiveHealth(logger)
	demo10_Ranker()
	demo11_SlowStart()
	demo12_Draining()

	fmt.Println("\nDone.")
}
