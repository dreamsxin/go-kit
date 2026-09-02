# sd - Service Discovery
English | [简体中文](README_zh.md)

The root `sd` package owns the protocol-neutral contracts — `Instance`, `Event`,
`Instancer`, `Registrar`, `Balancer`, `Match`, and `ErrNoEndpoints` — plus the
label readers every layer shares. Concrete components live in focused
subpackages and can be used independently: `sd/instance`, `sd/endpointer`,
`sd/selector`, `sd/balancer`, `sd/retry`, `sd/feedback`, `sd/health`,
`sd/client`.

This guide is the API reference for those packages. For how they compose into
one outbound path — ownership per layer, the `Pick`/`Done`/`Outcome` lifecycle,
provider choice, long-lived connections, shutdown order, and a troubleshooting
table — read
[Service discovery and routing](../docs/service-discovery.md).


## Quick start (no Consul needed)

```go
import (
    "github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/client"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
    "github.com/dreamsxin/go-kit/v2/sd/instance"
)

factory := endpointer.Factory(func(inst sd.Instance) (endpoint.Endpoint, io.Closer, error) {
	return makeClientEndpoint(inst.Address), nil, nil
})

// In-memory instancer — perfect for tests and local dev
cache := instance.NewCache()
cache.Update(sd.Event{Instances: sd.Addresses("host1:8080", "host2:8080")})

ep, closer, err := client.NewEndpoint(cache, factory, logger,
    client.WithMaxAttempts(3),
    client.WithTimeout(500*time.Millisecond),
)
if err != nil {
    return err
}
defer closer.Close()
resp, err := ep(ctx, request)
```

## With Consul

```go
import (
	"github.com/dreamsxin/go-kit/v2/sd/client"
	"github.com/dreamsxin/go-kit/v2/integrations/consul"
)

instancer := consul.NewInstancer(consulClient, logger, "my-service", true)

ep, closer, err := client.NewEndpoint(instancer, factory, logger,
    client.WithMaxAttempts(3),
    client.WithTimeout(500*time.Millisecond),
    client.WithInvalidateOnError(5*time.Second),
)
if err != nil {
    instancer.Close()
    return err
}
defer instancer.Close() //nolint:errcheck
defer closer.Close() // runs first: deregister and close endpoint connections
```

## Options

| Option | Default | Description |
|--------|---------|-------------|
| `WithMaxAttempts(n)` | 1 | Total attempts; must be at least 1 |
| `WithTimeout(d)` | 500ms | Positive total budget including all retries |
| `WithInvalidateOnError(d)` | disabled | Clear cache after SD error grace period |
| `WithBalancer(f)` | round robin | Replace the selection strategy |

Invalid options and nil required dependencies return an error before any
background goroutine starts.

## Instance metadata

An instance is an address plus static labels:

```go
type Instance = struct {
	Address  string
	Metadata map[string]any
}
```

Metadata carries the things a registry is designed to hold — zone, version,
protocol, capability, weight, tenant. It is *not* a channel for live load
signals: a registry write per metric sample would hammer the catalog, and every
consumer would read a stale number anyway. Balancers that need live signals
measure them in process; see the feedback table below.

Registries hand labels over as strings, while predicates are usually written
against typed literals. The `sd.Metadata*` readers coerce, so `5` and `"5"` both
work:

```go
weight, ok := sd.MetadataInt(inst.Metadata, "weight")
zone, ok := sd.MetadataString(inst.Metadata, "zone")
tls, ok := sd.MetadataBool(inst.Metadata, "tls")
```

Labels also reach the factory, because "how to connect" is a transport decision
the registration can influence:

```go
factory := endpointer.Factory(func(inst sd.Instance) (endpoint.Endpoint, io.Closer, error) {
	if secure, _ := sd.MetadataBool(inst.Metadata, "tls"); secure {
		return newTLSClient(inst.Address)
	}
	return newPlainClient(inst.Address)
})
```

With Consul, labels round-trip through the service `Meta` field:
`consul.MetaRegistrarOptions` writes them, and the instancer lifts them back
into `Instance.Metadata`. Tags stay out of metadata — they are a set, not
key/value pairs, and `NewInstancer` already filters on them server-side.

## Filtering the instance set

Selection and filtering are separate layers, so zone-aware routing is
composition rather than a zone-aware variant of every strategy. `endpointer`
decorates an instance set, and any balancer can sit on top of the result:

```go
set := endpointer.NewEndpointer(instancer, factory, logger)
defer set.Close()

// Envoy NO_FALLBACK: only this zone, or no endpoints at all.
local := endpointer.Filter(set, sd.MetadataEquals("zone", "cn-north-1a"))

// Envoy ANY_ENDPOINT: prefer this zone, fall back to the whole set when it is
// empty — the usual choice for zone affinity that must not cause an outage.
preferred := endpointer.Prefer(set, sd.MetadataEquals("zone", "cn-north-1a"))

lb := balancer.NewRoundRobin(preferred)
```

Predicates live in the root `sd` package because both layers read them:
`sd.MetadataEquals`, `sd.MetadataIn`, `sd.MetadataMatches`, `sd.HasMetadata`,
and `sd.And` / `sd.Or` / `sd.Not`. The instance layer uses the same predicates
through `selector.Filter` and `selector.Prefer`.

```go
sd.And(
	sd.MetadataEquals("version", "v2"),
	sd.MetadataIn("zone", "a", "b"),
	sd.Not(sd.HasMetadata("draining")),
)
```

`Filter` and `Prefer` re-evaluate on every selection, so a relabelled
instance moves in or out of the filtered set without reconnecting. Closing a
filtered set closes the source it wraps.

## Selection strategies

`sd/selector` owns the strategies; `sd/balancer` applies them to an endpoint
set. Six ship today, named after their Envoy/gRPC equivalents.

| Strategy | Balancer | Strategy (instance layer) | Picks by |
|----------|----------|---------------------------|----------|
| Round robin | `balancer.NewRoundRobin` | `selector.RoundRobin` | atomic counter |
| Random | `balancer.NewRandom` | `selector.Random` | uniform draw |
| Weighted random | `balancer.NewWeightedRandom` | `selector.WeightedRandom` | weight per instance |
| Scored | `balancer.NewScored` | `selector.Scored` | caller-supplied score, highest wins |
| Least request | `balancer.New` + `table.LeastRequest()` | `selector.LeastRequest` | in-flight count, power-of-two-choices |
| Consistent hash | `balancer.NewConsistentHash` | `selector.ConsistentHash` | hash of a request key |

```go
// Uniform random
client.WithBalancer(func(set endpointer.InstanceEndpointer) sd.Balancer {
	return balancer.NewRandom(set)
})

// Weighted from the registry: capacity is a registration label, and weight 0
// drains an instance without waiting for discovery to withdraw it.
client.WithBalancer(func(set endpointer.InstanceEndpointer) sd.Balancer {
	return balancer.NewWeightedRandom(set, balancer.MetadataWeight(balancer.DefaultWeightKey, 1))
})

// Least request: two random candidates, lower in-flight count wins. The table
// is the caller's, because the same measurements also drive scoring, ejection,
// and slow start — see "Writing a strategy" below.
client.WithBalancer(func(set endpointer.InstanceEndpointer) sd.Balancer {
	return balancer.New(set, table.LeastRequest(selector.WithChoices(2)))
})

// Scored: follow a load signal this process did not measure itself.
client.WithBalancer(func(set endpointer.InstanceEndpointer) sd.Balancer {
	return balancer.NewScored(set, func(instance sd.Instance) (float64, bool) {
		report, ok := reports.Latest(instance.Address) // your table, your protocol
		if !ok || report.Saturated() {
			return 0, false // false is a hard filter
		}
		return report.Score(), true
	})
})

// Consistent hash: every request for one tenant lands on the same instance
// for as long as that instance stays in the set.
client.WithBalancer(func(set endpointer.InstanceEndpointer) sd.Balancer {
	return balancer.NewConsistentHash(set, func(_ context.Context, request any) string {
		return request.(*GetProfileRequest).TenantID
	}, balancer.WithReplicas(200))
})
```

A `WeightFunc` is `func(sd.Instance) int`, so any weight source works —
`MetadataWeight` reads a registration label, a closure can read a local table.

Weighted, scored, least-request, and hash strategies need to know which
instance produced an endpoint, so they take `endpointer.InstanceEndpointer`
rather than the narrower `endpointer.Endpointer`. `endpointer.NewEndpointer`
already returns it.

Four behaviours are worth knowing before you pick one:

- Weighted random reports `sd.ErrNoEndpoints` when endpoints exist but every
  weight is zero or below. That is a selectable-instance shortage, and the
  default retry classifier treats it as temporary. `Scored` reports the same
  when every instance is excluded.
- Least request reads in-flight depth from a `feedback.Table` and records into
  the same one, so picking and accounting cannot drift apart. Nothing is
  published to the registry, and one table can also feed scoring and passive
  health. It is available at both layers: `selector.LeastRequest` takes any
  `LoadFunc`.
- `Scored` is the seam for signals measured elsewhere — a report the instances
  push, ORCA/LRS style out-of-band reporting, your own table. Such a signal is
  stale by at least one reporting interval; that is inherent to the channel, not
  a defect. Prefer least request when this process is on the data path.
- Every balancer uses `Pick(ctx, request)` and returns `sd.Picked`, which keeps
  the selected `Instance`, its `Endpoint`, and a `Done(sd.Outcome)` callback.
  Consistent hash reads the request from this common contract; there is no
  second request-aware interface.

Retry re-selects on every attempt, so a failed instance is not retried unless
the balancer hands it back. With consistent hashing that is intentional: the
same key resolves to the same instance until the set changes.

## Selecting without endpoints

Some callers only need an address: a proxy that dials the instance itself, or
an API that answers "where should I connect?". They assemble `sd/selector` and
never touch the endpoint factory, so no connection is created for an instance
nobody calls.

```go
import "github.com/dreamsxin/go-kit/v2/sd/selector"

instances := selector.Subscribe(instancer)   // or selector.Static(...)
defer instances.Close()

pool := selector.Prefer(instances, sd.MetadataEquals("zone", "cn-north-1a"))
pick := selector.New(pool, selector.WeightedRandom(selector.MetadataWeight("", 1)))

instance, done, err := pick.Select(ctx, request)   // sd.Instance: address plus labels
if err != nil { return err }
started := time.Now()
err = dial(instance.Address)
done(sd.Outcome{Err: err, Latency: time.Since(started)})
```

`Select` returns the same completion callback the balancer puts in
`sd.Picked.Done`, for the same reason: a strategy that keeps state per call has
to learn how that call ended. It is never nil and is safe to call twice, so
`defer done(outcome)` is fine. Dropping it is not a missing statistic but a
leak — a feedback table would count the selection as in flight forever, which
makes the instance look permanently saturated and keeps its entry alive.

The request is always passed to `selector.Select` and the strategy, so keyed
and unkeyed strategies share one path.


`Subscribe` keeps the latest snapshot and, like the endpointer, serves the last
good one through a discovery outage; add `selector.InvalidateOnError(d)` to drop
it after a grace period instead.

## Writing a strategy

Implement `selector.Strategy` and both layers accept it: `selector.New` for
instance selection, `balancer.New` for endpoint selection.

```go
type Strategy interface {
	Pick(ctx context.Context, request any, instances []sd.Instance) (index int, done sd.Done, err error)
}
```

The snapshot is passed in, so a strategy holds only its own state — a counter,
a ring, a score table — and can never pick against a snapshot its caller never
saw. Implementations must be safe for concurrent use: one strategy is shared by
every caller, and `sd/retry` selects on a fresh goroutine per attempt. Return
`sd.ErrNoEndpoints` when nothing is selectable. Return a `done` callback when
the strategy keeps local feedback state; it receives the call's `sd.Outcome`.

```go
lb := balancer.New(set, myStrategy{})       // usable by sd/client and sd/retry
pick := selector.New(instances, myStrategy{}) // usable without endpoints
```

To collect feedback from the calls themselves, use `sd/feedback.Table`. One
process-local table serves every policy: it scores, it counts what is in flight,
and it records when an address was first seen:

```go
table := feedback.NewTable()
ejector := feedback.NewEjector(table, feedback.EjectionPolicy{
	MaxErrorRate: 0.5,
	MinSamples:   5,
})

// Keep the table the size of the service, not of the deployment history:
// addresses that leave discovery take their measurements with them.
following := feedback.Follow(instancer, table, ejector)
defer following.Close()

strategy := table.Wrap(selector.Filtered(selector.Scored(table.Score()), ejector.Filter()))

lb := balancer.New(set, strategy)
defer lb.Close()

picked, err := lb.Pick(ctx, request)
if err != nil { return err }
started := time.Now()
response, err := picked.Endpoint(ctx, request)
picked.Done(sd.Outcome{Err: err, Latency: time.Since(started)})
_ = response
```

`feedback.Follow` takes one subscription to the instancer and feeds every
`feedback.Retainer` — the table and any ejector — so a departed address is
forgotten in one place. A discovery error is not treated as "everything is
gone": the last good set stays.

`sd.Match` is a per-instance predicate for static labels; passive ejection is an
`sd.InstanceFilter`, which receives the whole candidate set, because whether one
instance may be ejected depends on how many others are also failing. Use
`sd.Keep(match)` to put a label predicate in the same filter list.
`selector.Filtered` runs filters per selection and maps the choice back onto the
caller's snapshot; `endpointer.Filter` remains the right tool for a static view
of the endpoint set.

### Ejection and recovery

`feedback.Ejector` is the passive outlier detector. It compares each instance
against `EjectionPolicy` (`MaxErrorRate`, `MaxLatency`, `MaxInFlight`, gated by
`MinSamples`) and removes the offenders from the candidate set.

`MaxEjectionPercent` defaults to 50: when more than half of the candidates look
unhealthy, nothing is ejected, since a pool failing as a whole usually means a
shared dependency or a threshold set too tight. Envoy calls this panic mode.

An ejection expires after `BaseDuration`, doubled for each previous ejection of
that address and capped at `MaxDuration` — the same escalation Envoy applies,
with no half-open probation. When the window expires the ejector calls
`Table.Reset` for that address: un-ejecting without discarding the measurement
that caused the ejection would re-eject on the next pick, because a decaying
average never recovers without traffic. The ejection count deliberately survives
a clean pass — an address that flaps is ejected for longer each time — and is
cleared only when the address leaves discovery.

### Ranking instead of picking

`selector.Ranker` answers "which N instances should I use?" instead of "which
one". It is the shape a routing service needs, where the caller — a client, a
gateway, an agent — connects on its own:

```go
rank := selector.NewRanker(instances, table.Score(), ejector.Filter())
top, err := rank.Rank(ctx, 3)   // best first, deterministic on ties
```

`n <= 0` returns everything scorable, ordered. Ties break on address so two
processes ranking the same snapshot agree.

### Slow start

A cold instance loses least-request and scored selection outright: it has no
in-flight requests and no latency history, so it wins every comparison and is
handed the full share of traffic before its caches, pools, and JIT are warm.
`selector.SlowStart` ramps its weight up over a window instead:

```go
weight := selector.SlowStart(selector.MetadataWeight("weight", 1), table.FirstSeen(), 30*time.Second)
strategy := table.Wrap(selector.WeightedRandom(weight))
```

The instance reaches its configured weight when the window elapses; before that
it holds at least 1, so it is never starved. `table.FirstSeen` supplies the
timestamps, which is why the ramp survives a strategy being rebuilt.

### Draining

`sd.StateKey` (`"state"`) with `sd.StateReady` / `sd.StateDraining` is the label
convention for an instance that is still registered but should stop receiving
new work. `sd.Serving()` and `sd.Draining()` are the matching predicates:

```go
strategy := selector.Filtered(selector.RoundRobin(), sd.Keep(sd.Serving()))
```

Draining is a property of the registration, so it belongs in metadata, not in
the feedback table: a shutting-down instance is healthy, it is just leaving.

## Active health checks

Passive detection only sees instances that receive traffic — a cold instance or
an unreachable one is never measured. `sd/health` probes them directly and
decorates an instancer, so every layer downstream is unchanged:

```go
import "github.com/dreamsxin/go-kit/v2/sd/health"

checked := health.Check(instancer, health.TCPProbe(2*time.Second),
	health.WithInterval(10*time.Second),
	health.WithUnhealthyThreshold(3))
defer checked.Close()

ep := endpointer.NewEndpointer(checked, factory, logger)
```

`health.HTTPProbe(scheme, path, timeout)` treats any status ≥ 400 as a failure.
Thresholds are consecutive results, so one lost packet does not remove an
instance. A `Probe` must return when its context is cancelled; `Close` cancels
and waits for the round in flight.

Two deliberate choices: an instance is treated as healthy until its first probe
completes (`WithInitiallyHealthy(false)` to invert), and once every instance has
been probed and none passed, the checker republishes the unchecked set rather
than an empty one — a probe that is itself broken must not black-hole the
service. The fail-open applies only after every instance has produced a result,
so unprobed instances stay hidden while already-passed instances remain usable.
`Close` stops the probes and deregisters from the source without closing it.



## Architecture

```
Instancer → [health.Check] → [selector.Filter] → Selector             → instance
Instancer → [health.Check] → Endpointer → [Filter] → Balancer → Retry → endpoint
                                                        ↑ Outcome ↓
                                              feedback.Table + Ejector
```

Two assemblies, one set of strategies; callers that never issue the call
themselves stop at the first line. What each layer owns, and what it must not,
is in [Service discovery and routing](../docs/service-discovery.md#the-request-path).

```go
// Manual assembly (full control)
ep   := endpointer.NewEndpointer(instancer, factory, logger)
defer ep.Close()
lb   := balancer.NewRoundRobin(ep)
call := retry.Retry(3, 500*time.Millisecond, lb)
```


For low-level assembly, cache invalidation is configured with
`endpointer.InvalidateOnError`. The higher-level `client.NewEndpoint`
constructor exposes the equivalent `client.WithInvalidateOnError` option.

## Retry strategies

```go
// Fixed max attempts
retry.Retry(3, time.Second, lb)

// Production calls should provide an explicit retry classifier.
retry.WithClassifier(time.Second, lb,
    func(n int, err error) (keepTrying bool, replacement error) {
        return n < 5, nil
    },
	func(err error) bool {
		var retryable interface{ Retryable() bool }
		return errors.As(err, &retryable) && retryable.Retryable()
	},
)
```

The default classifier retries explicit `Retryable() == true` errors and
temporary no-endpoint conditions. Unknown and protocol errors are permanent.
For gRPC, pass `integrations/grpc.Retryable` explicitly through
`client.WithRetryable`; domain write safety remains an application decision.

`Instancer.Close` stops provider watches. `Endpointer.Close` waits for its update loop and closes all resources returned
by the endpoint factory. Treat the closer as part of the constructor contract,
not as an optional cleanup hook.

## Consul registration

```go
registrar := consul.NewRegistrar(client, logger, "my-service", "10.0.0.1", 8080,
    consul.IDRegistrarOptions("my-service-1"),
    consul.MetaRegistrarOptions(map[string]string{
        "zone":    "cn-north-1a",
        "version": "v2",
        "weight":  "5",
    }),
    consul.CheckRegistrarOptions(&stdconsul.AgentServiceCheck{
        HTTP:     "http://10.0.0.1:8080/health",
        Interval: "10s",
    }),
)
if err := registrar.Register(); err != nil {
    return err
}
defer func() { _ = registrar.Deregister() }()
```

`MetaRegistrarOptions` merges, so several calls can contribute labels from
different config sources. Keep live load out of it: report static labels here,
measure load in the balancer.

`Instancer.Close` cancels and joins the active Consul blocking query, so call it
after endpoint-owned resources have been closed.

## See also

- [Service discovery and routing](../docs/service-discovery.md) — how these
  packages compose, and the production and troubleshooting guidance
- `examples/sd/` — runnable demo of every sd component
- `examples/profilesvc/client/` — Consul-backed client example

