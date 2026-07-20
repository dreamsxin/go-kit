# Architecture And Boundaries

This document defines the durable architecture of go-kit v2. It describes
ownership and extension rules, not a temporary implementation roadmap.

## Product Scope

go-kit v2 is a component-oriented framework for building Go services with a
consistent runtime model and a contract-driven generator.

The framework provides:

- service, endpoint, and transport separation;
- endpoint middleware for cross-cutting request behavior;
- HTTP and gRPC adapters;
- service discovery, balancing, and controlled execution;
- interaction primitives and MCP transport;
- project generation from Go IDL, Protobuf, and database schemas;
- a small-service assembly API through `kit`.

The core does not provide business platforms. IAM, outbox workflows, job
leasing, object storage, secret management, and complete transaction frameworks
belong in independent integration modules or applications.

## Request Path

```text
Transport request
    -> decode
    -> endpoint middleware
    -> endpoint
    -> service method
    -> encode
    -> transport response
```

Each layer owns one kind of decision:

| Layer | Owns | Must not own |
| --- | --- | --- |
| Service | Business rules and domain orchestration | HTTP/gRPC types and status mapping |
| Endpoint | Transport-neutral request boundary and middleware | Socket/server lifecycle |
| Transport | Protocol decode, encode, headers, and status | Business rules and retry policy |
| Assembly | Dependency wiring and process lifecycle | Hidden global state |

## Package Responsibilities

### `kit`

`kit` is a high-level assembly scaffold for small services. It composes the
normal endpoint and transport packages and owns HTTP/gRPC server lifecycle.

- `kit.New` validates configuration and returns an error.
- `Service.Run(ctx)` follows a caller-owned context.
- `kit.HandleJSON` and `kit.HandleJSONEndpoint` preserve endpoint middleware.
- `Service.Handle` and `Service.HandleFunc` are raw HTTP escape hatches.

Application routes should not be moved to raw HTTP handlers merely to reduce a
few lines of endpoint wiring.

### `endpoint`

`endpoint` defines the transport-independent request function, middleware
composition, timeout, metrics, logging, rate limiting, and circuit breaking.

Endpoint middleware observes business call results. It should not infer errors
from HTTP status codes or gRPC wire details.

### `transport`

Transport packages adapt endpoints to protocols:

- `transport/http/server` and `transport/http/client`;
- `transport/grpc/server` and `transport/grpc/client`.

They own bounded decoding, response status handling, protocol metadata,
streaming interfaces, and transport-specific errors. They do not decide whether
a business operation is safe to retry.

### `sd`

`sd` converts discovered instances into endpoint sets and executes calls through
balancers and optional retry strategies. Updates are snapshots, not mutable
caller-owned slices. Cancellation must interrupt both calls and retry backoff.
Constructors return explicit closers for subscription goroutines and
factory-created client connections.

### `interaction`

`interaction` defines tools, resources, prompts, sessions, notifications, and
policy hooks. `interaction/mcp` exposes those capabilities through MCP
Streamable HTTP.

MCP is an optional standards-based integration surface. General contract
discovery remains OpenAPI/JSON Schema; the framework does not maintain a
parallel proprietary tool-discovery endpoint.

Provider implementations must copy mutable caller data and must not invoke user
callbacks while holding internal locks.

### `log`

`log` is the framework logging adapter. Libraries return errors; process entry
points decide when to terminate.

### Optional observability adapters

`observability/slog` adapts endpoint outcomes to the standard-library
`log/slog` API without changing the core zap logger. `observability/otel` is a
separate module that adapts endpoint calls to application-owned OpenTelemetry
tracers and meters. Neither adapter logs or records request/response payloads;
operation names and application attributes must remain bounded.

### Optional HTTP security

`security/http` wraps standard-library handlers with trusted-proxy resolution,
client-IP policy, CORS, signed double-submit CSRF, and security headers. It is
assembled around transport handlers and does not change endpoint contracts.
Authentication establishes a principal at the protocol boundary; business
authorization remains in endpoint or service policy.

### `cmd/microgen`

`microgen` is a build-time tool. Parsers produce a common IR that drives HTTP
routes, transports, Go and TypeScript SDKs, OpenAPI 3.1, JSON Schema 2020-12,
and optional MCP tool adapters. Templates render projects from that IR. Runtime
packages must not depend on generator internals.

See [MICROGEN.md](MICROGEN.md) for source modes and generated-file ownership.

## Middleware Boundary

Endpoint middleware and HTTP middleware are intentionally different:

- endpoint middleware sees decoded requests, business responses, and business
  errors;
- HTTP middleware sees methods, paths, headers, status codes, and byte streams.

Metrics, logging, timeout, rate limit, and circuit breaker options configured on
`kit` apply to routes registered through `HandleJSON` or
`HandleJSONEndpoint`. Raw handlers receive only explicitly installed HTTP
middleware.

`kit.WithHTTPMiddleware` is the explicit whole-server boundary for standard
`http.Handler` middleware. It wraps health, JSON endpoint, raw HTTP, and
generated routes without converting HTTP policy into endpoint middleware.

Circuit breakers are scoped per route. Business validation errors should not be
treated as infrastructure failure unless an application explicitly classifies
them that way.

## Error And Retry Contract

- Libraries return errors instead of logging fatal or installing signal
  handlers.
- Transport clients treat non-success protocol status as errors.
- Retry is opt-in. Production callers should provide an explicit retryable error
  classification; the built-in default treats unknown errors as permanent.
- Write operations are not retried merely because an error occurred.
- Backoff waits honor context cancellation.

## Lifecycle Contract

The process entry point owns signals and root context. Framework services own
listeners and graceful shutdown after startup succeeds.

```text
main creates signal context
    -> assemble dependencies
    -> start service
    -> wait for cancellation or serve error
    -> bounded graceful shutdown
    -> return final error to main
```

Startup errors must be synchronous when possible. A service instance cannot be
started twice or restarted after shutdown.

Resource-owning constructors return a closer. Shutdown proceeds from consumers
to providers: close endpoint/endpointer resources before stopping their
Instancer, then close transports and process-level dependencies.

## Extension Rules

Prefer, in order:

1. Compose existing public packages.
2. Add a small option or interface at the package that owns the behavior.
3. Add an optional integration package.
4. Change core contracts only when the behavior is broadly required.

Avoid global registries, hidden goroutines, package-level process control, and
framework branches for one application.

## v2 Stability

v2.0.0 freezes the compatibility contract for exported runtime APIs, module
paths, CLI flags, generated ownership boundaries, documented configuration keys,
and documented protocol behavior. Compatible changes follow semantic versioning;
incompatible changes require a new major version and must be deliberate, tested,
and documented in [CHANGELOG.md](CHANGELOG.md) and [MIGRATION.md](MIGRATION.md).
