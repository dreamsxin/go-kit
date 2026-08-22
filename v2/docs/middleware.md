# Middleware

English | [简体中文](middleware_zh.md)

Middleware is the only extension point of the endpoint layer. This page covers
composition order, the built-in catalog, and the four flow-control patterns.

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
a single route with its own chain is built with `endpoint.NewBuilder` and
registered through `kit.HandleJSONEndpoint`.

HTTP middleware is a separate boundary: `kit.WithHTTPMiddleware` and
`security/http.Chain` compose standard `http.Handler` middleware and wrap every
route including health checks.

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
