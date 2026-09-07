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

`interaction/mcp` 子包为该运行时提供完整遵循 MCP 规范的 Streamable HTTP JSON-RPC 适配器。`MCP-Protocol-Version` 请求头选择协议版本：`2026-07-28`（无状态）或 `2025-06-18`（握手，缺少该头时也选择它）。

- `server/discover` —— 能力与支持的版本，`2026-07-28` 下提供
- `initialize` / `notifications/initialized` —— 仅 `2025-06-18`
- `ping`
- `tools/list`、`tools/call`
- `resources/list`、`resources/read`、`resources/templates/list`
- `prompts/list`、`prompts/get`
- `completion/complete`
- `logging/setLevel`
- SSE 流式传输（POST 与 GET），`2025-06-18` 下提供
- 服务端发起的采样（`sampling/createMessage`），`2025-06-18` 下提供
- 服务端发起的通知（日志、进度、list-changed）

在 `2026-07-28` 下，请求自带客户端身份（`params._meta`），在 `Mcp-Method` 与 `Mcp-Name` 头中重复自己的方法与目标，并且不需要任何会话：`IdentityFromContext` 与 `ClientCapabilityFromContext` 提供过去由会话状态承载的信息。列表与读取结果带 `ttlMs` 与 `cacheScope`，由 `StreamableHandler.ListCacheTTL` 和 `ListCacheScope` 配置。未显式指定运行时 `sessionId` 的工具调用在一个仅限本次调用的运行时会话中执行。

工具在调用中途需要调用方提供东西时，返回 `*interaction.InputRequired` 而不是结果。在 `2026-07-28` 下它变成 `resultType: "input_required"`，带 `inputRequests` 与不透明的 `requestState`；调用方作答后带 `inputResponses` 重发同一次调用，工具再跑一遍，此时 `interaction.InputAnswersFromContext` 返回答案以及它要求回传的状态。调用方没有声明能回答的问题会被 `-32021` 拒绝，而不是照问。在 `2025-06-18` 下同一个返回值是错误：那一版通过会话流询问，对应 `StreamableHandler.SendSamplingRequest`。

设置 `StreamableHandler.RequestStateKey` 即可对该状态做认证。配置了密钥时，被改动或已过期的 `requestState` 在工具看到它之前就被拒绝，`RequestStateTTL` 限制可重放的时间窗（默认五分钟）；没有密钥时状态按调用方返回的原样接受，因此工具必须像对待任何客户端输入一样校验它。服务同一地址的所有实例需要使用相同的密钥——无状态部署会把续跑的调用交给任意一个实例。

在 `2025-06-18` 下，每个 MCP 传输会话拥有一个交互运行时会话。未显式指定运行时 `sessionId` 的工具调用会复用该会话，而 DELETE 或 TTL 过期会将其结束并释放。这样可以在一段对话中保持钩子与事件身份的稳定，而不必为每次调用保留一个已关闭的运行时会话。

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
