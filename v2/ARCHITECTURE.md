# Architecture And Boundaries
English | [简体中文](ARCHITECTURE_zh.md)

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

`kit` is a high-level assembly scaffold for small services. The
transport-neutral `Host` orchestrates lifecycle components without owning any
protocol; the `HTTP` component composes the normal endpoint and HTTP
transport packages and mounts into a Host through the provider-neutral
`kit.Lifecycle` contract.

- `kit.NewHTTP` and `kit.NewHost` validate configuration and return errors.
- `Host.Run(ctx)` follows a caller-owned context.
- `kit.HandleJSONTyped`, `kit.HandleJSON`, and `kit.HandleJSONEndpoint` preserve
  endpoint middleware; the typed entry point is preferred for concrete
  responses.
- `HTTP.Handle` and `HTTP.HandleFunc` are raw HTTP escape hatches.
- `kit/grpc` is an optional lifecycle component and is not imported by core
  `kit`.

Application routes should not be moved to raw HTTP handlers merely to reduce a
few lines of endpoint wiring.

### `endpoint`

`endpoint` defines the transport-independent request function and standard
library middleware composition, timeout, and metrics. Provider-specific
logging, rate limiting, and circuit breaking remain explicit adapters.

`Metrics` is a mutable collector owned by middleware. Concurrent readers use
`Snapshot()`, which returns the lock-free, copyable `MetricsSnapshot` value.

Endpoint middleware observes business call results. It should not infer errors
from HTTP status codes or gRPC wire details.

### `transport`

Transport packages adapt endpoints to protocols:

- `transport/http/server` and `transport/http/client`;
- `integrations/grpc/server` and `integrations/grpc/client`.

They own bounded decoding, response status handling, protocol metadata,
streaming interfaces (including the Server-Sent Events server), and
transport-specific errors. They do not decide whether
a business operation is safe to retry.

### `sd`

The root `sd` package owns provider-neutral discovery contracts.
`sd/endpointer`, `sd/balancer`, and `sd/retry` are independently usable runtime
components; `sd/client` is their optional convenience composition. Updates are
snapshots, not mutable caller-owned slices. Cancellation interrupts both calls
and retry backoff. Constructors return explicit closers for subscription
goroutines and factory-created client connections. Protocol retry classification
belongs to the protocol adapter, not generic discovery. Consul support lives in
the independent `integrations/consul` module.

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

`log` was a deprecated standard-library compatibility facade for projects
generated before the direct refactor and has been removed. Applications use
`log/slog` directly and opt into provider adapters explicitly. Libraries
return errors; process entry points decide when to terminate.

### Optional observability adapters

`observability/slog` adapts endpoint outcomes and transport errors to the
standard-library `log/slog` API. `integrations/zap` owns equivalent
Zap-specific adapters, so core packages remain provider-neutral.
`observability/otel` is a separate module that adapts endpoint calls to
application-owned OpenTelemetry tracers and meters. These adapters do not log
or record request/response payloads; operation names and application attributes
must remain bounded.

### Optional security

`security` defines transport-neutral subject contracts: the authenticated
principal (`Subject`), `Authenticator`, subject context propagation, and
endpoint middleware for coarse enforcement (`RequireAuthenticated`,
`RequireRole`). Failures are classified through `apperror` so transports map
them uniformly. Credential extraction and validation stay protocol specific
and application owned.

`security/http` wraps standard-library handlers with trusted-proxy resolution,
client-IP policy, CORS, signed double-submit CSRF, and security headers. It is
assembled around transport handlers and does not change endpoint contracts.
Authentication establishes a principal at the protocol boundary; business
authorization remains in endpoint or service policy.

### `cmd/microgen`

`microgen` is a build-time tool. Parsers produce a common IR that drives HTTP
routes, transports, Go and TypeScript SDKs, OpenAPI 3.1, JSON Schema 2020-12,
and optional MCP tool adapters. Templates render projects from that IR. Runtime
packages must not depend on generator internals. Parser, schema, IR, and
generation implementation packages live under `cmd/microgen/internal`; the CLI
is the supported entry point.

See [MICROGEN.md](MICROGEN.md) for source modes and generated-file ownership.

## Middleware Boundary

Endpoint middleware and HTTP middleware are intentionally different:

- endpoint middleware sees decoded requests, business responses, and business
  errors;
- HTTP middleware sees methods, paths, headers, status codes, and byte streams.

Endpoint middleware installed through `kit.WithEndpointMiddleware` applies to
routes registered through `HandleJSON` or `HandleJSONEndpoint`. Raw handlers
receive only explicitly installed HTTP middleware. Dependency-owning adapters
such as Zap or token buckets are created by the application and
passed through this generic option. Circuit breaking and rate limiting are
built into the core `endpoint` package and hold no third-party dependencies.

`kit.WithHTTPMiddleware` is the explicit whole-server boundary for standard
`http.Handler` middleware. It wraps health, JSON endpoint, raw HTTP, and
generated routes without converting HTTP policy into endpoint middleware.

Circuit-breaker scope is application owned. Create one adapter per route when
routes must not share breaker state. Business validation errors should not be
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

Components may implement `kit.NamedLifecycle` to attach a stable name to
startup, asynchronous failure, and shutdown diagnostics, and
`kit.ReadinessProvider` to bridge asynchronous warm-up into the `/readyz` and
`/health` readiness checks. Asynchronous component errors are reported to the
service error channel until shutdown.

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

## Module Dependency Layers

```text
L0 foundation (no third-party dependencies, independently usable)
   apperror · endpoint · transport (root) · transport/http · sd ·
   interaction · security

L1 transport adapters (depend on L0)
   transport/http/server · transport/http/client · security/http

L2 assembly (depends on L0+L1)
   kit · kit/grpc (independent module)

L3 optional composition (independent packages, no new dependencies)
   observability/slog · sd/client

L4 independent modules (third-party dependencies, versioned separately)
   observability/otel · integrations/zap · integrations/grpc ·
   integrations/consul

L5 build-time tooling (never enters the runtime dependency graph)
   cmd/microgen
```

Dependencies point downward only. L0 packages must not import L2–L5.
`sd` requires a caller-provided `Instancer` implementation (for example
`integrations/consul`); `kit` is self-contained and starts its own HTTP
server. `cmd/microgen` output depends on L0–L3 only.

## Context Conventions

- Every framework context key ships as an exported `WithXxx(ctx, v)` /
  `XxxFromContext(ctx)` pair; key types are never exported.
- Request correlation values (trace context, request ID) live in `endpoint`.
  Protocol-native objects (`*http.Request`, response writers) live in the
  transport layer and must not be read by endpoint or service code.
- The authenticated principal travels through `security.WithSubject` /
  `security.SubjectFromContext`.
- Business values (user models, transactions) never occupy framework-reserved
  keys; they move through request structures.

## Stability

`v2.6.0` is the current published contract. It is the approved
architecture-evolution release: the assembly layer (`kit.Service` split into
`kit.Host` + `kit.HTTP`), the error model (`server.HTTPError` removed), and
the generated custom-routes hook changed incompatibly by explicit approval;
the exceptions are recorded in the release policy. Later patch releases fix
behavior and minor releases add capabilities. Further incompatible changes
require a new major module version unless separately approved.
