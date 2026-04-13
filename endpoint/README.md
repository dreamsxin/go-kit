# endpoint

The `endpoint` package is the core runtime abstraction of `go-kit`.

It is where business operations are wrapped with reusable runtime policy such as:

- timeout
- logging
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

`Failer` allows a response type to carry a business error without using the Go error return value.

Use it only when the transport requires a successful wire-level response even on business failure.

Most business logic should still prefer normal Go errors.

## Recommended Entry Points

For most services, these are the main entry points:

- `Endpoint`
- `Middleware`
- `NewBuilder`
- `NewTypedBuilder`
- `Chain`
- `TimeoutMiddleware`
- `MetricsMiddleware`
- `ErrorHandlingMiddleware`
- `LoggingMiddleware`
- `Unwrap`

Related extension packages:

- `endpoint/circuitbreaker`
- `endpoint/ratelimit`

## Builder API

The builder API is the recommended default for composing endpoint behavior.

Example:

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

Why prefer the builder:

- clearer than hand-wrapping multiple middleware layers
- expresses runtime policy in one place
- stays aligned with the framework's preferred composition style

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
        Use(circuitbreaker.Gobreaker(cb)).
        Build(),
)
```

Use typed endpoints when:

- type safety matters at call sites
- you want to reduce runtime type assertions

You can adopt typed endpoints incrementally.

## Built-In Middleware

Core middleware in `endpoint`:

- `MetricsMiddleware`
- `ErrorHandlingMiddleware`
- `TimeoutMiddleware`
- `LoggingMiddleware`

Specialized middleware packages:

- `endpoint/circuitbreaker`
  - Gobreaker
  - HandyBreaker
  - Hystrix integration
- `endpoint/ratelimit`
  - `NewErroringLimiter`
  - `NewDelayingLimiter`

## What Belongs In `endpoint`

Good responsibilities for this layer:

- runtime timeout policy
- request accounting and metrics
- structured error wrapping
- tracing and observability
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

`endpoint` core is part of the stable framework surface.

That includes:

- `Endpoint`
- `Middleware`
- builder-style composition
- the framework's central middleware model

More specialized subpackages such as `endpoint/circuitbreaker` and `endpoint/ratelimit` are public and supported, but still somewhat more evolvable.

## Best Practices

1. Keep endpoint middleware reusable across services.
2. Prefer endpoint middleware over transport-specific policy code.
3. Keep business logic out of endpoint wrappers unless it is truly policy-adjacent.
4. Use typed endpoints where they improve safety and readability.
5. Treat endpoint composition as the default place for runtime governance.

## Related Docs

- [README.md](../README.md)
- [FRAMEWORK_BOUNDARIES.md](../FRAMEWORK_BOUNDARIES.md)
- [STABILITY.md](../STABILITY.md)
- [PACKAGE_SURFACES.md](../PACKAGE_SURFACES.md)
- [ANTI_PATTERNS.md](../ANTI_PATTERNS.md)
