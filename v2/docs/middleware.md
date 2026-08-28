# Middleware

English | [简体中文](middleware_zh.md)

Middleware is the primary extension point of the endpoint layer; the HTTP
boundary has its own `http.Handler` middleware. This page covers composition
order, the built-in catalog, the four flow-control patterns, and where each
cross-cutting concern belongs across the layers.

## Composition

`endpoint.Chain` and `endpoint.Builder` compose middleware in declaration
order; the first middleware is the outermost:

```go
ep := endpoint.NewBuilder(createUser).
    WithValidation().
    WithTimeout(5 * time.Second).
    WithMetrics(&metrics).
    Use(endpoint.RateLimitMiddleware(limiter)).
    Use(breaker.Middleware()).
    Build()
```

Service-wide middleware for every route goes through `kit.WithEndpointMiddleware`;
a single route with its own chain uses `kit.HandleJSONTypedWithMiddleware`, or
is built with `endpoint.NewBuilder` and registered through
`kit.HandleJSONEndpoint`.

HTTP middleware is a separate boundary: `kit.WithHTTPMiddleware` and
`security/http.Chain` compose standard `http.Handler` middleware and wrap every
route including health checks.

## Cross-cutting concerns by layer

Logging, tracing, metrics, and error handling have one canonical home per
layer:

| Concern | Service layer | Endpoint layer | Transport layer |
| --- | --- | --- | --- |
| Logging | stays clean; reads correlation IDs from the context | `slogadapter.LoggingMiddleware`, `integrations/zap`, or `slogadapter.NewTelemetry` | `server.AccessLogMiddleware` for protocol facts; `ServerErrorHandler` records failures |
| Tracing | the context carries trace and request IDs | `TracingMiddleware` (W3C correlation), `oteladapter.TracingMiddleware` (spans) | `transporthttp.ExtractTraceparent` / `InjectTraceparent` header propagation |
| Metrics | — | `MetricsMiddleware`, `oteladapter.NewMetrics` | access-log status/bytes; `ServerFinalizer` hooks |
| Errors | returns `apperror` classifications | `ErrorHandlingMiddleware` attaches the operation name; `ValidationMiddleware` short-circuits | `ErrorEncoder` maps kinds to statuses; `ErrorHandler` observes |

Placement rules:

- The service layer stays pure: cross-cutting behavior is applied by the
  endpoint middleware that wraps it. Service code reads correlation IDs from
  the context and returns `apperror`-classified failures.
- The endpoint layer is the canonical home for business-level logging,
  tracing, metrics, and error decoration — it sees decoded requests, business
  responses, and business errors.
- The transport layer owns protocol facts: access logs, status mapping, and
  header propagation. It never re-interprets business results.

## Flow control

A middleware receives the wrapped endpoint and decides per request how the rest
of the chain runs. Four patterns cover the practical cases:

| Pattern | What it does | Built-in example |
| --- | --- | --- |
| Short-circuit | Answer without calling `next` | validation, bulkhead, backpressure |
| Branch | Send the request to a different endpoint | `Fallback` |
| Repeat | Call `next` again with backoff | `sd/retry` |
| Replace | Wrap `next` with different behavior | `TimeoutMiddleware`, `MetricsMiddleware` |

The chain is fixed at construction; middleware cannot rewire routes at runtime.
That keeps request paths deterministic and testable.

## The built-in catalog

| Middleware | Behavior | Rejection |
| --- | --- | --- |
| `ValidationMiddleware` | validates `Validatable` requests | 400 `bad_request.validation` |
| `TimeoutMiddleware` | bounded endpoint duration | 500 deadline exceeded |
| `MetricsMiddleware` | request count and duration | never rejects |
| `ErrorHandlingMiddleware` | wraps endpoint errors with the operation name | never rejects |
| `TracingMiddleware` | W3C trace context propagation | never rejects |
| `BackpressureMiddleware` | global in-flight cap | 429 |
| `CircuitBreaker` | consecutive failures trip the breaker, probe closes it | 429 |
| `RateLimitMiddleware` | reject over-limit requests | 429 |
| `DelayRateLimitMiddleware` | wait for a token instead of rejecting | context error |
| `Fallback` | answer with a fallback endpoint on failure | never rejects |
| `BulkheadMiddleware` | per-key concurrency isolation | 429 |

`Fallback` and `BulkheadMiddleware` are Builder shortcuts
(`WithFallback`, `WithBulkhead`); the rest are composed with `Use`.

## Debugging the chain

`Builder.Describe` returns the middleware chain in application order, so a
startup log can print exactly what runs:

```go
b := endpoint.NewBuilder(createUser).
    WithValidation().
    UseNamed("auth", authMiddleware).
    WithTimeout(5 * time.Second)
log.Printf("chain: %s -> endpoint", strings.Join(b.Describe(), " -> "))
// chain: validation -> auth -> timeout -> endpoint
```

The built-in `With*` shortcuts record labels automatically; custom middleware
composed with `Use` appear as `?` unless labeled with `UseNamed`.

## Nesting

A wrapped endpoint can itself be a chain. A fallback that carries its own
timeout and metrics:

```go
degraded := endpoint.NewBuilder(cachedResponse).
    WithTimeout(time.Second).
    WithMetrics(&degradedMetrics).
    Build()
ep := endpoint.NewBuilder(primary).WithFallback(degraded).Build()
```

See the runnable tour in [examples/middleware](../examples/README.md).
