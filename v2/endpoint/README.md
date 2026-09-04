# endpoint
English | [简体中文](README_zh.md)

The `endpoint` package is the core runtime abstraction of `go-kit`.

It is where business operations are wrapped with reusable runtime policy such as:

- timeout
- metrics
- tracing
- backpressure
- circuit breaking
- rate limiting

If `service` is the business layer and `transport` is the protocol layer, `endpoint` is the runtime governance layer between them.

## Core Abstractions

### `Endpoint`

The central type is:

```go
type Endpoint func(ctx context.Context, request any) (response any, err error)
```

This is the callable unit shared by:

- transports
- middleware
- service wrappers
- service discovery and client-side execution flows

### `Middleware`

The standard middleware shape is:

```go
type Middleware func(Endpoint) Endpoint
```

This keeps runtime policies composable and transport-agnostic.

### `Failer`

`Failer` lets a response type carry its own business error instead of returning
it. When a response implements `Failed() error` and returns non-nil, the HTTP
and gRPC servers discard the response and encode that error through their error
encoder — the same status, stable code, and error handler a returned error gets.

Reach for it only when the service signature cannot return an error (a batch
handler filling one slot per item, an adapter over a callback API). It is *not*
a way to answer 200 with an error field: for that, put the field in the response
struct and do not implement `Failer`.

## Recommended Entry Points

For most services, these are the main entry points:

- `Endpoint`
- `Middleware`
- `NewBuilder`
- `NewTypedBuilder`
- `Chain`
- `TimeoutMiddleware`
- `RecordingMiddleware` / `MetricsMiddleware`
- `ErrorHandlingMiddleware`
- `RecoveryMiddleware` / `WithRecovery`
- `TracingMiddleware`
- `BackpressureMiddleware`
- `NewCircuitBreaker`
- `RateLimitMiddleware` / `DelayRateLimitMiddleware`
- `RetryMiddleware`
- `Unwrap`

Related extension packages:

- `observability/slog`
- `integrations/zap`

## Builder API

The builder API is the recommended default for composing endpoint behavior.

Example:

```go
var metrics endpoint.Metrics

ep := endpoint.NewBuilder(base).
    WithRecording("POST /users", &metrics).
    WithErrorHandling("CreateUser").
    WithTimeout(5 * time.Second).
    WithCircuitBreaker(endpoint.NewCircuitBreaker()).
    WithRateLimit(limiter).
    Build()

snapshot := metrics.SnapshotFor("POST /users")
fmt.Println(snapshot.RequestCount, snapshot.ErrorCount, snapshot.AverageDuration())
```

Read the collector through `Snapshot` (total), `SnapshotFor` (one operation), or
`Operations` (the recorded labels). Each returns a detached value that is safe
to copy; the collector's own counters are unexported and guarded internally.

To export the same observations elsewhere, implement `endpoint.Recorder` and
pass it to `RecordingMiddleware` alongside — or instead of — the in-memory
collector. `Metrics` deliberately keeps only counters and a duration total;
histograms and time series belong to a metrics backend.

Why prefer the builder:

- clearer than hand-wrapping multiple middleware layers
- expresses runtime policy in one place
- stays aligned with the framework's preferred composition style

Builder contract note:

- `NewBuilder` requires a non-nil base endpoint
- `Use(...)` and `UseNamed(...)` require non-nil middleware values
- invalid composition input **panics** at assembly time rather than deferring
  the problem to request time — assembly runs at startup, so a bad chain fails
  the process instead of a request
- `UseNamed(label, m)` records a label that `Describe()` returns in application
  order (outermost first, unlabeled entries as `"?"`), which is what to log at
  startup to see the chain a route actually got

Metrics label note: `WithRecording(operation, ...)` labels every observation
with `operation`, so `Metrics.SnapshotFor(operation)` and `Operations()` work.
`WithMetrics(&m)` records with **no** operation label, so only the aggregate
`Snapshot()` is meaningful and `SnapshotFor` returns a zero value. Prefer
`WithRecording` unless the aggregate is genuinely all you want. Operation labels
must come from a bounded set — a label per user or per URL path grows the map
for the process's lifetime.

## `Chain`

`Chain` is the lower-level middleware composition helper.

Example:

```go
ep = endpoint.Chain(
    loggingMiddleware,
    metricsMiddleware,
    authMiddleware,
)(base)
```

Middleware order remains important:

- the first middleware passed to `Chain` is the outermost one

## Middleware Flow Control

The chain is fixed at construction, but a middleware owns everything that
happens next: it receives the wrapped endpoint and decides per request how the
rest of the chain runs. Four patterns cover the practical cases.

**Short-circuit** - stop the chain and answer immediately. The built-in
`ValidationMiddleware`, `BackpressureMiddleware`, `RateLimitMiddleware`, and the
circuit breaker all do this:

```go
func requireTenant(next endpoint.Endpoint) endpoint.Endpoint {
    return func(ctx context.Context, request any) (any, error) {
        if tenantFromContext(ctx) == "" {
            return nil, endpoint.NewValidationError("tenant", "is required")
        }
        return next(ctx, request)
    }
}
```

**Branch** - send the request somewhere else. `Fallback` is exactly this
pattern: the fallback endpoint can itself be a fully middleware-built chain,
so a branch can carry its own timeout and metrics:

```go
degraded := endpoint.NewBuilder(cachedResponse).
    WithTimeout(time.Second).
    Build()
ep := endpoint.NewBuilder(primary).WithFallback(degraded).Build()
```

**Repeat** - call the rest of the chain again. `RetryMiddleware` wraps `next`
and invokes it per attempt with its own backoff and classification. (`sd/retry`
does the same thing one layer out, but it is a *balancer-driven endpoint*, not a
`Middleware`: it picks a fresh instance per attempt.)

**Replace** - wrap `next` with different behavior instead of calling it
directly: `MetricsMiddleware` observes its result, `Fallback` answers from a
second endpoint when the first fails. The wrapped endpoint stays opaque; a
middleware never needs to know how deep the chain is.

`TimeoutMiddleware` is deadline propagation, not enforcement: it derives a
context with the deadline and calls `next` on the same goroutine. Everything
downstream that respects `ctx` — the HTTP and gRPC clients, database drivers,
`sd/retry` backoff — returns when it expires. An endpoint that ignores
cancellation runs to completion regardless, so treat the timeout as a bound on
what the caller *waits for downstream*, not as a way to kill work in progress.

One thing middleware cannot do is rewire the chain of already-constructed
routes at runtime - composition happens at assembly, which keeps request paths
deterministic and testable. Per-route variation belongs at assembly time
(`kit.WithEndpointMiddleware` for every route; build the endpoint with
`endpoint.NewBuilder` and register it through `kit.HandleJSONEndpoint` when a
single route needs its own chain).

## Typed Endpoints

`TypedEndpoint[Req, Resp]` provides compile-time request and response typing while preserving the same runtime model.

Example:

```go
var ep endpoint.TypedEndpoint[HelloReq, HelloResp] =
    func(ctx context.Context, req HelloReq) (HelloResp, error) {
        return HelloResp{Message: "Hello, " + req.Name}, nil
    }

typed := endpoint.Unwrap[HelloReq, HelloResp](
    endpoint.NewTypedBuilder(ep).
        WithTimeout(5 * time.Second).
        Use(endpoint.NewCircuitBreaker().Middleware()).
        Build(),
)
```

Use typed endpoints when:

- type safety matters at call sites
- you want to reduce runtime type assertions

You can adopt typed endpoints incrementally.

Type assertion note:

- `Wrap()` and `Unwrap()` return typed assertion errors when request or response values do not match the expected types, instead of panicking on mismatch.

## Built-In Middleware

Core middleware in `endpoint`:

- `RecordingMiddleware`
- `MetricsMiddleware`
- `ErrorHandlingMiddleware`
- `TimeoutMiddleware`
- `TracingMiddleware`
- `ValidationMiddleware`
- `BackpressureMiddleware`
- `InFlightMiddleware`
- `BulkheadMiddleware`
- `RateLimitMiddleware`
- `DelayRateLimitMiddleware`
- `RetryMiddleware`
- `Fallback`

`RecoveryMiddleware` is the process-boundary guard for endpoint panics. Put it
outermost so it covers all inner middleware; the default handler returns a
classified internal error that built-in transports redact as 500. Supply a
`PanicHandler` when you need stack/error reporting, but return a classified
error rather than exposing the recovered value:

```go
ep := endpoint.NewBuilder(base).
    WithRecovery(func(ctx context.Context, request, recovered any) error {
        logger.ErrorContext(ctx, "endpoint panic", "panic", recovered)
        return endpoint.DefaultPanicHandler(ctx, request, recovered)
    }).
    Build()
```

`CircuitBreaker` is not itself a `Middleware`: it is a stateful object shared by
every request, so you construct it once with `NewCircuitBreaker()` and install
`breaker.Middleware()` (or `Builder.WithCircuitBreaker(breaker)`). Keep the
`*CircuitBreaker` value — it is where `State()` lives.

`BackpressureMiddleware(max)` caps total in-flight calls; `InFlightMiddleware(max,
&counter)` does the same but also publishes the live count into the caller's
`*int64`, for exporting as a gauge.

`CircuitBreaker` is a dependency-free endpoint circuit breaker: consecutive
failures trip it open, it rejects with `ErrCircuitOpen` (HTTP 503, with the
remaining open window reported as `Retry-After`), and a probe after the open
window decides recovery. Rate limiting ships as `RateLimitMiddleware` (reject)
and `DelayRateLimitMiddleware` (wait) over the application-owned `RateLimiter`
contract; `ErrRateLimited` encodes as 429 because the caller exceeded its quota.

The breaker defaults to 5 consecutive failures to trip, 1 probe success to
close, and a 1 minute open window (`DefaultBreaker*`); any non-positive setting
falls back to the default. `CircuitBreaker.State()` exposes the current state,
which is the value to export as a gauge — a breaker open for more than a minute
is worth alerting on. `RateLimiterFuncs` adapts plain functions to the
`RateLimiter` contract, and a limiter that also implements `RetryAfterReporter`
has its delay attached to `ErrRateLimited`. Both limiters are process-local, so
a multi-replica deployment either scales the per-replica rate by the replica
count or supplies a shared, application-owned limiter.

`RetryMiddleware` repeats transient failures with exponential backoff and full
jitter. Its default classifier answers in a fixed order: context errors and
local rejections (`ErrCircuitOpen`, `ErrBulkheadFull`, `ErrBackpressure`,
`ErrRateLimited`) are never retried; then an error implementing
`interface{ Retryable() bool }` decides for itself — this **overrides** its
`apperror` kind, which is how a 408 or 429 from an upstream becomes retryable;
otherwise only `apperror.KindUnavailable` is retried, read from either
`apperror.Kinder` or `apperror.KindNamer`. Unclassified errors are not retried.
A retry hint on the error (`RetryAfterReporter`) overrides the schedule up to
`MaxRetryAfterHint`, and the caller owns idempotency. A transport client is an
`Endpoint` too, so the same stack applies to outbound calls:
`transport/http/client.HTTPStatusError` classifies itself by status code and
reports the server's `Retry-After`.

`TracingMiddleware` speaks the W3C Trace Context format. It joins an incoming
`TraceContext` (extracted from the `traceparent` header by
`transport/http.ExtractTraceparent`) under the same trace ID, mints a
W3C-conformant trace otherwise, and exposes the same ID through
`TraceIDFromContext`. Outbound HTTP calls forward the active trace with
`transport/http.InjectTraceparent`.

`ValidationMiddleware` calls `Validate() error` on requests that implement
`Validatable` before business logic runs. `NewValidationError` and
`ValidationError.Add` collect field-level failures; the HTTP transport maps
`ValidationError` to 400 with the stable code `bad_request.validation`, and
requests without a `Validate` method pass through unchanged:

```go
type CreateUserRequest struct{ Name string }

func (r CreateUserRequest) Validate() error {
    if r.Name == "" {
        return endpoint.NewValidationError("name", "is required")
    }
    return nil
}

ep := endpoint.NewBuilder(createUser).WithValidation().Build()
```

`Fallback` answers with a fallback endpoint when the primary fails, keeping
the error away from callers while a dependency recovers. It skips the fallback
when the caller's context is already done — a cancellation is not a dependency
failure — and joins both errors when the fallback fails too, so the primary
cause stays reachable through `errors.Is`.

`BulkheadMiddleware` limits concurrency per resource key (tenant, dependency),
so one slow key cannot consume the shared budget the way the global
`BackpressureMiddleware` count can. It is a **queue, not an immediate shed**:
requests whose key pool is full wait for a slot, so saturation first shows up as
latency. When the caller's context ends the wait, the error classifies as that
context error — a caller timeout is 504, a disconnect is 499 — and wraps
`ErrBulkheadFull`, so `errors.Is` still reports the saturation. Pair it with
`WithTimeout` so the queue is bounded by something. Bulkhead keys must stay
bounded, since each key owns a slot pool for the endpoint's lifetime:

```go
ep := endpoint.NewBuilder(callDependency).
    WithBulkhead(20, func(req any) string { return req.(Request).TenantID }).
    WithFallback(cachedResponse).
    Build()
```

Logging is provider-specific and lives outside the core package:

```go
ep = zapadapter.LoggingMiddleware(logger, "CreateUser")(ep)
// Or use observability/slog with a standard-library slog.Logger.
```

This keeps the core `endpoint` import graph limited to the Go standard
library and the zero-dependency `apperror` package.

Circuit breaking, rate limiting, and retrying are built into the core package
and hold no third-party dependencies:

- `NewCircuitBreaker` (`.Middleware()`) rejects calls with `ErrCircuitOpen`
  while open and probes to recover. Two trip conditions are available:
  `WithBreakerFailureThreshold` counts consecutive failures, and
  `WithBreakerMaxErrorRate` counts the failure share of a rolling window, which
  is what catches a dependency failing a third of the time — a run that short
  never trips a counter. `WithBreakerSlowCallThreshold` counts a slow answer as a
  failure, and `WithBreakerFailurePredicate` decides which errors count at all
  (caller cancellation does not, by default).
- `RateLimitMiddleware` rejects over-limit requests with `ErrRateLimited`;
  `DelayRateLimitMiddleware` waits for a token and honors context
  cancellation. Limiters implement the `RateLimiter` contract and stay
  application owned.
- `RetryMiddleware` repeats transient failures with backoff; the classifier and
  the schedule are replaceable.

Every rejection error classifies itself through `apperror`, so HTTP and gRPC
derive the same status from it without special-casing the sentinel values.

## What Belongs In `endpoint`

Good responsibilities for this layer:

- runtime timeout policy
- request accounting and metrics
- structured error wrapping
- request correlation and protocol-neutral instrumentation hooks
- resilience wrappers
- reusable invocation policy

## What Does Not Belong In `endpoint`

Avoid putting these concerns here:

- protocol-specific encode/decode logic
- HTTP or gRPC request mapping
- database access logic
- product-specific workflow orchestration
- one-off application behavior that cannot be generalized

If a concern is protocol-specific, it likely belongs in `transport`.
If it is pure domain behavior, it likely belongs in `service`.

## Extension Points

The primary supported extension surface is custom middleware.

Recommended extension patterns:

- compose custom `Middleware`
- use `Builder.Use(...)`
- wrap typed endpoints through `NewTypedBuilder`
- plug circuit breaker or rate limiter adapters into the middleware chain

Avoid:

- creating parallel middleware models that bypass `Endpoint`
- encoding transport-specific concerns into middleware unless unavoidable

## Stability Notes

The published `v2.0.0` endpoint API remains a historical stable baseline. The
approved `v2.1.0` SemVer exception resets this package contract to the API
reviewed in `tools/testdata/api_surface.sha256`. After `v2.1.0`, compatibility
again covers:

- `Endpoint`
- `Middleware`
- builder-style composition
- the framework's central middleware model

Circuit breaking and rate limiting used to live in separate
`integrations/circuitbreaker` and `integrations/ratelimit` modules. Both moved
into this package in v2.4.0 and those modules were removed, so there is no
extra module to select: `NewCircuitBreaker` and `RateLimitMiddleware` ship
here.

## Best Practices

1. Keep endpoint middleware reusable across services.
2. Prefer endpoint middleware over transport-specific policy code.
3. Keep business logic out of endpoint wrappers unless it is truly policy-adjacent.
4. Use typed endpoints where they improve safety and readability.
5. Treat endpoint composition as the default place for runtime governance.

## Related Docs

- [README.md](../README.md)
- [ARCHITECTURE.md](../ARCHITECTURE.md)
- [PRODUCTION.md](../PRODUCTION.md)
