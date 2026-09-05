# interaction（交互）

[English](README.md) | 简体中文

提供传输层无关的 AI 交互运行时契约的包。

适用场景：

- 会话生命周期
- 事件流
- 工具注册与工具调用
- 授权、审计或策略钩子

该包有意不依赖 HTTP、gRPC、WebSocket、MCP 或 `microgen`。各种传输层和生成的项目适配器应构建于这些契约之上，而不是把传输层特有的类型嵌入交互业务逻辑。

当前入口点：

- `NewRuntime` — 构建器模式，提供 `WithSessions`、`WithEvents`、`WithTools`、`WithHooks`、`WithResources`、`WithPrompts`、`WithLogger`
- `Runtime.ListTools`
- `Runtime.ReleaseSession`，用于由传输层负责的会话清理
- `NewMemorySessionStore`
- `NewMemoryEventSink`
- `NewMemoryToolRegistry`
- `ToolFunc` — 统一的工具适配器，带有可选的 `Description` 和 `Schema` 字段
- `HookFuncs`
- `AuthorizationHook`
- `AuditHook`
- `mcp.NewHandler` — Streamable HTTP MCP 传输（`mcp.NewStreamableHandler` 的别名）

`interaction/mcp` 子包为该运行时提供完整遵循 MCP 规范的 Streamable HTTP JSON-RPC 适配器：

- `initialize` / `notifications/initialized`
- `ping`
- `tools/list`、`tools/call`
- `resources/list`、`resources/read`、`resources/templates/list`
- `prompts/list`、`prompts/get`
- `completion/complete`
- `logging/setLevel`
- SSE 流式传输（POST 与 GET）
- 服务端发起的采样（`sampling/createMessage`）
- 服务端发起的通知（日志、进度、list-changed）

每个 MCP 传输会话拥有一个交互运行时会话。未显式指定运行时 `sessionId` 的工具调用会复用该会话，而 DELETE 或 TTL 过期会将其结束并释放。这样可以在一段对话中保持钩子与事件身份的稳定，而不必为每次调用保留一个已关闭的运行时会话。

`interaction/mcp` 是生成的 AI 协议接口层。它在交互会话内部发现并执行已注册的运行时工具；框架不再额外生成一个平行的 `/skill` 发现端点。

策略钩子：

- `AuthorizationHook` 在工具调用之前运行，当配置的 `Authorizer` 拒绝访问时返回 `ErrUnauthorized`。
- `AuditHook` 通过应用提供的 `AuditSink` 记录工具调用前后的审计记录。

这些钩子有意保持传输层无关。HTTP、gRPC 流式、WebSocket 和 MCP 适配器应把主体（subject）和请求元数据传入运行时，而不是为每种传输层分别实现独立的策略栈。

上报：

- `Runtime.WithLogger(logger)` 用调用方的 `*slog.Logger` 上报每一次工具调用，字段与请求
  路径一致：`tool`、`session`、`duration`、`success`、`error`，以及调用 context 携带时的
  `trace_id` 与 `request_id`。于是一次工具调用能和承载它的请求对上，而不是另一个世界。
- 失败或被钩子拒绝的调用记在 Error 级。别处不会报告它：工具失败是以结果的形式回到模型，
  而不是一个传输错误。
- logger 为 nil 时什么都不记，这也是默认值。

实现说明：

- 内存实现适合用于测试、演示和本地实验。生产部署应提供可持久化的实现。
- `NewMemoryEventSink` 默认每个会话最多保留 10,000 条事件；可用
  `NewMemoryEventSinkWithLimit` 设置更小的上限，或直接构造 `MemoryEventSink`
  作为明确无界的测试 sink。
- 这不是一个 WebSocket 运行时；WebSocket 应仍然由适配器层面来决策。
