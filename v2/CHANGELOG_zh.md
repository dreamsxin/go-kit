# 变更日志

[English](CHANGELOG.md) | 简体中文

## [2.8.1] - 发布候选

对全部运行时 package 做了一轮契约审计。下列每一条都是代码与它自身文档化契约不
一致的地方。版本号取 patch 是因为这些改动是纠正而非新特性集；其中若干条确实改变
了可观测行为，已排在最前。

### 修复 - 行为

- `security`：匿名或没有身份的 subject 不再满足 `RequireAuthenticated` 与
  `RequireRole`。`Middleware` 也不再丢弃只带 roles 或 claims 的 subject——此前这
  会让一个已认证的调用者拿到 401。
- `interaction`：nil 的 `AuthorizerFunc` 改为拒绝而非放行。它只会以 typed nil 的
  形式落到 `Authorizer` 字段上，而 `AuthorizationHook` 自己的 nil 检查看不到这种
  情况，所以这原本是一条 fail-open 路径。
- `interaction`：当某个 hook 拒绝调用时，`Runtime.CallTool` 会对已经放行的 hook
  执行 `AfterToolCall`，使 `AuditHook` 记录这次拒绝。此前审计 sink 完全看不到拒绝。
- `kit`：panic 的健康检查被报告为不健康的检查，而不是让进程退出。检查跑在各自的
  goroutine 中，而探针是未认证请求。
- HTTP 错误编码器：实现了 `transporthttp.PublicMessager` 的错误现在自行决定它的
  消息，包括空消息——空表示"这里没有任何内容是给客户端的"。`apperror.WrapCause`
  因此兑现了它文档中"cause 保持内部"的承诺；对于不在该契约内的错误，500 以下仍
  回落到 `err.Error()`。
- `TextErrorEncoder` 通过与 JSON、纯文本编码器相同的规则解析消息。此前它在 500
  也会应用 `PublicMessager`，并且把 499 命名为 "HTTP error" 而不是
  "Client Closed Request"。
- `integrations/grpc`：`StatusError` 给出的公开消息只包含上游 code，不包含上游
  description，与 `client.HTTPStatusError` 保持一致。
- 超限请求体在 JSON 与 raw codec 两条路径上都归类为 413，与
  `ParseMultipartForm` 以及 `RawBodyCodecWithMaxBytes` 的文档一致。此前经
  `JSONDecodeError` 是 400，直接抛出时是 500。
- `endpoint.TimeoutMiddleware` 与 `sd/retry`：非正的 timeout 表示不施加 deadline，
  而不是把一个已过期的 context 交给调用。
- `endpoint.BackpressureMiddleware`、`endpoint.InFlightMiddleware` 与
  `sd/retry.Retry`：非正的上限被 clamp 到 1，与 `BulkheadMiddleware` 原有行为一
  致。此前上限为 0 会拒绝每一个请求。
- `sd`：包装型 strategy 或 balancer 在丢弃一次成功的内层 `Pick` 时会释放它的
  `Done`。`selector.Filtered`、`feedback`、`selector` 与 `balancer` 各自都在拒绝
  分支上泄漏了一个 in-flight 预留。
- 成功路径的响应编码器会忽略超出 100-999 的 `StatusCoder` 值，而不是把它交给
  `WriteHeader`——后者会 panic。
- `interaction`：缺少 `Sessions`、`Events` 或 `Tools` 的 `Runtime` 报告
  `ErrRuntimeNotConfigured`，而不是在第一次调用时 panic；`StartSession` 在无法发
  出启动事件时会释放它已创建的 session。
- `observability/otel`、`observability/slog`、`integrations/zap`：panic 的
  endpoint 被报告为 panic。此前 otel span 以 Unset 结束且不记录错误，zap 记录
  "endpoint call succeeded"，slog 什么都不记录。

### 新增

- `endpoint.FailerMiddleware` 与 `endpoint.ResponseError`，以及
  `Builder.WithFailer`。`Failer` 响应到达传输层时错误为 nil，因此指标、熔断器和
  重试都把它算作成功。安装在最内层时，该中间件先把它转换出来，且不改变客户端可
  观测到的任何东西。
- `security.Subject.Authenticated`：区分"subject 存在于 context 中"与"principal
  已被建立"的检查。
- `sd.Release`：包装型 strategy 用来交回自己无法使用的 `Done` 的辅助函数。
- `interaction.ErrRuntimeNotConfigured`。

### 修复 - 文档

doc comment 在本仓库属于已评审的 API 快照，因此单独记录而不并入上面的条目。

- `JSONErrorEncoder` 没有 doc comment；描述它的那段被挂到了
  `JSONErrorEncoderWithKindMapper` 上。`RawBodyCodec` 的注释从句子中间开始。
- `sd/endpointer` 声称 round-robin 与 random balancer 接受较窄的 `Endpointer`，
  `sd/client.NewEndpoint` 声称它组合了一个 `Endpointer`。该模块中的一切都返回
  `InstanceEndpointer`。
- `SubjectFromContext` 声称对匿名调用者返回 false。

## [2.8.0] - 2026-09-04

这是当前 `go-kit/v2` 产品线的首次公开发布。仓库以单一 Go module 发布：

```text
github.com/dreamsxin/go-kit/v2
```

运行时 package、传输适配器、服务发现 provider、可观测性适配器和
`cmd/microgen` 统一由同一个 module 和根 tag 进行版本管理。`examples`、
`tools` 与 `tools/contractcheck` 仍是仓库内部 workspace module。

### 包含内容

- `Service -> Endpoint -> Transport` 服务架构。
- 类型化 HTTP JSON 处理器、自定义编解码、SSE、gRPC 适配器以及 Go/TypeScript 客户端。
- 传输无关的应用错误，HTTP 与 gRPC 使用一致的错误映射。
- endpoint 中间件：校验、超时、追踪、指标、恢复、限流、熔断、降级、背压、
  舱壁和重试。
- 服务发现快照、端点缓存、负载均衡、重试、健康检查、被动摘除、反馈以及长连接统计。
- 交互运行时：会话、工具、资源、提示、授权、审计钩子和 MCP Streamable HTTP。
- 配置、OpenAPI、JSON Schema、数据库脚手架和确定性的 `microgen` 扩展流程。
- 明确的生命周期所有权、有界停机、请求关联和 package 级依赖门禁。

### 发布契约

- 唯一发布 tag 是根 `v2.8.0` tag。
- 历史 v2 module tag（根标签和嵌套标签）已删除。
- 未来版本使用一个版本和一个根 tag。
- 行为与 API 变化记录在本文件中；本次首次公开基线不维护单独的迁移历史。
