# Changelog

All notable user-visible changes should be recorded here.

This project has not reached v1.0. Until then, entries should clearly distinguish stable behavior from preview behavior.

## Unreleased

### Stable

- Implemented full MCP 2025-06-18 protocol support in `interaction/mcp`:
  - Resources: `resources/list`, `resources/read`, `resources/templates/list`
  - Prompts: `prompts/list`, `prompts/get` with argument rendering
  - Completions: `completion/complete` with `PromptCompleter` interface for prompt argument auto-completion
  - Logging: `logging/setLevel` with syslog severity levels
  - Cursor-based pagination for all list methods
  - `StreamableHandler` for full Streamable HTTP transport (POST/GET/DELETE with SSE streams and session management)
  - Server-initiated notifications: `notifications/message`, `notifications/progress`, `notifications/resources/updated`, `notifications/resources/list_changed`, `notifications/prompts/list_changed`, `notifications/tools/list_changed`
  - MCP Sampling: `sampling/createMessage` with async request-response correlation via SSE
- Added `interaction.ResourceProvider`, `interaction.PromptProvider`, and `interaction.PromptCompleter` interfaces with in-memory implementations.
- Added `interaction.MemoryResourceProvider` and `interaction.MemoryPromptProvider` for tests and small deployments.
- Added `examples/mcp_full` demonstrating tools, resources, prompts, completions, and server-initiated notifications via Streamable HTTP transport.
- Updated `microgen` templates to document MCP capabilities including `completion/complete` and server-initiated notifications.
- Fixed flaky gRPC deadline test error message assertion (`"deadline"` → `"DeadlineExceeded"`).

### Documentation

- Added `OBSERVABILITY.md` with tracing, metrics, logging, request correlation, and OpenTelemetry integration guidance.
- Added `SECURITY_HARDENING.md` with authentication, authorization, request limits, audit, secrets, error response, and generated-project hardening guidance.
- Updated `README.md` and `README_zh.md` to document the full MCP endpoint capabilities including Streamable HTTP transport, sampling, completions, and notifications.
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
