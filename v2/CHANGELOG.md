# Changelog
English | [简体中文](CHANGELOG_zh.md)

All notable v2 changes are recorded here. Legacy history remains available
through the immutable v0 and v1 tags.

## [Unreleased]

### Added

- `security` root package: transport-neutral subject contracts for
  authentication boundaries — `Subject`, `Authenticator`, `WithSubject` /
  `SubjectFromContext`, and endpoint middleware `Middleware`,
  `RequireAuthenticated` (401), `RequireRole` (403), all classified through
  apperror so every transport maps them uniformly.
- `kit.NamedLifecycle` and `kit.ReadinessProvider`: lifecycle components can
  name themselves for startup/failure/shutdown diagnostics and bridge
  asynchronous warm-up into the `/readyz` and `/health` readiness checks.
- `apperror` convenience constructors: `InvalidArgument`, `Unauthenticated`,
  `PermissionDenied`, `NotFound`, `Conflict`, `Unavailable`, and `WrapCause`
  for cause-preserving errors without a public message.
- `endpoint.ValidationError` implements `apperror.Kinder` and
  `apperror.KindNamer`, so transports map validation failures through the
  standard apperror path.
- `sd/client.NewEndpoint` accepts a nil logger and falls back to
  `slog.Default()`.
- `slogadapter.NewTelemetry` assembles the built-in observability dimensions
  into one middleware chain with the canonical order (tracing → metrics →
  logging) and feeds one `endpoint.Metrics` collector; `Telemetry.Apply`
  installs the chain on a Builder with stable labels.
- Server-Sent Events moved to the transport layer: `server.NewSSEServer` and
  `server.NewSSEServerTyped` adapt a streaming handler to `http.Handler`
  with the standard Server hooks (ServerBefore, decode-before-headers,
  ServerErrorHandler, ServerFinalizer); `kit.HandleSSETyped` registers typed
  streams through the endpoint middleware chain, so authentication, tracing,
  metrics, and validation now apply to streams. One stream counts as one
  request; decode failures map to regular error responses.
- ARCHITECTURE documents the module dependency layers (L0–L5) and context
  key conventions.

### Fixed

- `kit` lifecycle watchers now consume asynchronous component errors until
  shutdown; previously only the first error per component was reported.

### Removed (breaking)

- `server.HTTPError`, `server.NewHTTPError`, and `server.WrapHTTPError`.
  Carrying an HTTP status out of the endpoint or service layer crossed the
  layer boundary. Classify failures with `apperror` (transport-neutral, works
  over gRPC too); protocol-specific customization remains available through
  `transporthttp.StatusCoder`, `ErrorCoder`, `PublicMessager`, and `Headerer`.
- The deprecated `log` compatibility facade. Use `log/slog` directly.
- `kit.SSEWriter` and the stream-function signature of `kit.HandleSSE`.
  Streaming now lives in the transport layer: use `server.SSEStream` (same
  method set) with `kit.HandleSSETyped` (endpoint middleware applies) or
  the `HTTP.HandleSSE` method for raw streaming handlers.
- `kit.Service`, `kit.New`, `kit.MustNew`, and `Service.Run` (breaking).
  Assembly is now split into a transport-neutral `kit.Host` orchestrating
  `kit.Lifecycle` components and a `kit.HTTP` component owning routes,
  health checks, and the HTTP server. Use `kit.NewHTTP` / `kit.MustNewHTTP`
  for the component and `kit.NewHost(kit.WithLifecycle(...))` + `Host.Run`
  for the process. `HandleJSON*` and `HandleSSETyped` take `*kit.HTTP`;
  `Handle`, `HandleFunc`, and `HandleSSE` are methods on `*kit.HTTP`. Pure
  worker or gRPC-only services can now run through a Host without HTTP.

### Changed

- Documentation now references the core `endpoint` circuit breaker and rate
  limiter instead of the removed `integrations/circuitbreaker` and
  `integrations/ratelimit` adapters.
- microgen (breaking for generated projects): the user-owned
  `cmd/custom_routes.go` hook is standard-library only now —
  `func registerCustomRoutes(r *http.ServeMux)` registers routes directly and
  `customRouteDescriptions() []string` ("METHOD /path" entries) feeds
  `/debug/routes` and the startup route listing, replacing the old return of
  generator-internal route entries. Generated projects write manifest schema
  `microgen.project.v3`; extend rejects pre-v3 projects with an actionable
  migration hint.

## [2.5.2] - 2026-08-22

### Added

- Custom body formats over HTTP: `server.RawBodyCodec` and
  `server.RawBodyCodecWithMaxBytes` turn two pure functions into the
  transport codec (bounded body, preserved StatusCoder/Headerer), and
  `server.TextErrorEncoder` keeps error responses in the route's format
  instead of defaulting to JSON. apperror is optional on such routes; the
  framework forces no error model.
- `examples/customcodec`: a runnable custom-format service (length-prefixed
  binary body over HTTP, format-matched error responses).

## [2.5.1] - 2026-08-22

### Added

- Dual-protocol binding: `transport.Binding[Req, Resp]` carries one
  middleware-built endpoint and serves HTTP and gRPC without duplicated
  assembly. `Binding.TypedEndpoint()` feeds the typed JSON servers directly;
  `grpcserver.NewServer` with the two protobuf mapping functions covers the
  gRPC side. The same middleware chain runs on both protocols.

## [2.5.0] - 2026-08-22

### Added

- gRPC custom error-kind mapping: `grpcserver.ErrorEncoderWithKindMapper`
  resolves the gRPC code through an application mapper first and falls back
  to the built-in mapping; `grpcserver.CodeForErrorKind` exposes the built-in
  mapping for composition.
- MCP interaction error mapping: `mcp.RPCCodeForInteractionError` and
  `mcp.ErrorMapperForInteraction` map the interaction sentinel errors to
  JSON-RPC codes, so custom mappers build on a documented contract instead of
  reverse-engineering the handler.
- Response assembly combinator: `server.WrapJSONResponse` wraps the response
  value (envelope, post-processing) while preserving the original response's
  StatusCoder and Headerer behavior.
- Middleware chain introspection: `endpoint.Builder.UseNamed` labels
  middleware and `Builder.Describe` returns the chain in application order;
  the built-in `With*` shortcuts record labels automatically. Startup logs
  can now print the assembled chain.
- Startup-time request validation: `transporthttp.ValidateQueryStruct[T]`
  checks query/path request struct tags and supported field types at
  assembly, surfacing unsupported types at startup instead of the first
  request.

## [2.4.4] - 2026-08-22

### Added

- Custom error-kind status mapping: `server.JSONErrorEncoderWithKindMapper`
  resolves the HTTP status through an application mapper first and falls back
  to the built-in mapping for unknown kinds;
  `server.HTTPStatusForErrorKind` exposes the built-in mapping for
  composition. Applications can now define their own `apperror.Kind` values
  with custom statuses without replacing the whole error encoder.
- `client.DecodeJSONResponse` and `client.DecodeJSONResponseWithMaxBodyBytes`
  export the default JSON response decoder, so a custom client composed with
  `NewExplicitClient` reuses the same status handling and body limit.

## [2.4.3] - 2026-08-22

## [2.4.2] - 2026-08-23

## [2.4.1] - 2026-08-22

## [2.4.0] - 2026-08-21

### Added

- Transport-level response assembly: `server.ServerResponseEncoder` overrides
  the success encoder for the JSON entry points, and
  `kit.WithJSONServerOptions` applies server options (envelope, error format,
  hooks) to every JSON route in one place. Per-route options take precedence.
- `examples/envelope`: response assembly at the transport boundary - business
  handlers stay envelope-free while `{code, message, data}` and its matching
  error format are defined once at assembly.
- Documentation for composition and nesting: the transport guide explains
  accumulating versus replacing components and how to combine body, path,
  query, and multipart parsers; the endpoint guide documents the four
  middleware flow-control patterns (short-circuit, branch, repeat, replace).

## [2.3.0] - 2026-08-20

### Added

- W3C Trace Context propagation in the core packages: `endpoint.TraceContext`
  with `ParseTraceparent`, and `transport/http` `ExtractTraceparent` /
  `InjectTraceparent` RequestFuncs for servers and clients.
  `endpoint.TracingMiddleware` now joins an incoming trace context under the
  same trace ID and mints W3C-conformant 32-hex-character trace IDs otherwise.
- `kit.HandleSSE` and `kit.SSEWriter` for Server-Sent Events streams with
  per-event flushing and client-disconnect cancellation; named, JSON, and
  multi-line data events, comment heartbeats, and retry hints.
- `server.ParseMultipartForm` for bounded multipart/form-data uploads
  (total-body, per-file, and in-memory caps; 413/415/400 classification) and
  `server.WriteAttachment` for sanitized file downloads.
- Request validation convention: `endpoint.Validatable`,
  `endpoint.ValidationMiddleware`, and `endpoint.ValidationError` with
  field-level failures; the HTTP error encoder maps them to 400 with the
  stable code `bad_request.validation`.
- Pagination convention in `transport/http`: `ParsePage` (defaults 1/20,
  size capped at 100, invalid query values rejected as validation errors),
  `Page.Limit/Offset`, and the generic `PageResult[T]` wire shape.
- `examples/auth`: application-owned authentication and authorization
  middleware with Bearer API keys, 401/403 responses classified by
  `apperror`, and public health routes.
- `examples/todosvc`: an end-to-end SQLite CRUD service with a CGO-free
  repository, `apperror` classification, path-parameter routes, and
  database closure during graceful shutdown.
- Resilience middleware: `endpoint.Fallback` answers with a fallback
  endpoint when the primary fails, and `endpoint.BulkheadMiddleware`
  isolates concurrency per resource key; `ErrBackpressure` and the new
  `ErrBulkheadFull` now encode as HTTP 429 instead of 500.
- PRODUCTION.md gains Deployment (static containers, probe wiring,
  termination-budget alignment, config injection), Alerting (starter
  alert set mapped to the documented metric signals), and Background
  Jobs (directory structure and `kit.Lifecycle` wiring) sections.

### Changed

- Trace IDs minted by `endpoint.TracingMiddleware` are 32 lowercase hex
  characters (W3C format) instead of 16. Existing callers that treat the ID
  as an opaque string are unaffected.

### Fixed

- `microgen` no longer registers the `-add-tables` flag: it was parsed but
  never consumed, misleading users into thinking extend mode could append
  tables. MICROGEN.md now states the covered generator version and lists the
  source-mode and `-service` options with defaults.

## [2.2.0] - 2026-08-14

This development cycle contains one explicitly approved, narrowly scoped v2
SemVer exception: `Metrics.Snapshot()` now returns `MetricsSnapshot` instead of
`Metrics`. The migration is documented in [MIGRATION.md](MIGRATION.md).

### Added

- Transport-neutral `apperror` classification with consistent HTTP and gRPC
  mappings, including a default gRPC error encoder.
- Fully typed JSON assembly helpers in `kit` and `transport/http/server` for
  concrete request and response types.
- A lock-free `endpoint.MetricsSnapshot` value with `AverageDuration()`.
- A bounded request-ID validator option and conservative validation for
  caller-supplied request IDs.
- Application-owned generated configuration extensions in `config/custom.go`,
  including defaults, environment, and validation hooks preserved on rerun.

### Changed

- **Approved SemVer exception:** `Metrics.Snapshot()` returns
  `MetricsSnapshot`. Field access through an inferred local variable is
  unchanged; code explicitly declaring the result as `Metrics` must update its
  type.
- `microgen` uses a pure-Go SQLite introspection driver, so installing and
  running the CLI no longer requires CGO or GCC.
- Generated demo clients delegate to the generated Go SDK instead of maintaining
  a second HTTP/gRPC implementation.
- Health checks execute concurrently with per-check timeouts and allow at most
  one active invocation of each named check.
- Quickstart and composition examples use typed handlers and propagate listener
  and shutdown errors through the process lifecycle.
- The `main` branch now contains only the maintained v2 product line; legacy v1
  source and documentation remain available through immutable release tags.
- The repository-root README is now a concise v2 entry point instead of a
  duplicate v1 usage guide.

### Fixed

- Default HTTP and gRPC error encoders no longer expose unclassified internal
  error messages to clients, while classified application errors retain stable
  public codes and messages.
- Concurrent health probes no longer accumulate serial latency or launch
  overlapping timed-out checks.
- Example metrics reads use atomic snapshots, and snapshots no longer carry an
  internal mutex that triggers `go vet` copylock failures.
- Public API snapshot collection ignores tool download diagnostics written to
  stderr.

## [2.1.0] - 2026-08-14

This release is the explicitly approved direct-refactor v2 SemVer exception. It is
not source compatible with `v2.0.0`; follow [MIGRATION.md](MIGRATION.md) before
upgrading.

### Added

- Executable architecture dependency gates and a reviewed dependency closure
  comparison against the published `v2.0.0` baseline.
- A provider-neutral `kit.Lifecycle` contract and optional `kit/grpc`
  component for multi-server applications.
- Manifest-driven verification for the core and all independently versioned
  modules through `make verify-published-core` and `make verify-published`.
- A 4 MiB default success-response limit for `transport/http/client.NewJSONClient`,
  with an explicit constructor for larger contracts.
- Transport-owned interaction session release through `Runtime.ReleaseSession`.

### Changed

- The repository owner approved publishing the direct incompatible refactor
  under `/v2` as a documented direct-refactor SemVer exception; `v2.0.0` remains
  immutable and the published root refactor release is `v2.1.0`.
- The root runtime module now has no third-party requirements. gRPC, Consul,
  Gobreaker, rate limiting, Zap, and OpenTelemetry live in independent modules.
- `microgen` implementation packages are internal, generated code uses direct
  `slog`, and minimal HTTP projects omit optional provider and database modules.
- The recommended example is now `examples/quickstart` using `kit`; explicit
  endpoint/transport wiring lives in `examples/manual_composition`.
- Core `kit` is an HTTP-only assembly layer. Endpoint middleware with external
  dependencies is installed explicitly through `kit.WithEndpointMiddleware`.
- HTTP and gRPC transports default to a no-op error handler; error reporting is
  application-owned through the `observability/slog` or `integrations/zap`
  adapters.
- `microgen` now defaults config, model/repository, and database runtime wiring
  to off. `-from-db` still always emits the introspected models.
- Full regeneration preserves user-owned service implementations, `cmd/main.go`,
  config YAML, and project README; endpoint and transport artifacts are tracked
  as generator-owned in the manifest.
- Generated debug route registration and route printing are opt-in, rate limiting
  is disabled by default, middleware timeout comes from config, and inbound retry
  configuration has been removed.
- Generated HTTP and gRPC listeners bind before serving; generated database
  handles are checked and closed during shutdown.
- MCP tool calls reuse the runtime session bound to the MCP transport session,
  which is released on DELETE or TTL expiry.
- Go IDL generation fails on an invalid interface method instead of silently
  omitting the method.

### Fixed

- Consul remote config loading now honors its timeout and response size limit
  without pulling a second Viper-based provider stack into generated projects.
- The documented v2 release tag is the root `v2.0.0` tag required by Go module
  resolution; the historical incorrect `v2/v2.0.0` tag was removed.

### Removed

- Old provider-owned package paths under `endpoint`, `transport/grpc`, and
  `sd/consul`; use the corresponding independent integration modules.
- `kit.WithGRPC`, `Service.GRPCServer`, `kit.WithRateLimit`,
  `kit.WithCircuitBreaker`, `kit.WithLogging`, and the root transport
  `NewLogErrorHandler` convenience API.
- Dead combined config template and obsolete Swagger 2.0 Make targets/tooling.

## [2.0.0] - 2026-07-20

First stable v2 release. Exported runtime APIs, the `microgen` CLI and
configuration, generated ownership, and documented protocol behavior now follow
the compatibility policy in [RELEASE.md](internal/docs/RELEASE.md).

### Added

- Independent `github.com/dreamsxin/go-kit/v2` module.
- Error-returning `kit.New`, context-driven `Service.Run`, configurable graceful
  shutdown timeout, and `kit.MustNew` for explicit panic-on-invalid setup.
- Final generated configuration validation for server, logging, database,
  middleware, and remote-provider settings.
- Deterministic formatting and text normalization for generated output.
- Repository-wide UTF-8 validation that rejects BOMs, invalid byte sequences,
  and Unicode replacement characters in maintained text files.
- External generated-project smoke coverage using `go mod tidy` and
  `go test ./...`.
- Shared HTTP path/query codec for generated transports, clients, and SDKs.
- OpenAPI 3.1 generation and a standalone JSON Schema 2020-12 bundle directly
  from the common `microgen` IR.
- Zero-runtime-dependency TypeScript Fetch clients generated from the same IR,
  with strict compiler settings and external type-check coverage.
- Shared non-GET path parameter encoding and decoding for generated transports,
  clients, and SDKs.
- Versioned `.microgen/manifest.json` project identity with source, capability,
  route, service, model, middleware, and generator-owned artifact metadata.
- OpenAPI 3.1 parser validation and JSON Schema 2020-12 compilation for Go IDL,
  Protobuf, and database generation integration paths.
- A release contract check that type-checks generated SDKs with a pinned
  TypeScript compiler.
- Shared executable HTTP behavior coverage for generated Go and TypeScript SDKs,
  including path, query, body, headers, and non-2xx errors.
- Reviewed deterministic contract snapshots for Go IDL, Protobuf, and database
  generation paths.
- Optional `observability/slog` endpoint logging and independent
  `observability/otel` tracing/metrics adapters with application-owned provider
  setup.
- Optional standard-library `security/http` middleware for trusted proxy and
  client IP resolution, IP policy, CORS, signed double-submit CSRF, and security
  response headers.
- `kit.WithHTTPMiddleware` for whole-server standard-library middleware across
  health, endpoint, raw HTTP, and generated routes.
- Release gates for reviewed public API drift, maintained Markdown links,
  module tidy state, focused race tests, vet, and committed v2-scope cleanliness.

### Changed

- `kit` no longer installs process signal handlers or calls fatal logging during
  service lifecycle.
- `Service.GRPCServer` returns an error when gRPC is not configured.
- Generated config precedence is local YAML, optional remote config, final
  environment overrides, then validation.
- Service-discovery registration returns its initial snapshot synchronously and
  publishes later updates without closing consumer channels.
- In-memory interaction providers copy mutable resources, blobs, templates,
  prompts, and render arguments.
- Generated HTTP servers use the standard library `http.ServeMux`; generated GET
  clients and servers share one tagged query contract and do not send JSON bodies.
- Generated Go clients and SDKs use the same complete HTTP paths as server route
  registration and OpenAPI output.
- Generated OpenAPI projects embed Swagger UI 5 assets and serve both
  `/openapi.json` and `/schema.json` without CDN dependencies.
- HTTP JSON client timeout construction is explicit through
  `NewJSONClientWithTimeout`.
- Service-discovery retry defaults to one attempt and only retries explicitly
  classified transient errors when additional attempts are configured.
- Service-discovery endpoint constructors return an owned closer and validate
  required dependencies and timing options before starting background work.
- v2 documentation is task-oriented and no longer duplicates v1 release history,
  temporary roadmaps, or session snapshots.
- Extend scanning uses the project manifest as its primary capability source and
  reports filesystem or ownership drift before mutation.
- Generated Go SDKs expose `APIError` with stable status-code and response-body
  fields, aligned with the TypeScript SDK error contract.
- MCP Streamable HTTP now enforces the initialization lifecycle, protocol
  version, browser Origin policy, and client sampling capability. Logging levels
  are session-scoped, and server messages use one active SSE stream.
- `kit` and generated HTTP servers use streaming-safe defaults with bounded
  header reads and no default response write deadline.
- Consul registrar operations return errors; instancer shutdown cancels and
  joins the active blocking query.
- Generated Go SDKs resolve URLs structurally and bound response bodies.
- Generated repositories accept only model-derived ordering fields.

### Fixed

- Prompt render callbacks no longer run while the provider lock is held.
- Consul retry waits respond to shutdown and repeated `Stop` calls are safe.
- Endpointer shutdown no longer races with producer sends on a closed channel.
- Endpointer shutdown waits for its update loop and releases every client
  resource still owned by the endpoint cache.
- Endpoint caches no longer sort caller slices in place or expose their internal
  endpoint slice to callers.
- Generated environment values remain the highest-priority config source after
  remote loading.
- Generated Go files fail generation before a malformed partial file is written.
- Append-service and append-model refresh OpenAPI, JSON Schema, and TypeScript
  client artifacts instead of leaving generated contracts stale.
- Service, model, and middleware append operations refresh the project manifest
  last and reject projects with unresolved manifest drift.
- Database-derived contract IR now matches generated Go IDL for optional create
  fields, update fields, list query parameters, and response JSON shapes.
- HTTP response-writer interception preserves optional streaming interfaces,
  ignores repeated status writes, and accounts for `io.ReaderFrom` bytes.
- Buffered HTTP client decode failures close the response body and cancel the
  request context.
- gRPC clients preserve caller-provided outgoing metadata while applying hooks.
- MCP tool-triggered sampling now uses the transport session from request
  context, and tool execution failures return `isError: true` results.

### Removed

- v1 compatibility claims and v1.0/v1.6 release planning from v2 documentation.
- Duplicate architecture, generator design, project snapshot, roadmap, stability,
  observability, security, and maintainer documents.
- Duplicate HandyBreaker and built-in Hystrix implementations; Gobreaker is the
  single circuit-breaker adapter in core.
- Redundant `sd.NewEndpointCloser`; lifecycle ownership is part of every
  `sd.NewEndpoint` construction.
- Swagger 2.0 annotation output, `swagger_host`, and `APP_SWAGGER_HOST`; Swagger
  UI now reads the generated `/openapi.json` contract.
- The non-standard `microgen -skill` option, generated `skill/` package,
  `/skill` discovery endpoint, repository AI `SKILL.md`, and dedicated skill
  example. OpenAPI/JSON Schema remain the general contract formats, while the
  optional interaction runtime exposes tool discovery and execution through MCP.
