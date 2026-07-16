# Changelog

All notable user-visible changes should be recorded here.

This project has not reached v1.0. Until then, entries should clearly distinguish stable behavior from preview behavior.

## Unreleased

### Fixed

- **Interaction generator test portability** (`cmd/microgen/generator`): stopped binding schema assertions to gofmt indentation while preserving field and type checks.
- **SSE notification race** (`interaction/mcp`): `TestE2E_ServerNotifications` could fail with `"no active SSE stream for session"` because the notification was sent before the GET handler goroutine registered its SSE writer. Test now retries until the writer is ready.
- **`best_practice` sentinel error** (`examples/best_practice`): `errors.Is` never matched because `errors.New("name is required")` was called inline, creating a new pointer each time. Replaced with a package-level `var errNameRequired`.
- **`Start()` silent failure** (`kit`): HTTP listen errors occurred inside a goroutine and called `log.Fatalf`, killing the process without returning to the caller. `Start()` now binds listeners synchronously via `net.Listen` and returns an `error`.
- **Partial startup rollback** (`kit`): `Service.Start` now binds HTTP and gRPC listeners before starting either server and closes the HTTP listener when gRPC binding fails.
- **Session leak in `callTool`** (`interaction/mcp`): Auto-created sessions were never closed. Added `defer EndSession` for sessions created implicitly during tool calls.
- **`recover()` anti-pattern** (`interaction/mcp/sampling.go`): Replaced `defer recover()` concurrency guard with mutex-protected delete-before-send, eliminating the panic entirely.
- **`context.Background()` in completion handler** (`interaction/mcp`): `handleCompletionComplete` now accepts and propagates `ctx` from the dispatch call instead of using `context.Background()`.
- **`service.tmpl` logging signature**: `LoggingMiddleware` parameter type was `*log.Logger` (stdlib) but the import had been changed to `kitlog`, causing `undefined: log` in all generated projects. Fixed to `*kitlog.Logger`.
- **`service.tmpl` hardcoded NopLogger**: `LoggingMiddleware` and `serviceImpl` both used `kitlog.NewNopLogger()` regardless of config. `ServiceConfig` now accepts a `Logger *kitlog.Logger` field and passes it through.
- **`profilesvc_test.go` unchecked error** (`examples/profilesvc`): HTTP GET response error was ignored before `defer Body.Close()`, triggering a `go vet` warning. Now checks error first.
- **Duplicate resource overwrite** (`interaction`): `MemoryResourceProvider.Register` silently overwrote existing resources. Now returns `ErrResourceExists` sentinel error on duplicate URI.
- **`ResourcePromptMemory` duplicate detection** (`interaction`): Added `ErrResourceExists` sentinel and duplicate check in `Register()`.

### Added

- **Strict JSON decoding** (`transport/http/server`, `kit`, `microgen`): added bounded body decoding, unknown-field rejection, trailing-data rejection, strict JSON handler constructors, and `JSONDecodeError` status mapping. `kit.HandleJSON` and generated HTTP routes now use strict JSON decoding by default.
- **Structured JSON errors** (`transport/http/server`): `JSONErrorEncoder` now emits a stable `code` field while preserving the historical `error` field. Added `HTTPError`, `NewHTTPError`, and `WrapHTTPError` for custom status, error code, public message, and headers.
- **Health checks** (`kit`): added `/livez` and `/readyz` plus `WithLivenessCheck` and `WithReadinessCheck`; `/health` remains compatible and combines configured checks.
- **HTTP server configuration** (`kit`): added `WithHTTPServerConfig` for read/write/idle timeouts and maximum header size when using `Service.Start`.
- **Asynchronous serve errors** (`kit`): added `Service.Errors()` so applications can react to HTTP or gRPC serving failures after startup.
- **`examples/kit_basic`**: New standalone runnable example demonstrating the high-level `kit.New` + `kit.JSON` + `svc.Handle` + `svc.Run` API — the fastest path from zero to a running service. Includes 5 tests.
- **`ErrResourceExists`** sentinel error in `interaction` package for duplicate resource registration.
- **Doc comments** for 4 exported symbols in `endpoint/endpoint_cache.go`: `EndpointCloser`, `NewEndpointCache`, `Update`, `Endpoints`.
- **`WithHooks` append semantics** (`interaction`): Updated doc comment to explicitly document that `WithHooks` accumulates (unlike `WithSessions`/`WithEvents`/`WithTools` which replace).

### Changed

- **`ServeStreamable` deprecated** (`interaction/mcp`): Marked with `Deprecated:` doc comment and migration example showing `NewStreamableHandler` usage.
- **`client.tmpl` gRPC upgrade** (`cmd/microgen`): Replaced deprecated `grpc.Dial` with `grpc.NewClient`, removed `grpc.WithTimeout` in favor of context deadlines.
- **`sdk.tmpl` gRPC upgrade** (`cmd/microgen`): Updated doc example from deprecated `grpc.WithInsecure()` to `grpc.WithTransportCredentials(insecure.NewCredentials())`.

### Removed

- **`MethodKindWebSocketSession`** from the microgen IR. WebSocket transport had no implementation — the constant and associated documentation references were dead code. The project focuses on MCP Streamable HTTP and gRPC as supported transports.
- **`MethodKindEventSource`** from the microgen IR. Unused constant with no references in parsers, generators, or templates.
- **WebSocket documentation references** cleaned across 14 files: `STABILITY.md` (removed fictitious `transport/ws` row), `README.md`, `README_zh.md`, `AI_FIRST_ROADMAP.md` (removed Phase 8), `REFACTOR_ROADMAP.md`, `RELEASE.md`, `MIGRATION.md`, `PROJECT_SNAPSHOT.md`, `PACKAGE_SURFACES.md`, `MICROGEN_COMPATIBILITY.md`.
- **Dead code**: commented-out `reimplementInterfaces` line in `transport/http/server/server.go`; `var _ = time.Second` hack and unused `"time"` import in `endpoint_generated_chain.tmpl`.
- **Phantom `common/` directory** reference removed from `examples/README.md` learning path.

## v1.6.0 - 2026-06-12

### Stable

- Promoted `interaction`, `interaction/mcp`, WebSocket transport, and generated interaction adapters from preview to stable scope.
- Implemented full MCP 2025-06-18 protocol support in `interaction/mcp`:
  - Resources: `resources/list`, `resources/read`, `resources/templates/list`
  - Prompts: `prompts/list`, `prompts/get` with argument rendering
  - Completions: `completion/complete` with `PromptCompleter` interface for prompt argument auto-completion
  - Logging: `logging/setLevel` with syslog severity levels (now rejects invalid levels)
  - Cursor-based pagination for all list methods
  - `StreamableHandler` for full Streamable HTTP transport (POST/GET/DELETE with SSE streams and session management)
  - Server-initiated notifications: `notifications/message`, `notifications/progress`, `notifications/resources/updated`, `notifications/resources/list_changed`, `notifications/prompts/list_changed`, `notifications/tools/list_changed`
  - MCP Sampling: `sampling/createMessage` with async request-response correlation via SSE
  - Session TTL with background cleanup via `StartCleanup()` / `StopCleanup()`
- Unified `ToolFunc` type with optional `Description` and `Schema` fields, replacing separate `ToolFunc` + `DescribedToolFunc`.
- `NewRuntime()` builder pattern with `WithSessions`, `WithEvents`, `WithTools`, `WithHooks`, `WithResources`, `WithPrompts` chaining, replacing variadic constructor.
- `NewHandler` is now an alias for `NewStreamableHandler` — both return the full Streamable HTTP handler.
- Added `interaction.ResourceProvider`, `interaction.PromptProvider`, and `interaction.PromptCompleter` interfaces with in-memory implementations.
- Added `interaction.MemoryResourceProvider` and `interaction.MemoryPromptProvider` for tests and small deployments.
- Added `examples/mcp_basic` (minimal MCP hello-world) and `examples/mcp_full` (tools, resources, prompts, completions, notifications).
- Fixed sampling race condition: `DeliverResponse` now guards against closed-channel panic when `UnregisterSession` races with response delivery.
- Error propagation in `handleResourcesList`, `handlePromptsList`, and `handleResourceTemplatesList` — errors are returned as JSON-RPC errors instead of silently returning empty results.
- Updated `microgen` templates to document MCP capabilities including `completion/complete` and server-initiated notifications.
- Fixed flaky gRPC deadline test error message assertion (`"deadline"` → `"DeadlineExceeded"`).

### Breaking Changes

- `ToolFunc` merged with `DescribedToolFunc` — use optional `Description` and `Schema` fields instead.
- `NewRuntime()` is now zero-argument with builder chaining — old variadic `NewRuntime(sessions, events, tools, hooks...)` is removed.
- `simple.go` Handler removed — `NewHandler` now aliases `NewStreamableHandler`.

### Documentation

- Added `OBSERVABILITY.md` with tracing, metrics, logging, request correlation, and OpenTelemetry integration guidance.
- Added `SECURITY_HARDENING.md` with authentication, authorization, request limits, audit, secrets, error response, and generated-project hardening guidance.
- Updated all documentation to remove preview status from `interaction`, `interaction/mcp`, WebSocket, and generated interaction adapters.
- Updated `README.md` and `README_zh.md` to document full MCP endpoint capabilities and stable scope.
- Added doc comments to ~30+ exported symbols across `interaction` and `interaction/mcp` packages.
- Updated `PACKAGE_SURFACES.md` to reflect the expanded `interaction` and `interaction/mcp` public API surface.
- Updated `DOCS_INDEX.md` interaction package description.

## v1.5.0 - 2026-05-17

### Stable

- Promoted documented core runtime and `microgen` generated-output behavior to the `v1.5.0` stable release scope.
- Promoted generated Proto gRPC streaming support for supported server-stream, client-stream, and bidirectional-stream RPC shapes to stable generated-output behavior.
- Documented the `v1.5.0` stable scope and compatibility boundary across release, stability, workflow, and migration docs.

### Stable (promoted from preview)

- Added transport-neutral `interaction.AuthorizationHook` and `interaction.AuditHook` helpers for tool-call policy and audit integration.
- Updated generated README output to explain that `/skill?format=mcp` is discovery metadata and executable AI sessions should use the `interaction` runtime and `interaction/mcp` adapter.
- Added an `examples/interaction_policy` example showing MCP-style tool calls with authorization and audit hooks.

## v1.5.0-preview.1 - 2026-05-17

### Preview

- Added IR method kinds for unary, server-stream, client-stream, bidirectional-stream, WebSocket-session, and event-source contract shapes.
- Added generated gRPC streaming preview support for Proto server-stream, client-stream, and bidirectional-stream RPCs.
- Added generated gRPC streaming SDK clients and success-path integration coverage for streaming flows.
- Added generated gRPC streaming integration coverage for error propagation and cancellation paths.
- Added generated streaming SDK guidance and coverage for synchronous callback backpressure behavior.
- Added generated streaming coverage for slow-consumer context deadline behavior.
- Added the `interaction` package for transport-neutral AI sessions, events, tool calls, runtime coordination, and hooks.
- Added `interaction/mcp`, a MCP-style JSON-RPC HTTP endpoint for listing and calling registered interaction runtime tools.

### Documentation

- Clarified that the current framework position is `v0.8 Beta`, not an industrial v1.0 release.
- Added release policy, migration policy, and the AI interaction roadmap for gRPC streaming, WebSocket, and AI-native server behavior.
- Updated roadmap status to make WebSocket optional and identify remaining gRPC streaming, AI runtime, and v1.0 hardening gaps.

### Planning

- Added `v0.9 AI Interaction Preview` as the next major milestone.
- Added `v1.0 Industrial` checklist for API stability, generated-output compatibility, security, observability, and release governance.
