# endpoint

The `endpoint` package is the core abstraction of the framework.

## Core types

| Type | Description |
|------|-------------|
| `Endpoint` | `func(ctx, request) (response, error)` — the single callable unit |
| `Middleware` | `func(Endpoint) Endpoint` — wraps an endpoint to add behaviour |
| `Factory` | `func(addr) (Endpoint, io.Closer, error)` — creates endpoints from addresses |
| `Failer` | Optional interface on response types to carry business errors |

## TypedEndpoint (compile-time type safety)

`TypedEndpoint[Req, Resp]` eliminates runtime type assertions.

```go
// Define a typed endpoint — no interface{} anywhere
var ep endpoint.TypedEndpoint[HelloReq, HelloResp] =
    func(ctx context.Context, req HelloReq) (HelloResp, error) {
        return HelloResp{Message: "Hello, " + req.Name}, nil
    }

// Apply middleware via NewTypedBuilder, then recover type safety with Unwrap
typed := endpoint.Unwrap[HelloReq, HelloResp](
    endpoint.NewTypedBuilder(ep).
        WithTimeout(5 * time.Second).
        Use(circuitbreaker.Gobreaker(cb)).
        Build(),
)

// Call site is fully type-safe — no .(HelloResp) assertion needed
resp, err := typed(ctx, HelloReq{Name: "world"})
fmt.Println(resp.Message)
```

**Migration path:** existing `Endpoint` code continues to work unchanged.
Adopt `TypedEndpoint` incrementally at the boundaries where type safety matters most.

## Builder (recommended)

```go
var metrics endpoint.Metrics

ep := endpoint.NewBuilder(base).
    WithMetrics(&metrics).
    WithErrorHandling("CreateUser").
    WithTimeout(5 * time.Second).
    Use(circuitbreaker.Gobreaker(cb)).
    Use(ratelimit.NewErroringLimiter(limiter)).
    Build()
```

## Chain (lower-level)

```go
ep = endpoint.Chain(
    loggingMiddleware,
    metricsMiddleware,
    authMiddleware,
)(base)
// call order: logging → metrics → auth → base
```

## Built-in middleware

| Middleware | Import | Description |
|-----------|--------|-------------|
| `MetricsMiddleware` | `endpoint` | Counts requests, successes, errors, duration |
| `ErrorHandlingMiddleware` | `endpoint` | Wraps errors with operation name |
| `TimeoutMiddleware` | `endpoint` | Cancels context after deadline |
| `LoggingMiddleware` | `endpoint` | Logs each call with duration |
| `Gobreaker` | `endpoint/circuitbreaker` | sony/gobreaker circuit breaker |
| `HandyBreaker` | `endpoint/circuitbreaker` | streadway/handy circuit breaker |
| `Hystrix` | `endpoint/circuitbreaker` | afex/hystrix-go circuit breaker |
| `NewErroringLimiter` | `endpoint/ratelimit` | Reject immediately when over limit |
| `NewDelayingLimiter` | `endpoint/ratelimit` | Wait for token (respects ctx deadline) |

## Failer

Implement `Failer` on a response type to carry business errors without using
the Go error return value.  Useful when the transport protocol requires a
successful wire-level response even on business failure (e.g. gRPC).

```go
type MyResponse struct {
    Result string
    Err    error
}

func (r MyResponse) Failed() error { return r.Err }
```

## Metrics

```go
var m endpoint.Metrics
ep = endpoint.MetricsMiddleware(&m)(ep)

fmt.Printf("requests=%d success=%d errors=%d avg_ms=%.1f\n",
    m.RequestCount, m.SuccessCount, m.ErrorCount,
    float64(m.TotalDuration.Milliseconds())/float64(m.RequestCount))
```

## See also

- `examples/middleware/` — runnable demo of every middleware
- `examples/quickstart/` — minimal HTTP service using Builder
