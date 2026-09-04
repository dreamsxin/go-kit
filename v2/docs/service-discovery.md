# Service Discovery And Routing

English | [简体中文](service-discovery_zh.md)

This chapter explains the outbound request path when a service has more than
one backend. The package guides answer what each package exports; this chapter
answers how to assemble discovery, endpoint construction, selection, feedback,
health checks, retry, and shutdown as one system.

The core in-process assemblies below have a runnable counterpart in
`examples/sd` (`go run ./examples/sd`). Provider and long-lived bridge sections
are integration templates; the application owns the provider and transport.

## Pick The Smallest Assembly

| Need | Assembly |
| --- | --- |
| one fixed address or test pool | `selector.Static` + `selector.New` |
| dynamic endpoints and connection reuse | `Instancer` -> `Endpointer` -> `Balancer` |
| retries with an explicit policy | add `sd/retry` or `sd/client` |
| measured least-request or passive ejection | `feedback.Table` + `feedback.Follow` |
| active liveness checks | `health.Check` before the endpointer/selector |
| caller-owned long connection | pick once, call `Done` when the connection ends |

The invariant is simple: `Pick` returns identity and `Done(Outcome)`. Close each
consumer before the source it uses.

## The request path

```text
Consul / etcd / another registry
    -> Instancer snapshot
    -> optional health.Check
    -> Endpointer + Factory
    -> selector filters and strategy
    -> Balancer.Pick
    -> endpoint call
    -> Picked.Done(Outcome)
    -> retry or the next selection
```

The registry publishes snapshots, not live load metrics. `sd.Instance` carries
an address and static metadata such as zone, protocol, version, weight, or
draining state. Dynamic measurements stay in the process that issues calls.

The ownership rules are deliberately separate:

| Layer | Owns | Does not own |
| --- | --- | --- |
| `sd.Instancer` | snapshots and provider watch lifecycle | endpoint connections |
| `sd/health` | active probes and probe state | registry lifecycle |
| `sd/endpointer` | endpoint factories and factory resources | selection policy |
| `sd/selector` | filters and selection strategies | endpoint construction |
| `sd/balancer` | identity-preserving endpoint selection | source ownership |
| `sd/feedback` | local measurements and ejection policy | registry writes |
| `sd/retry` | attempt policy and backoff | business retry safety |

## The core contracts

An instancer publishes an immutable-by-convention `sd.Event` and must implement
`Close`:

```go
type Instancer interface {
    Register(chan Event) Event
    Deregister(chan Event)
    Close() error
}
```

An endpoint balancer receives the request on every pick and returns the selected
identity together with a completion callback:

```go
type Balancer interface {
    Pick(ctx context.Context, request any) (Picked, error)
    Close() error
}

type Picked struct {
    Instance Instance
    Endpoint endpoint.Endpoint
    Done     Done
}

type Outcome struct {
    Err     error
    Latency time.Duration
    Bytes   int64
}
```

Call `Done` exactly once after the endpoint returns. Implementations supplied
by go-kit make the callback idempotent, so an adapter may safely use `defer`.
`Bytes` is an application-defined total. The generic retry layer cannot infer
it from opaque request and response values; a protocol adapter or bridge that
counts traffic should fill it.

The instance-only selector has the same feedback half of the contract.
`selector.New` binds a strategy to a source and returns a `Selector`; close it
when you are done, which releases the strategy and nothing else — the source and
the instancer behind it stay yours:

```go
type Selector interface {
    Select(ctx context.Context, request any) (sd.Instance, sd.Done, error)
    Close() error
}

pool := selector.Subscribe(instancer)
defer pool.Close()

pick := selector.New(pool, selector.RoundRobin())
defer pick.Close()

instance, done, err := pick.Select(ctx, request)
if err != nil {
    return err
}
started := time.Now()
err = dial(instance.Address)
done(sd.Outcome{Err: err, Latency: time.Since(started)})
```

This matters for table-backed strategies. Dropping `done` leaves the selected
instance permanently in flight and makes least-request and health decisions
wrong over time.

## A minimal assembly

For a fixed or already-created instancer, `sd/client` is the shortest path:

```go
factory := endpointer.Factory(func(instance sd.Instance) (endpoint.Endpoint, io.Closer, error) {
    return makeClientEndpoint(instance.Address), nil, nil
})

call, resources, err := client.NewEndpoint(instancer, factory, logger,
    client.WithMaxAttempts(3),
    client.WithTimeout(500*time.Millisecond),
)
if err != nil {
    return err
}
defer resources.Close()

response, err := call(ctx, request)
```

`NewEndpoint` defaults to round robin. `resources.Close` closes the balancer
first and the endpointer second. The instancer remains owned by the caller and
must be closed after its consumers:

```go
defer instancer.Close() //nolint:errcheck
defer resources.Close() // runs first
```

For full control, assemble the layers explicitly:

```go
set := endpointer.NewEndpointer(instancer, factory, logger)
defer set.Close()

strategy := selector.RoundRobin()
lb := balancer.New(set, strategy)
defer lb.Close()

call := retry.Retry(3, 500*time.Millisecond, lb)
```

`retry.Retry` takes both an attempt cap and a wall-clock budget, and the budget
wins: a call that runs out of time stops mid-schedule. The failure is a
`retry.Error` carrying every attempt made so far, with the context error as its
`Final`, so `errors.Is(err, context.DeadlineExceeded)` still matches while the
message names the instances that failed. Only a budget that expires before any
attempt completes returns the bare context error.

## Filtering and lifecycle state

Static metadata filters belong at the source or endpoint-set boundary:

```go
local := endpointer.Filter(set, sd.MetadataEquals("zone", "cn-north-1a"))
preferred := endpointer.Prefer(set, sd.MetadataEquals("zone", "cn-north-1a"))
```

`Filter` fails when the subset is empty. `Prefer` falls back to the complete
set. Both are re-evaluated when the endpoint set is read.

Dynamic policies use `selector.Filtered`, which receives the candidate set on
every pick:

```go
strategy := selector.Filtered(
    selector.RoundRobin(),
    sd.Keep(sd.Serving()),
)
lb := balancer.New(set, strategy)
```

`sd.StateKey` is `"state"`; `sd.StateDraining` means an instance remains
registered but must receive no new work. Existing connections are not closed by
service discovery. Draining is a registration property, not a feedback sample.

## Choosing a strategy

Three shapes of question, three contracts. Mixing them up is where routing code
usually goes wrong.

Single-instance selection — `selector.Strategy`, returning one index plus the
`Done` callback for that call:

| Need | Strategy | Notes |
| --- | --- | --- |
| even distribution | `selector.RoundRobin` | one atomic counter |
| independent random distribution | `selector.Random` | avoids client lockstep |
| static capacity weights | `selector.WeightedRandom` | zero weight drains an instance |
| measured or reported score | `selector.Scored` | highest score wins |
| local in-flight fairness | `table.LeastRequest` | power of two choices; the bare `selector.LeastRequest` takes any `LoadFunc` |
| request affinity | `selector.ConsistentHash` | request is always available |

Candidate ranking — `selector.Ranker`, returning an ordered `[]sd.Instance`:

| Need | Component | Notes |
| --- | --- | --- |
| shortlist for a caller that dials itself | `selector.NewRanker` | best first, ties broken by address |

Both `Scored` and `NewRanker` use the same request-aware score contract:

```go
type ScoreFunc func(
    ctx context.Context,
    request any,
    instance sd.Instance,
) (score float64, ok bool)
```

Return `ok == false` to exclude an instance. A score can ignore `ctx` and
`request` when it is purely instance-based, or use them for tenant, region, or
operation-specific routing.
`ScoreFunc` runs once per candidate on the selection hot path, so keep it
bounded and local: do not perform network I/O or wait while holding an
application lock. `NaN` is treated as unavailable; positive and negative
infinity remain valid scores.

Decorators, which wrap one of the above rather than answering a question
themselves:

| Need | Component | Notes |
| --- | --- | --- |
| exclude candidates per selection | `selector.Filtered` | takes `sd.InstanceFilter`s |
| ramp up a cold instance | `selector.SlowStart` | decorates a `WeightFunc` |

`Ranker` is deliberately not a `Strategy`. A strategy names the instance for one
call and owns that call's lifecycle through `Done`; a ranker returns candidates
and owns nothing, because there is no single call to attribute. Forcing it into
`Pick(...) (index, done, error)` would leave `Done` undefined — the whole list,
one entry of it, or every subsequent dial? Round robin and consistent hashing
also cannot rank: they define which instance is next, not a total order.

The endpoint constructors are thin wrappers around the same selector strategies:
`balancer.NewRoundRobin`, `NewRandom`, `NewWeightedRandom`, `NewScored`, and
`NewConsistentHash`. Use `balancer.New(set, strategy)` when composing filters,
feedback, or a custom strategy.

## Custom middleware and components

The built-ins are composition points, not closed implementations. A caller may
wrap or replace each layer through its narrow contract:

| Layer | Extension point | Typical middleware |
| --- | --- | --- |
| selector | `selector.Strategy` | logging, quotas, custom affinity |
| ranking | `selector.Ranker` | cache, shortlist limits, request policy |
| candidate set | `sd.InstanceFilter` | labels, tenancy, ejection |
| endpoint selection | `sd.Balancer` | tracing, metrics, admission control |
| endpoint creation | `endpointer.Factory` | transport setup, TLS, connection pooling |
| active health | `health.Probe` | authentication, probe metrics, custom protocol |
| retry | `retry.Callback` / `retry.Classifier` | idempotency and protocol policy |

A strategy middleware must forward the selected index, the inner callback, and
`Close` — `selector.CloseStrategy(inner)` is the one-line form, and skipping it
hides the inner strategy's cleanup, because only the outermost strategy is
visible to `selector.New` and `balancer.New`. A balancer middleware must forward
`Picked.Instance` and `Picked.Endpoint`, wrap `Picked.Done` at most once, and
delegate `Close` to the inner balancer. A factory or probe wrapper should
preserve the original error and closer semantics. These rules let custom
components interoperate with the same feedback table and retry executor as the
built-ins.

For request-aware scoring, implement the unified `selector.ScoreFunc` rather
than a second strategy interface:

```go
score := selector.ScoreFunc(func(ctx context.Context, request any, instance sd.Instance) (float64, bool) {
    tenant, _ := request.(Request)
    if tenant.Region != "" && instance.Metadata["region"] != tenant.Region {
        return 0, false
    }
    return localLoadScore(ctx, instance), true
})
strategy := selector.Filtered(selector.Scored(score), sd.Keep(sd.Serving()))
lb := balancer.New(set, strategy)
```

Instance-only scoring simply ignores `ctx` and `request`; custom code does not
need to implement every built-in policy to use the surrounding decorators.
Request/response middleware around the endpoint itself uses `endpoint.Middleware`
and `endpoint.Builder`; the contracts in this section wrap discovery components
and must preserve their callbacks and `Close` behavior.

## Feedback and passive ejection

`feedback.Table` is process-local. It records EWMA latency, error rate, bytes,
in-flight calls, and the first time an address entered the table. It never
writes a sample to the registry.

Use one table for the policies that should agree about the same traffic:

```go
table := feedback.NewTable()
ejector := feedback.NewEjector(table, feedback.EjectionPolicy{
    MaxErrorRate: 0.5,
    MinSamples:   5,
})

following := feedback.Follow(instancer, table, ejector)
defer following.Close()

strategy := table.Wrap(selector.Filtered(
    selector.Scored(table.Score()),
    ejector.Filter(),
))
lb := balancer.New(set, strategy)
```

`table.Wrap` is what puts calls into the table: it counts in-flight at `Pick` and
records the outcome at `Done`. Without it nothing is ever recorded, every
instance scores the same, and `selector.Scored` degrades to a random pick. An
instance with no samples scores the maximum, which is how it earns its first
calls; `selector.SlowStart` over `table.FirstSeen()` is the companion when that
first burst is too much for a cold process.

`feedback.Follow` retains state against the full discovery snapshot. Do not
retain against an already filtered candidate set: doing so would erase the
measurements that caused an instance to be ejected.

That includes a `health.Check` wrapping the same instancer — pass `instancer`,
not `checked`. A checker withdraws an instance it considers unhealthy, and a
retainer cannot tell a withdrawal from a deregistration: the ejector would drop
its ejection record and the table its measurements, so the instance comes back
with a clean slate as soon as probing recovers, and active and passive health
checking cancel each other out. `health.Check` still decorates the instancer
feeding the endpointer or selector; only `Follow` needs the undecorated one.

An ejector removes unhealthy candidates according to error rate, latency, and
in-flight thresholds. Ejection expires after `BaseDuration`, backs off for
repeat offenders up to `MaxDuration`, and resets the measurements that caused
the ejection. `MaxEjectionPercent` defaults to 50%; when the whole pool looks
bad, panic mode keeps the unchecked candidate set available.

Least request is the same composition without an endpoint-specific constructor:

```go
strategy := table.LeastRequest(selector.WithChoices(2))
lb := balancer.New(set, strategy)
```

For an external load report, use `selector.Scored` directly. For a caller that
needs a shortlist rather than one pick, use `selector.NewRanker`. Both receive
the request through the unified `ScoreFunc`:

```go
pool := selector.Subscribe(instancer)   // or selector.Static(...)
defer pool.Close()

ranker := selector.NewRanker(pool, table.Score(), ejector.Filter())
top, err := ranker.Rank(ctx, request, 3)
```

## Slow start

New instances are cold: they may have empty caches, unwarmed JIT code, or an
empty connection pool. `selector.SlowStart` decorates a weight function and
ramps it up over a window:

```go
weight := selector.SlowStart(
    selector.MetadataWeight("weight", 1),
    table.FirstSeen(),
    30*time.Second,
)
strategy := table.Wrap(selector.WeightedRandom(weight))
```

The ramp floors at one so a warming instance is not starved, and a weight of
zero is left alone because zero means "never pick me". `table.FirstSeen()`
reports when an instance entered the table, so drive the table from discovery —
`feedback.Follow` — and the ramp begins when the instance joins the service.
Without that, the table only learns of an instance on its first call, and an
instance nobody has called yet is unknown, which slow start treats as brand new.

## Active health checks

Passive feedback cannot see an instance that receives no traffic. `health.Check`
decorates an instancer and probes each address with bounded concurrency:

```go
checked := health.Check(instancer,
    health.TCPProbe(2*time.Second),
    health.WithInterval(10*time.Second),
    health.WithUnhealthyThreshold(3),
)
defer checked.Close()

set := endpointer.NewEndpointer(checked, factory, logger)
defer set.Close()
```

`health.HTTPProbe` treats responses below 400 as healthy. Thresholds are
consecutive results. By default an instance is healthy until its first probe
completes; `WithInitiallyHealthy(false)` keeps each unprobed instance out of the
published set while already-passed instances can continue serving. If every
instance has produced a result and none passes, the checker publishes the
unchecked set instead of turning a broken probe into an outage;
`WithFailOpen(false)` publishes the empty set instead, for callers where
reaching a dead instance is worse than reaching none. A custom Probe must return
when its context is cancelled because `Checker.Close` waits for active probes.

Which way to fail is a decision to make deliberately, not a default to accept by
omission:

- Read paths, caches, tolerant proxies: fail open. The probe is more likely
  broken than every backend at once, and publishing nothing turns a monitoring
  fault into an outage.
- Writes, non-idempotent operations, backends mid-migration: fail closed.
  Callers get `sd.ErrNoEndpoints` and can retry or shed load, which is
  recoverable; a write applied twice is not.

The same trade-off appears one layer down as
`feedback.EjectionPolicy.MaxEjectionPercent`, and the two answers should agree: a
service that fails closed here has no reason to let passive ejection empty the
pool either.

## Providers

Consul and etcd satisfy the `sd.Instancer` contract — `contract.go` in each
asserts it at compile time — and do not enter the dependency closure of generic
discovery packages.

```go
consulInstancer := consul.NewInstancer(consulClient, logger, "users", true)
defer consulInstancer.Close() //nolint:errcheck

etcdInstancer := etcd.NewInstancer(etcdClient, logger, "users")
defer etcdInstancer.Close() //nolint:errcheck
```

Consul uses health-passing service entries and can carry static labels through
service metadata. The etcd provider stores one leased registration per instance,
renews it, and resumes prefix watching from the last revision after a watch
disconnect. See the provider guides for registration options and operational
requirements.

## Long-lived connections

An L4 balancer chooses once when a connection is opened. Requests carried by a
long-lived tunnel remain pinned to that backend. For bridge or gateway traffic,
the useful outcome is therefore connection duration, dial error, and bytes
relayed, not only unary request latency:

```go
picked, err := lb.Pick(ctx, dialRequest)
if err != nil {
    return err
}
started := time.Now()
conn, err := dial(picked.Instance.Address)
if err != nil {
    picked.Done(sd.Outcome{Err: err, Latency: time.Since(started)})
    return err
}

bytes, err := proxy(conn)
picked.Done(sd.Outcome{
    Err:     err,
    Latency: time.Since(started),
    Bytes:   bytes,
})
```

The endpoint layer does not assume connection semantics; the bridge adapter is
responsible for deciding when the connection outcome is complete.

## Shutdown and troubleshooting

Close consumers before their sources:

```text
retry call stops
 -> balancer.Close
 -> endpointer.Close
 -> selector / feedback followers close
 -> health.Check.Close
 -> Instancer.Close
```

Common symptoms:

| Symptom | Check |
| --- | --- |
| no endpoint | discovery snapshot, endpoint factory errors, filters, zero weights |
| one instance receives all traffic | missing `Done`, stale in-flight table, or a score that excludes peers |
| all instances disappear | active probe configuration, ejection panic threshold, discovery error handling |
| retries repeat the same backend | consistent hash is intentionally sticky; use a different strategy if failover is required |
| shutdown hangs | a custom Probe or endpoint closer is ignoring context or taking too long |
| table grows over deployments | call `feedback.Follow` with the full discovery instancer |

Retry classification remains an application decision. `sd/retry` records
`retry.Attempt{Address, Err, Latency}` for completed failures, but it cannot
decide whether a business operation is safe to repeat.
