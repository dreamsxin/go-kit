# Changelog

All notable v2 changes are recorded here. Legacy history remains available
through the immutable v0 and v1 tags.

## [Unreleased]

## [2.2.0] - Release Candidate

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
the compatibility policy in [RELEASE.md](RELEASE.md).

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
