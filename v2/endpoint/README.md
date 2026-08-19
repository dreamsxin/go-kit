# endpoint

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
- `TracingMiddleware`
- `BackpressureMiddleware`
- `Unwrap`

Related extension packages:

- `integrations/circuitbreaker`
- `integrations/ratelimit`
- `observability/slog`
- `integrations/zap`

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

snapshot := metrics.Snapshot()
fmt.Println(snapshot.RequestCount, snapshot.ErrorCount, snapshot.AverageDuration())
```

Use `Snapshot` for every concurrent read. It returns `MetricsSnapshot`, a
lock-free value that is safe to copy. The collector's exported fields remain
available for source compatibility, but reading them while middleware is
updating the collector is a data race.

Why prefer the builder:

- clearer than hand-wrapping multiple middleware layers
- expresses runtime policy in one place
- stays aligned with the framework's preferred composition style

Builder contract note:

- `NewBuilder` requires a non-nil base endpoint
- `Use(...)` requires non-nil middleware values
- invalid composition input fails fast instead of deferring the problem to request time

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

Type assertion note:

- `Wrap()` and `Unwrap()` return typed assertion errors when request or response values do not match the expected types, instead of panicking on mismatch.

## Built-In Middleware

Core middleware in `endpoint`:

- `MetricsMiddleware`
- `ErrorHandlingMiddleware`
- `TimeoutMiddleware`
- `TracingMiddleware`
- `ValidationMiddleware`
- `BackpressureMiddleware`

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

Logging is provider-specific and lives outside the core package:

```go
ep = zapadapter.LoggingMiddleware(logger, "CreateUser")(ep)
// Or use observability/slog with a standard-library slog.Logger.
```

This keeps the core `endpoint` import graph limited to the Go standard library.

Specialized middleware packages:

- `integrations/circuitbreaker`
  - `Gobreaker`
- `integrations/ratelimit`
  - `NewErroringLimiter`
  - `NewDelayingLimiter`

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

Specialized modules such as `integrations/circuitbreaker` and
`integrations/ratelimit` remain independently selectable components.

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
