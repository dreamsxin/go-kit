# interaction

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

- `NewRuntime` — builder pattern with `WithSessions`, `WithEvents`, `WithTools`, `WithHooks`, `WithResources`, `WithPrompts`
- `Runtime.ListTools`
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

Implementation notes:

- In-memory implementations are suitable for tests, demos, and local experiments.
  Production deployments should provide durable implementations.
- This is not a WebSocket runtime; WebSocket should remain an adapter decision.
