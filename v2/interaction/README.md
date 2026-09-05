# interaction
English | [简体中文](README_zh.md)

Package for transport-neutral AI interaction runtime contracts.

Use it for:

- session lifecycle
- event streams
- tool registration and tool calls
- authorization, audit, or policy hooks

This package intentionally does not depend on HTTP, gRPC, WebSocket, MCP, or
`microgen`. Transports and generated project adapters should build on top of
these contracts instead of embedding transport-specific types into interaction
business logic.

Current entry points:

- `NewRuntime` — builder pattern with `WithSessions`, `WithEvents`, `WithTools`, `WithHooks`, `WithResources`, `WithPrompts`, `WithLogger`
- `Runtime.ListTools`
- `Runtime.ReleaseSession` for transport-owned session cleanup
- `NewMemorySessionStore`
- `NewMemoryEventSink`
- `NewMemoryToolRegistry`
- `ToolFunc` — unified tool adapter with optional `Description` and `Schema` fields
- `HookFuncs`
- `AuthorizationHook`
- `AuditHook`
- `mcp.NewHandler` — Streamable HTTP MCP transport (alias for `mcp.NewStreamableHandler`)

The `interaction/mcp` subpackage provides a full MCP-compliant Streamable HTTP
JSON-RPC adapter for the runtime:

- `initialize` / `notifications/initialized`
- `ping`
- `tools/list`, `tools/call`
- `resources/list`, `resources/read`, `resources/templates/list`
- `prompts/list`, `prompts/get`
- `completion/complete`
- `logging/setLevel`
- SSE streaming (POST and GET)
- Server-initiated sampling (`sampling/createMessage`)
- Server-initiated notifications (log, progress, list-changed)

Each MCP transport session owns one interaction runtime session. Tool calls
without an explicit runtime `sessionId` reuse it, and DELETE or TTL expiry ends
and releases it. This keeps hook and event identity stable across a conversation
without retaining one closed runtime session per call.

`interaction/mcp` is the generated AI protocol surface. It discovers and
executes registered runtime tools inside interaction sessions; the framework no
longer generates a parallel `/skill` discovery endpoint.

Policy hooks:

- `AuthorizationHook` runs before a tool call and returns `ErrUnauthorized`
  when the configured `Authorizer` denies access.
- `AuditHook` records before/after tool-call audit records through an
  application-provided `AuditSink`.

These hooks are intentionally transport-neutral. HTTP, gRPC streaming,
WebSocket, and MCP adapters should pass subject and request metadata into the
runtime rather than implementing separate policy stacks per transport.

Reporting:

- `Runtime.WithLogger(logger)` reports every tool call through the caller's
  `*slog.Logger` with the request path's own attributes: `tool`, `session`,
  `duration`, `success`, `error`, plus `trace_id` and `request_id` when the
  call's context carries them. A tool call therefore joins the request that
  carried it instead of being a separate world.
- A failed or hook-rejected call is reported at Error level. Nothing else
  reports it: a tool failure travels back to the model as a result, not as a
  transport error.
- A nil logger reports nothing, which is the default.

Implementation notes:

- In-memory implementations are suitable for tests, demos, and local experiments.
  Production deployments should provide durable implementations.
- `NewMemoryEventSink` retains at most 10,000 events per session by default;
  use `NewMemoryEventSinkWithLimit` for a smaller bound or construct
  `MemoryEventSink` directly for an explicitly unbounded test sink.
- This is not a WebSocket runtime; WebSocket should remain an adapter decision.
