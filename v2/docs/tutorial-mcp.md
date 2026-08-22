# Tutorial: An MCP Server

English | [简体中文](tutorial-mcp_zh.md)

This tutorial exposes a tool over MCP Streamable HTTP, the protocol AI clients
speak. The complete code is the runnable [examples/mcp_basic](../examples/README.md)
example.

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
		name, _ := call.Input["name"].(string)
		return interaction.ToolResult{Output: map[string]any{"greeting": "Hello, " + name + "!"}}, nil
	},
})
```

## 3. Serve MCP

`interaction/mcp` exposes the runtime over Streamable HTTP:

```go
mux := http.NewServeMux()
mux.Handle("/mcp", mcp.NewHandler(rt))
```

## 4. Call it

MCP sessions require the initialize handshake before tools are called:

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

## 5. Policy hooks

Authorization and audit attach to the runtime, not the transport:

```go
rt := interaction.NewRuntime().WithHooks(
	interaction.AuthorizationHook{Authorizer: allowTools("greet")},
	interaction.AuditHook{Sink: audits},
)
```

Denied calls are rejected before execution and never reach the audit sink. The
full policy walkthrough is [examples/interaction_policy](../examples/README.md).

## Where to go next

- [interaction guide](../interaction/README.md): resources, prompts, and SSE
- [PRODUCTION](../PRODUCTION.md): write timeouts for long-lived MCP streams
