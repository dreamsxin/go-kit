# Upgrade Notes

English | [简体中文](MIGRATION_zh.md)

go-kit v2 follows semantic versioning. This page records the upgrade actions
needed for the current release; the complete per-release change list lives in
[CHANGELOG.md](CHANGELOG.md).

## Migrating From Legacy go-kit (v0/v1 Style)

Codebases built on the old go-kit style (`github.com/go-kit/kit`: per-route
`transport/http.NewServer` with decode/encode function pairs, `endpoint.Set`
structs, `NewGRPCServer`/`NewGRPCClient` assembly blocks) map to v2
constructs like this:

| Legacy construct | go-kit v2 equivalent |
| --- | --- |
| `httptransport.NewServer(ep, decode, encode, opts...)` per route | `kit.HandleJSONTyped` / `kit.HandleJSONWithMiddleware`; `server.NewServer` for custom codecs |
| Per-route decode/encode function pairs | Typed handlers need no codecs; `server.RawBodyCodec` covers custom wire formats |
| `endpoint.Set` struct wiring | Route registration on the `kit.HTTP` component; microgen-generated `endpoints.go` |
| `NewGRPCServer` / grpc handler struct | `integrations/grpc` server + `transport.Binding` (one endpoint serves HTTP and gRPC) |
| `NewGRPCClient` endpoint blocks | `integrations/grpc/client`; `sd/client` composes discovery, balancing, and retry |
| `jwt.HTTPToContext()` / `jwt.GRPCToContext()` | `security` subject contract + application-owned credential extraction at the protocol boundary |
| `opentracing.HTTPToContext` / `TraceClient` | `endpoint.TracingMiddleware` (W3C) or `observability/otel` |
| `ratelimit.NewErroringLimiter` | Built-in `endpoint.RateLimitMiddleware` |
| `switch err.Error() {...}` status mapping | `apperror` classification; transports map kinds; `JSONErrorEncoderWithKindMapper` for custom statuses |
| `Err string` response fields (error-as-string over the wire) | Return errors; `endpoint.Failer` when a proto message must carry the failure |
| Hand-written proto↔domain mappers | Generated transports (`microgen` from `.proto`) |

Pitfalls the legacy style is prone to, fixed by design in v2:

- **Silent middleware drift**: per-method middleware applied by copy-paste or
  comment toggles leaves routes without rate limiting or tracing without any
  signal. In v2 middleware is declared once at component level or explicitly
  per route, and chains are introspectable via `endpoint.Builder.Describe()`.
- **Error classification loss**: errors serialized as strings lose their kind
  across the wire. `apperror` kinds survive across HTTP and gRPC.
- **Mapper drift**: hand-written proto↔domain converters drift from the
  schema over time; generated codecs stay in sync with the contract.

Recommended migration order: move one service at a time; start with typed
JSON handlers (`kit`), then replace error-string conventions with `apperror`,
then collapse duplicate HTTP/gRPC assemblies into `transport.Binding`, and
finally let `microgen` own regenerated transports.

## Upgrading To Unreleased (sd instance metadata)

Service discovery now carries labels, not just addresses. Three source changes,
all mechanical.

1. `sd.Event.Instances` is `[]sd.Instance`. When there are no labels to report,
   `sd.Addresses` builds the snapshot:

   ```go
   // before
   cache.Update(sd.Event{Instances: []string{"host1:8080", "host2:8080"}})

   // after
   cache.Update(sd.Event{Instances: sd.Addresses("host1:8080", "host2:8080")})

   // with labels
   cache.Update(sd.Event{Instances: []sd.Instance{
       {Address: "host1:8080", Metadata: map[string]any{"zone": "a", "weight": 5}},
   }})
   ```

2. `endpointer.Factory` receives the whole instance, so it can honour the labels
   that decide how to connect:

   ```go
   // before
   func(instance string) (endpoint.Endpoint, io.Closer, error) {
       return newClient(instance), nil, nil
   }

   // after
   func(instance sd.Instance) (endpoint.Endpoint, io.Closer, error) {
       return newClient(instance.Address), nil, nil
   }
   ```

3. `balancer.NewWeightedRandom` takes a `balancer.WeightFunc`:

   ```go
   // before
   balancer.NewWeightedRandom(set, func(instance string) int { return weights[instance] })

   // after
   balancer.NewWeightedRandom(set, func(instance sd.Instance) int { return weights[instance.Address] })

   // or read the weight the registration reported
   balancer.NewWeightedRandom(set, balancer.MetadataWeight(balancer.DefaultWeightKey, 1))
   ```

Custom `sd.Instancer` implementations publish `[]sd.Instance`; the field layout
is otherwise unchanged, and the aliases stay anonymous structs so provider
modules can keep mirroring them structurally without importing core.

If you already used subset filtering, the predicates moved to the root `sd`
package so the new instance layer can share them, and the two decorators were
renamed to match `selector.Filter` / `selector.Prefer`:

```go
// before
endpointer.Subset(set, endpointer.MetadataEquals("zone", "a"))
endpointer.PreferSubset(set, endpointer.MetadataEquals("zone", "a"))

// after
endpointer.Filter(set, sd.MetadataEquals("zone", "a"))
endpointer.Prefer(set, sd.MetadataEquals("zone", "a"))
```

`Match`, `MetadataIn`, `MetadataMatches`, `HasMetadata`, `And`, `Or`, and `Not`
moved the same way. Behaviour is unchanged: `Filter` fails when nothing matches,
`Prefer` falls back to the full set.

The selection contract is now deliberately unified. `sd.Balancer` exposes
`Pick(ctx, request) (sd.Picked, error)` and `Close`; `Endpoint`, `EndpointFor`,
and `RequestBalancer` are removed. `sd.Picked` carries the selected instance and
endpoint plus a `Done(sd.Outcome)` callback that must run after every call.
`selector.Strategy` likewise uses one `Pick(ctx, request, instances)` method;
there is no `RequestStrategy`/`PickFor` pair. `retry.Error.Attempts` records the
address and latency for each failed attempt. `sd.Instancer` now also requires
`Close() error`.

`sd/feedback.Table` is the in-process result table for EWMA latency, error rate,
bytes, in-flight counts, and first-seen times. Use `Table.Wrap`, `Table.Score`,
`Table.LeastRequest`, or `Table.FirstSeen` to connect local observations to
strategy selection; it does not write live signals back to the registry. Passive
ejection is a separate component, `feedback.Ejector`.

Three things to get right when adopting it:

```go
// before
lb := balancer.NewLeastRequest(set, balancer.WithTable(table))
strategy := table.Wrap(selector.Scored(table.Score()))
healthy := table.Healthy(policy)          // was an sd.Match
following := table.Follow(instancer)

// after
lb := balancer.New(set, table.LeastRequest())             // the table is always the caller's
ejector := feedback.NewEjector(table, feedback.EjectionPolicy{
	MaxErrorRate: 0.5,
	MinSamples:   5,
})
following := feedback.Follow(instancer, table, ejector)   // drop replaced addresses
defer following.Close()
strategy := table.Wrap(selector.Filtered(selector.Scored(table.Score()),
	ejector.Filter()))                                     // now an sd.InstanceFilter
```

`selector.ScoreFunc` now receives `(context.Context, request, sd.Instance)` and
is shared by `selector.Scored`, `selector.NewRanker`, and `feedback.Table.Score`.
Instance-only scores can ignore the first two arguments; request-specific scores
can use them directly without another request-aware interface.

`balancer.NewLeastRequest` is gone, along with the `balancer.LoadFunc`,
`LeastRequestOption`, `DefaultChoices`, and `WithChoices` aliases; the selector
originals are the ones to use. The endpoint layer no longer imports `sd/feedback`
at all, so a round-robin assembly does not compile the feedback layer in. Least
request is now composed the same way every other feedback-driven policy is:
`balancer.New(set, table.LeastRequest(...))`. That also removes the old
`table == nil` shorthand, which built a table the caller had no handle to and
therefore could never `Retain` — an unbounded map in any process that survives a
rolling deployment.


`Ejector.Filter` is an `sd.InstanceFilter` because its ejection cap is a decision
about the whole candidate set. Call `feedback.Follow` — or `Table.Retain(snapshot)`
and `Ejector.Retain(snapshot)` yourself — or the table keeps one entry per address
it has ever seen, which grows with every rolling deployment.

Ejection is no longer permanent. An ejection expires after
`EjectionPolicy.BaseDuration` (doubled per previous ejection of that address, up
to `MaxDuration`) and expiry resets the address's measurements. If you replicate
this policy yourself, reset the measurement too: an EWMA that receives no traffic
never recovers, so returning an instance without clearing what ejected it
re-ejects it on the next pick.

New optional layers, none of which change existing assemblies:

```go
// Active probing — a decorator on the instancer, so nothing downstream changes.
checked := health.Check(instancer, health.TCPProbe(2*time.Second))
defer checked.Close()

// Ranking instead of picking, for a routing service.
top, err := selector.NewRanker(instances, table.Score(), ejector.Filter()).Rank(ctx, request, 3)

// Slow start, so a cold instance does not win every comparison at once.
weight := selector.SlowStart(selector.MetadataWeight("weight", 1), table.FirstSeen(), 30*time.Second)
```

Draining now has a documented label: set `sd.StateKey` to `sd.StateDraining` on
the registration and filter with `sd.Keep(sd.Serving())`.

`selector.Select` now returns the completion callback, matching what the
balancer already put in `sd.Picked.Done`:

```go
// before
instance, err := pick.Select(ctx, request)

// after
instance, done, err := pick.Select(ctx, request)
if err != nil { return err }
started := time.Now()
err = dial(instance.Address)
done(sd.Outcome{Err: err, Latency: time.Since(started)})
```

The callback is never nil on success and is idempotent. Report the outcome even
when the strategy looks stateless — the instance layer used to drop it, which
made any table-backed strategy count every selection as in flight forever.




Behavior changes that need no source edit but do need attention:

- A metadata-only change is a change. Subscribers are notified when an instance
  is relabelled even though the address set is identical. The endpointer reuses
  the live endpoint in that case, so no reconnect happens.
- A snapshot containing the same address twice now yields one endpoint. The
  duplicate is dropped instead of replacing the first entry and leaking its
  closer.
- With Consul, service `Meta` is surfaced as `Instance.Metadata`. Tags are not:
  they remain a filtering input for `TagsInstancerOptions`.

## Upgrading To v2.7.0


Three source changes:

1. `endpoint.Metrics` counter fields are unexported. Read through `Snapshot()`,
   which keeps the same field names:

   ```go
   // before
   count := metrics.RequestCount

   // after
   count := metrics.Snapshot().RequestCount
   ```

2. `endpoint.TypeAssertError.Want` is a `reflect.Type`. Build the error with the
   generic constructor instead of a composite literal:

   ```go
   // before
   var zero Req
   return &endpoint.TypeAssertError{Got: request, Want: zero}

   // after
   return endpoint.NewTypeAssertError[Req](request)
   ```

3. `endpoint.RateLimiterFunc` is now `RateLimiterFuncs`. Only the type name
   changed; the fields are the same:

   ```go
   // before
   limiter := endpoint.RateLimiterFunc{AllowFn: bucket.Allow}

   // after
   limiter := endpoint.RateLimiterFuncs{AllowFn: bucket.Allow}
   ```

Behavior changes that need no source edit but do need attention:

- An endpoint timeout now encodes as HTTP 504 instead of 500, matching
  `apperror.KindDeadlineExceeded` and the gRPC adapter. A client disconnect
  (`context.Canceled`) now encodes as 499 instead of 500. Clients and alerting
  rules that treated either as a 500 must be updated.
- `ErrCircuitOpen`, `ErrBulkheadFull`, and `ErrBackpressure` now encode as HTTP
  503 instead of 429, because they classify themselves as
  `apperror.KindUnavailable`. `ErrRateLimited` stays 429. Over gRPC all four
  previously surfaced as `Internal` and now carry their real codes. Update
  client retry policy and any dashboard that counted a tripped breaker as 429.
- `kit.WithMetrics` labels each observation with the route pattern and runs
  outermost in the chain. `metrics.Snapshot()` still reports the total, but it
  now also counts requests rejected by a rate limit or a breaker; per-route
  numbers come from `metrics.SnapshotFor(pattern)`.
- `Fallback` stops running the fallback for an already-cancelled caller and
  joins both errors when the fallback fails, so callers that relied on the
  fallback masking every failure will now see the primary cause.

## Upgrading To v2.6.0

`v2.6.0` is the approved architecture-evolution release and contains
intentional breaking changes. Required source changes:

1. Assembly layer: `kit.Service`, `kit.New`, `kit.MustNew`, and `Service.Run`
   are removed. Split the wiring:

   ```go
   // before
   svc, err := kit.New(":8080", opts...)
   kit.HandleJSONTyped(svc, "/hello", handler)
   svc.Run(ctx)

   // after
   svc, err := kit.NewHTTP(":8080", opts...)
   kit.HandleJSONTyped(svc, "/hello", handler)
   host, err := kit.NewHost(kit.WithLifecycle(svc))
   host.Run(ctx)
   ```

   `Handle`, `HandleFunc`, and `HandleSSE` are now methods on `*kit.HTTP`.
2. Error model: `server.HTTPError`, `server.NewHTTPError`, and
   `server.WrapHTTPError` are removed. Classify failures with `apperror`
   (transports map kinds uniformly, including gRPC); protocol-specific
   customization uses `transporthttp.StatusCoder`, `ErrorCoder`,
   `PublicMessager`, and `Headerer`.
3. The deprecated `log` facade is removed; use `log/slog` directly.
4. SSE: `kit.SSEWriter` is removed. Use `server.SSEStream` (same method set)
   with `kit.HandleSSETyped` (endpoint middleware applies) or the
   `HTTP.HandleSSE` method for raw streams.
5. Generated projects (`microgen` `v0.3.0`): the user-owned
   `cmd/custom_routes.go` hook is standard-library only now —
   `func registerCustomRoutes(r *http.ServeMux)` registers routes directly;
   move listing entries into `customRouteDescriptions() []string`. Extend
   refuses pre-v3 manifests (`microgen.project.v2`) with this migration hint;
   rerun a full generation afterwards to refresh the manifest.

New capabilities worth adopting: `security` subject contracts
(`RequireAuthenticated`, `RequireRole`), `slogadapter.NewTelemetry`,
`kit.NamedLifecycle` / `kit.ReadinessProvider`, and `apperror` convenience
constructors.

## Upgrading To v2.4.1

`v2.4.1` is backward compatible: additive capabilities and behavioral fixes
only. No source changes are required.

Behavioral notes worth reviewing when jumping from `v2.2.0`:

- Trace IDs minted by `endpoint.TracingMiddleware` are 32 lowercase hex
  characters (W3C Trace Context format) instead of 16 (changed in `v2.3.0`).
  Callers that treat the ID as an opaque string are unaffected.
- `endpoint.ErrBackpressure` and `endpoint.ErrBulkheadFull` are encoded as
  HTTP 429 instead of 500 (changed in `v2.3.0`).

## Compatibility Policy

- Patch releases fix behavior; minor releases add capabilities. Both are
  backward compatible within `/v2`.
- Incompatible changes require a new major module version, except approved
  and recorded deviations: the `v2.1.0` direct refactor, the `v2.2.0`
  metrics return-type fix, the `v2.6.0` architecture evolution, and the
  `v2.7.0` error-contract unification.
- v2 does not carry deprecated forwarding APIs. Documentation for earlier
  releases remains available through the immutable release tags.
