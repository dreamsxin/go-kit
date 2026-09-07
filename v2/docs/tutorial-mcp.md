# Tutorial: An MCP Server

English | [简体中文](tutorial-mcp_zh.md)

This tutorial exposes a tool over MCP Streamable HTTP, the protocol AI clients
speak. The complete code is the runnable
[examples/mcp_basic/main.go](../examples/mcp_basic/main.go) example.

## 1. The goal

An MCP server on `/mcp` with one tool, `greet`, callable by any
MCP-compatible client.

## 2. Register the tool

The interaction runtime owns tools, resources, prompts, and sessions. A tool is
a name, a JSON Schema for its input, and a function:

```go
rt := interaction.NewRuntime()

_ = rt.RegisterTool(interaction.ToolFunc{
	ToolName:    "greet",
	Description: "Returns a greeting for the given name.",
	Schema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string", "description": "Who to greet"},
		},
	},
	Fn: func(_ context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
		args, _ := call.Input.(map[string]any)
		name, _ := args["name"].(string)
		return interaction.ToolResult{Output: map[string]any{"greeting": "Hello, " + name + "!"}}, nil
	},
})
```

## 3. Serve MCP

`interaction/mcp` exposes the runtime over Streamable HTTP. The shortest form
builds the mux and starts the session reaper for you:

```go
log.Fatal(mcp.ListenAndServe(":8080", rt))
```

To mount MCP beside your own routes, take the handler instead:

```go
mux := http.NewServeMux()
mux.Handle("/mcp", mcp.NewHandler(rt)) // alias for NewStreamableHandler
```

Register the pattern without a method verb. The handler dispatches POST, GET,
and DELETE itself and answers anything else with 405 and an `Allow` header;
registering only `POST /mcp` would break session termination.

For a service that owns the HTTP lifecycle, use `Serve` so cancellation drains
the HTTP server and releases MCP sessions:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
if err := mcp.Serve(ctx, ":8080", rt); err != nil {
	log.Fatal(err)
}
```

When mounting the handler yourself, call `StartCleanup` after setting
`SessionTTL`, and call `Shutdown(ctx)` before the process exits. Shut down the
owning `http.Server` first when in-flight tool calls must be cancelled. Configure
`MaxSessions`, `MaxPostBodyBytes`, and `AllowedOrigins` for the deployment.

## 4. Call it

On `2026-07-28` a request stands alone. Name the revision, the method and the
target in headers, and call the tool:

```bash
curl -X POST http://localhost:8080/mcp \
  -H 'MCP-Protocol-Version: 2026-07-28' \
  -H 'Mcp-Method: tools/call' \
  -H 'Mcp-Name: greet' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{
        "name":"greet","arguments":{"name":"World"},
        "_meta":{"io.modelcontextprotocol/clientInfo":{"name":"curl","version":"1.0"}}}}'

# Capabilities, when you want them before committing to anything
curl -X POST http://localhost:8080/mcp \
  -H 'MCP-Protocol-Version: 2026-07-28' \
  -H 'Mcp-Method: server/discover' \
  -d '{"jsonrpc":"2.0","id":2,"method":"server/discover"}'
```

`Mcp-Method` must repeat the JSON-RPC method and `Mcp-Name` the target it
addresses, so a gateway can route, meter and authorize on headers alone; a
request whose headers disagree with its body is answered 400. `tools/list`,
`prompts/list`, `resources/list`, `resources/templates/list` and `resources/read`
carry `ttlMs` and `cacheScope`, which `StreamableHandler.ListCacheTTL` and
`ListCacheScope` configure. There is no session to open or delete: GET and DELETE
are answered 405 on this revision.

A client on `2025-06-18` — which is also what an absent `MCP-Protocol-Version`
header selects — still does the handshake:
```bash
# Initialize a session (note the Mcp-Session-Id response header)
curl -i -X POST http://localhost:8080/mcp \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}'

# Complete the lifecycle
curl -X POST http://localhost:8080/mcp \
  -H 'Mcp-Session-Id: <sid>' \
  -d '{"jsonrpc":"2.0","method":"notifications/initialized"}'

# Call the tool
curl -X POST http://localhost:8080/mcp \
  -H 'Mcp-Session-Id: <sid>' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"greet","arguments":{"name":"World"}}}'
```

## 5. Ask the caller something

A tool that needs a confirmation or a missing field returns a question instead of
a result. It runs again when the answer arrives, so write it as a guard:

```go
Fn: func(ctx context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
	answers, state := interaction.InputAnswersFromContext(ctx)
	if answers["confirm"] != true {
		return interaction.ToolResult{}, &interaction.InputRequired{
			Requests: map[string]interaction.InputRequest{
				"confirm": {Kind: "elicitation", Params: map[string]any{"message": "Delete 42 rows?"}},
			},
			State: map[string]any{"rows": 42},
		}
	}
	return interaction.ToolResult{Output: deleted(state)}, nil
}
```

On `2026-07-28` the call answers `resultType: "input_required"` with
`inputRequests` and an opaque `requestState`; the caller collects the answers and
repeats the call with `inputResponses` and that same state. Expect the tool to run
from the top each round. A question the caller never declared it can answer is
refused with `-32021` rather than asked, so declare `elicitation` in `_meta` when
the client can render one.

Set `StreamableHandler.RequestStateKey` so the state comes back the way it left:
with a key, an edited or expired `requestState` is refused before the tool sees
it, and `RequestStateTTL` bounds replay. Without a key the state is whatever the
caller returns, which is fine for a confirmation and not fine for a decision the
tool means to trust. Every instance behind the same address needs the same key.

On `2025-06-18` the same return value is an error — that revision asks over the
session stream instead, with `StreamableHandler.SendSamplingRequest`.

## 6. Policy hooks

Authorization and audit attach to the runtime, not the transport:

```go
rt := interaction.NewRuntime().WithHooks(
	interaction.AuthorizationHook{Authorizer: allowTools("greet")},
	interaction.AuditHook{Sink: audits},
)
```

Denied calls are rejected before execution and never reach the audit sink --
because `BeforeToolCall` hooks run in the order given and the first error returns
immediately. Put `AuthorizationHook` first if you want that; swap the two and
denials become audited instead. The full policy walkthrough is
[examples/interaction_policy/main.go](../examples/interaction_policy/main.go).

## Where to go next

- [interaction guide](../interaction/README.md): resources, prompts, and SSE
- [PRODUCTION](../PRODUCTION.md): write timeouts for long-lived MCP streams
