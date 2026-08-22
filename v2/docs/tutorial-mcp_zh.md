# 教程：一个 MCP 服务器

[English](tutorial-mcp.md) | 简体中文

本教程通过 MCP Streamable HTTP 暴露一个工具，这是 AI 客户端使用的协议。完整代码
即可运行的 [examples/mcp_basic](../examples/README_zh.md) 示例。

## 1. 目标

在 `/mcp` 上提供一个拥有 `greet` 工具的 MCP 服务器，任何兼容 MCP 的客户端都能
调用它。

## 2. 注册工具

interaction 运行时拥有工具、资源、提示与会话。工具由名称、用于输入的 JSON Schema
和一个函数组成：

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

## 3. 提供 MCP 服务

`interaction/mcp` 通过 Streamable HTTP 暴露运行时：

```go
mux := http.NewServeMux()
mux.Handle("/mcp", mcp.NewHandler(rt))
```

## 4. 调用它

MCP 会话要求先完成初始化握手，然后才能调用工具：

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

## 5. 策略钩子

授权与审计挂载到运行时，而不是传输层：

```go
rt := interaction.NewRuntime().WithHooks(
	interaction.AuthorizationHook{Authorizer: allowTools("greet")},
	interaction.AuditHook{Sink: audits},
)
```

被拒绝的调用会在执行前被拒绝，永远不会到达审计接收器。完整的策略教程见
[examples/interaction_policy](../examples/README_zh.md)。

## 接下来去哪

- [interaction 指南](../interaction/README_zh.md)：资源、提示与 SSE
- [PRODUCTION](../PRODUCTION_zh.md)：长连接 MCP 流的写入超时
