# 教程：一个 MCP 服务器

[English](tutorial-mcp.md) | 简体中文

本教程通过 MCP Streamable HTTP 暴露一个工具，这是 AI 客户端使用的协议。完整代码
即可运行的 [examples/mcp_basic/main.go](../examples/mcp_basic/main.go) 示例。

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
		args, _ := call.Input.(map[string]any)
		name, _ := args["name"].(string)
		return interaction.ToolResult{Output: map[string]any{"greeting": "Hello, " + name + "!"}}, nil
	},
})
```

## 3. 提供 MCP 服务

`interaction/mcp` 通过 Streamable HTTP 暴露运行时。最短的写法会替你建好 mux 并
启动会话清理：

```go
log.Fatal(mcp.ListenAndServe(":8080", rt))
```

若要把 MCP 挂在你自己的路由旁边，取 handler 即可：

```go
mux := http.NewServeMux()
mux.Handle("/mcp", mcp.NewHandler(rt)) // NewStreamableHandler 的别名
```

注册模式时不要带方法动词。handler 自己分发 POST、GET 与 DELETE，其他方法返回 405
并带 `Allow` 头；只注册 `POST /mcp` 会导致会话终止无法工作。

如果服务自己管理 HTTP 生命周期，使用 `Serve` 可以在取消时优雅停止 HTTP 服务并释放
MCP 会话：

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
if err := mcp.Serve(ctx, ":8080", rt); err != nil {
	log.Fatal(err)
}
```

手动挂载 handler 时，设置 `SessionTTL` 后调用 `StartCleanup`，进程退出前调用
`Shutdown(ctx)`。如果需要取消进行中的工具调用，应先关闭所属的 `http.Server`。
生产环境应配置 `MaxSessions`、`MaxPostBodyBytes` 和 `AllowedOrigins`。

## 4. 调用它

在 `2026-07-28` 下，一个请求自成一体。用头声明协议版本、方法和目标，然后调用工具：

```bash
curl -X POST http://localhost:8080/mcp \
  -H 'MCP-Protocol-Version: 2026-07-28' \
  -H 'Mcp-Method: tools/call' \
  -H 'Mcp-Name: greet' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{
        "name":"greet","arguments":{"name":"World"},
        "_meta":{"io.modelcontextprotocol/clientInfo":{"name":"curl","version":"1.0"}}}}'

# 需要先了解服务端能力时
curl -X POST http://localhost:8080/mcp \
  -H 'MCP-Protocol-Version: 2026-07-28' \
  -H 'Mcp-Method: server/discover' \
  -d '{"jsonrpc":"2.0","id":2,"method":"server/discover"}'
```

`Mcp-Method` 必须与 JSON-RPC 方法一致，`Mcp-Name` 必须与它寻址的目标一致，网关因此
只看头就能路由、计量和鉴权；头与请求体不一致的请求返回 400。`tools/list`、
`prompts/list`、`resources/list`、`resources/templates/list` 和 `resources/read`
会带上 `ttlMs` 与 `cacheScope`，由 `StreamableHandler.ListCacheTTL` 和
`ListCacheScope` 配置。这一版本没有会话可开或可删，GET 与 DELETE 返回 405。

`2025-06-18` 的客户端（缺少 `MCP-Protocol-Version` 头时也选择这一版本）仍走握手流程：

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

## 5. 向调用方提问

需要确认或缺少字段的工具，返回一个问题而不是结果。答案到达时它会再跑一遍，所以把它
写成一道门禁：

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

在 `2026-07-28` 下，这次调用返回 `resultType: "input_required"`，带 `inputRequests`
与不透明的 `requestState`；调用方收集答案后，带 `inputResponses` 与同一个状态重发调用。
预期工具每一轮都从头跑。调用方没有声明能回答的问题会被 `-32021` 拒绝而不是照问，所以
客户端能渲染表单时要在 `_meta` 中声明 `elicitation`。

设置 `StreamableHandler.RequestStateKey`，让状态原样回来：配置了密钥时，被改动或已过期
的 `requestState` 在工具看到它之前就被拒绝，`RequestStateTTL` 限制重放窗口。没有密钥时
状态就是调用方返回的任意内容——用于一次确认没问题，用于工具打算信任的决策则不行。服务
同一地址的所有实例需要使用相同的密钥。

在 `2025-06-18` 下同一个返回值是错误——那一版改为通过会话流询问，对应
`StreamableHandler.SendSamplingRequest`。

## 6. 策略钩子

授权与审计挂载到运行时，而不是传输层：

```go
rt := interaction.NewRuntime().WithHooks(
	interaction.AuthorizationHook{Authorizer: allowTools("greet")},
	interaction.AuditHook{Sink: audits},
)
```

被拒绝的调用会在执行前被拒绝，永远不会到达审计接收器——因为 `BeforeToolCall`
钩子按给定顺序运行，第一个返回错误就立即返回。想要这个效果，就把
`AuthorizationHook` 放在前面；两者调换顺序，被拒绝的调用就会被审计。完整的策略
教程见 [examples/interaction_policy/main.go](../examples/interaction_policy/main.go)。

那个钩子决定工具调用。客户端能问的其他一切——列出工具、读取资源、渲染提示——在
传输层决定，因为 HTTP 凭证在那里：

```go
h := mcp.NewStreamableHandler(rt)
h.Authorizer = mcp.MethodAuthorizerFunc(func(ctx context.Context, req mcp.MethodRequest) error {
	if req.Method == "server/discover" || req.Method == "ping" {
		return nil // 允许客户端在登录之前先弄清这台服务器是什么
	}
	subject, _ := security.SubjectFromContext(ctx)
	if !subject.Authenticated() {
		return fmt.Errorf("%w: %s needs a signed-in caller", interaction.ErrUnauthorized, req.Method)
	}
	if req.Method == "tools/call" && req.Target == "deploy" && !subject.HasRole("operator") {
		return fmt.Errorf("%w: deploy is operator-only", interaction.ErrUnauthorized)
	}
	return nil
})
```

主体来自 context，由认证了这个请求的那一层放进去——`security.Middleware`，或者你
自己调用 `security.WithSubject` 的 HTTP 中间件。`mcp` 对身份如何表示没有意见，也
绝不会从请求 body 里读出一个主体：那里的 `subject` 字段是调用方对自己的一项主张。
`req.Header` 也在，供你的策略读取真正据以判断的租户或凭证。

把 `Authorizer` 留成 nil，所有已实现的方法都会被服务。框架对“谁可以列出你的工具”
没有意见，因为它不可能有。

## 接下来去哪

- [interaction 指南](../interaction/README_zh.md)：资源、提示与 SSE
- [PRODUCTION](../PRODUCTION_zh.md)：长连接 MCP 流的写入超时
