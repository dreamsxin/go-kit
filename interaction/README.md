# interaction

Preview package for transport-neutral AI interaction runtime contracts.

Use it when experimenting with:

- session lifecycle
- event streams
- tool registration and tool calls
- authorization, audit, or policy hooks

This package intentionally does not depend on HTTP, gRPC, WebSocket, MCP, or
`microgen`. Transports and generated project adapters should build on top of
these contracts instead of embedding transport-specific types into interaction
business logic.

Current entry points:

- `NewRuntime`
- `Runtime.ListTools`
- `NewMemorySessionStore`
- `NewMemoryEventSink`
- `NewMemoryToolRegistry`
- `ToolFunc`
- `HookFuncs`
- `AuthorizationHook`
- `AuditHook`
- `mcp.NewHandler`

The `interaction/mcp` subpackage provides a preview MCP-style JSON-RPC HTTP
adapter for the runtime:

- `initialize`
- `tools/list`
- `tools/call`

It is separate from generated `/skill?format=mcp` output. `/skill?format=mcp`
describes available tools, while `interaction/mcp` can execute registered
runtime tools inside interaction sessions.

Policy hooks:

- `AuthorizationHook` runs before a tool call and returns `ErrUnauthorized`
  when the configured `Authorizer` denies access.
- `AuditHook` records before/after tool-call audit records through an
  application-provided `AuditSink`.

These hooks are intentionally transport-neutral. HTTP, gRPC streaming,
WebSocket, and MCP adapters should pass subject and request metadata into the
runtime rather than implementing separate policy stacks per transport.

Preview limits:

- In-memory implementations are for tests, demos, and local experiments.
- Event names and runtime shape may still change before v1.0.
- This is not a WebSocket runtime; WebSocket should remain an adapter decision.
- The MCP adapter is a preview endpoint, not a full MCP transport commitment.
