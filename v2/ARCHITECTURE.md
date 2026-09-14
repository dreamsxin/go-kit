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
belong in application code or explicitly chosen integration packages.

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
logging remains in explicit adapters. In-memory rate limiters and circuit
breakers live in `endpoint`, with structural contracts for replacement policies.

`Recorder` is the metrics extension point: `RecordingMiddleware` hands every
call an `Observation` (operation, duration, error), and any backend bridge is an
implementation of that interface. `Metrics` is the built-in in-memory collector.
Its counters are unexported and guarded internally, so the read paths are
`Snapshot()`, `SnapshotFor(operation)`, and `Operations()`, each returning a
copyable value detached from that state under a short critical section.

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
`sd/endpointer`, `sd/selector`, `sd/balancer`, `sd/retry`, `sd/feedback`, and
`sd/health` are independently usable runtime components; `sd/client` is their
optional convenience composition. Updates are snapshots, not mutable
caller-owned slices. A `Balancer.Pick` returns a `Picked` identity plus
`Done(Outcome)`, so retry and other callers can feed per-instance results into
local feedback without writing live metrics to the registry. Business call
feedback lives in one store, `feedback.Table`; active probe verdicts belong to
`health.Check`, and policy state such as which addresses are currently ejected
belongs to the policy, because it is per (policy, instance) rather than per
instance. A decorator never owns what it was handed: `balancer.New`,
`health.Check`, `selector.Subscribe`, and `endpointer.Filter` leave the source to
whoever built it, while `Selector.Close` and `Balancer.Close` release the
strategy chain they were given — which is why a wrapping strategy must forward
`Close` (see `selector.CloseStrategy`). Dependencies point one way: the
endpoint and selection layers do not import `sd/feedback` or `sd/health`, so an
assembly that does not use them does not compile them in. Active probing is a
decorator on `Instancer` rather than a stage of its own, so adding it changes no
downstream layer. Cancellation interrupts both calls and retry backoff. `Instancer.Close`
and constructor-returned closers own subscription goroutines and factory-created
client connections. Protocol retry classification belongs to the protocol
adapter, not generic discovery. Consul and etcd support live in optional
provider packages in the same published module.

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
`observability/otel` adapts endpoint calls to
application-owned OpenTelemetry tracers and meters. `observability/metrics`
renders what `endpoint.Metrics` already collected as Prometheus text exposition,
using the standard library only — a dependency gate keeps any metrics client out
of it, so mounting a scrape endpoint costs nothing; `observability/metrics/grpc`
is the gRPC bridge, kept separate so that gate holds. These adapters do not log
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
authorization remains in endpoint or service policy. An MCP endpoint adds one
seam between the two: `mcp.MethodAuthorizer` decides which methods and targets
that principal may reach, before any provider or tool is consulted.

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
    -> announce draining (readiness fails, Draining components told)
    -> wait the drain delay
    -> bounded graceful shutdown, then close what is left
    -> return final error to main
```

Startup errors must be synchronous when possible. A service instance cannot be
started twice or restarted after shutdown.

Components may implement `kit.NamedLifecycle` to attach a stable name to
startup, asynchronous failure, and shutdown diagnostics, `kit.ReadinessProvider`
to bridge asynchronous warm-up into the `/readyz` and `/health` readiness checks,
and `kit.Draining` to be told the process is stopping before any shutdown runs.
Asynchronous component errors are reported to the service error channel until
shutdown.

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

All runtime components, providers, and the generator ship in one Go module.
Independence is enforced at package imports: an HTTP-only application does not
compile a gRPC, registry, database, or telemetry adapter merely by using `kit`.
The module's dependency metadata still includes those optional dependencies.

| Boundary | Packages | Responsibility |
| --- | --- | --- |
| Request contracts | `endpoint`, `apperror`, `transport` | Request functions, middleware, classification, error reporting |
| Process health | `health` | Local liveness/readiness evaluation and HTTP probe mounting |
| HTTP protocol | `transport/http`, its `server` and `client`, `security/http` | Protocol encoding, propagation, HTTP policy |
| Discovery | `sd` and its subpackages | Snapshots, connection ownership, selection, retry, remote health and feedback |
| Process assembly | `kit` | Host lifecycle and an HTTP serving component |
| Optional gRPC | `integrations/grpc`, `kit/grpc` | gRPC transport adapters and a Host-compatible serving component |
| Other optional integrations | `integrations/consul`, `integrations/etcd`, `integrations/zap`, `observability` | Registry providers, logging and telemetry adapters |
| Interaction | `interaction`, `interaction/mcp` | Interaction runtime and its MCP protocol adapter |
| Build tooling | `cmd/microgen` | Generate applications; never imported by runtime packages |

The principal import directions are:

```text
kit/grpc -> kit, health, integrations/grpc, google.golang.org/grpc
kit -> health, endpoint, sd, transport/http/server
integrations/grpc -> endpoint, apperror, transport, google.golang.org/grpc
integrations/consul or integrations/etcd -> sd, sd/instance, provider SDK
sd/client -> sd/endpointer, sd/balancer, sd/retry
sd/balancer -> sd/endpointer, sd/selector
sd/feedback -> sd/balancer, sd/endpointer, sd/selector
sd/health -> sd/instance
```

These arrows describe imports, not startup order. `Host` accepts lifecycle
components through interfaces; it does not import the providers it runs.
Runtime components outside the assembly packages must not import `kit` or
generator internals. `endpoint` imports only
the standard library, and `security/http` imports nothing from this module.
`TestArchitectureDependencyGates`, `TestComponentsDoNotDependOnAssembly`, and
`TestKitHTTPAssemblyDoesNotResolveOptionalDependencies` enforce these boundaries.

Generated applications import the packages their enabled features need. Minimal
HTTP generation stays provider-free; gRPC generation imports
`integrations/grpc`, database generation imports its selected driver, and MCP
generation imports `interaction/mcp`. Generated code is runtime application
code; the generator itself remains a build-time dependency.

### Discovery Ownership

| Component | Owns | Leaves to its caller |
| --- | --- | --- |
| `sd/instance` | Snapshot copies, equality and subscriber delivery | Source acquisition and retry policy |
| `sd/health` | Remote probe workers and health verdicts; preserves source errors | Source lifecycle and downstream stale-view policy |
| `sd/endpointer` | Subscription and factory-created endpoint closers | Source lifecycle and selection strategy |
| `sd/selector`, `sd/balancer` | Selection and any owned strategy resources | Request execution and source lifecycle |
| `sd/retry` | Attempt budget, backoff, outcomes and error history | Idempotency policy and balancer lifecycle |
| `feedback.Table`, `Ejector` | Measurements and policy decisions, respectively | Discovery ownership; `Measured` supplies their shared subscription |

Process `health.Registry` answers whether this process is ready; `sd/health`
answers which remote instances passed probes. They remain separate because
neither result can establish the other's truth. A discovery error is not cleared
by a healthy probe. Consumers decide how long to keep their last successful
snapshot through their shared subscription state machine.

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

`v2.22.0` is the current released contract. It establishes the reviewed package
graph, lifecycle ownership, transport error model, and generated project layout
captured by the release manifest and API snapshot. Until the v2
compatibility freeze, minor releases may change behavior or remove APIs; after
the freeze, incompatible changes require a new major module version.

`v2.22.1` is the patch candidate on `main`, correcting discovery-state handling
and selection without changing public signatures or package boundaries.
