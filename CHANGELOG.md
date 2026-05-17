# Changelog

All notable user-visible changes should be recorded here.

This project has not reached v1.0. Until then, entries should clearly distinguish stable behavior from preview behavior.

## Unreleased

### Preview

- Added transport-neutral `interaction.AuthorizationHook` and `interaction.AuditHook` preview helpers for tool-call policy and audit integration.
- Updated generated README output to explain that `/skill?format=mcp` is discovery metadata and executable AI sessions should use the `interaction` runtime and `interaction/mcp` preview adapter.
- Added an `examples/interaction_policy` preview showing MCP-style tool calls with authorization and audit hooks.

## v1.5.0-preview.1 - 2026-05-17

### Preview

- Added IR method kinds for unary, server-stream, client-stream, bidirectional-stream, WebSocket-session, and event-source contract shapes.
- Added generated gRPC streaming preview support for Proto server-stream, client-stream, and bidirectional-stream RPCs.
- Added generated gRPC streaming SDK clients and success-path integration coverage for streaming flows.
- Added generated gRPC streaming integration coverage for error propagation and cancellation paths.
- Added generated streaming SDK guidance and coverage for synchronous callback backpressure behavior.
- Added generated streaming coverage for slow-consumer context deadline behavior.
- Added the `interaction` preview package for transport-neutral AI sessions, events, tool calls, runtime coordination, and hooks.
- Added `interaction/mcp`, a preview MCP-style JSON-RPC HTTP endpoint for listing and calling registered interaction runtime tools.

### Documentation

- Clarified that the current framework position is `v0.8 Beta`, not an industrial v1.0 release.
- Added release policy, migration policy, and the AI interaction roadmap for gRPC streaming, WebSocket, and AI-native server behavior.
- Updated roadmap status to make WebSocket optional and identify remaining gRPC streaming, AI runtime, and v1.0 hardening gaps.

### Planning

- Added `v0.9 AI Interaction Preview` as the next major milestone.
- Added `v1.0 Industrial` checklist for API stability, generated-output compatibility, security, observability, and release governance.
