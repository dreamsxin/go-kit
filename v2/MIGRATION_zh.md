# 升级说明

[English](MIGRATION.md) | 简体中文

go-kit v2 遵循语义化版本。本页记录当前版本所需的升级动作；每个版本的
完整变更列表见 [CHANGELOG.md](CHANGELOG_zh.md)。

## 从旧版 go-kit（v0/v1 风格）迁移

基于旧版 go-kit 风格（`github.com/go-kit/kit`：每路由
`transport/http.NewServer` + 解码/编码函数对、`endpoint.Set` 结构、
`NewGRPCServer`/`NewGRPCClient` 装配块）的代码库，构造映射如下：

| 旧版构造 | go-kit v2 等价物 |
| --- | --- |
| 每路由 `httptransport.NewServer(ep, decode, encode, opts...)` | `kit.HandleJSONTyped` / `kit.HandleJSONWithMiddleware`；自定义编解码用 `server.NewServer` |
| 每路由解码/编码函数对 | 类型化处理器无需编解码；自定义线上格式用 `server.RawBodyCodec` |
| `endpoint.Set` 结构装配 | 在 `kit.HTTP` 组件上注册路由；microgen 生成的 `endpoints.go` |
| `NewGRPCServer` / grpc handler 结构 | `integrations/grpc` 服务端 + `transport.Binding`（一个端点同时服务 HTTP 与 gRPC） |
| `NewGRPCClient` 端点块 | `integrations/grpc/client`；`sd/client` 一次装配发现、均衡与重试 |
| `jwt.HTTPToContext()` / `jwt.GRPCToContext()` | `security` 主体契约 + 应用在协议边界自有的凭据提取 |
| `opentracing.HTTPToContext` / `TraceClient` | `endpoint.TracingMiddleware`（W3C）或 `observability/otel` |
| `ratelimit.NewErroringLimiter` | 内置 `endpoint.RateLimitMiddleware` |
| `switch err.Error() {...}` 状态码映射 | `apperror` 分类；各传输映射种类；自定义状态用 `JSONErrorEncoderWithKindMapper` |
| 响应中的 `Err string` 字段（错误字符串化传输） | 直接返回错误；proto 消息必须携带失败时用 `endpoint.Failer` |
| 手写 proto↔领域映射器 | 生成传输（`microgen` 从 `.proto` 生成） |

旧版风格易犯、而 v2 从设计上消除的坑：

- **中间件静默漂移**：每方法中间件靠复制粘贴或注释开关应用，会让路由在毫无信号的情况下丢失限流或追踪。v2 中间件在组件级一次声明或按路由显式声明，链可经 `endpoint.Builder.Describe()` 自省。
- **错误分类丢失**：字符串化的错误跨线丢失种类；`apperror` 种类在 HTTP 与 gRPC 间保留。
- **映射器漂移**：手写 proto↔领域转换器随时间与 schema 漂移；生成的编解码与契约保持同步。

推荐迁移顺序：逐个服务迁移；先从类型化 JSON 处理器（`kit`）入手，再把错误字符串约定替换为 `apperror`，然后把重复的 HTTP/gRPC 装配收敛到 `transport.Binding`，最后让 `microgen` 接管再生成的传输。

## 从 v2.6.0 升级（未发布）

三处源码改动：

1. `endpoint.Metrics` 的计数字段不再导出。改为通过 `Snapshot()` 读取，字段名
   保持不变：

   ```go
   // 之前
   count := metrics.RequestCount

   // 之后
   count := metrics.Snapshot().RequestCount
   ```

2. `endpoint.TypeAssertError.Want` 改为 `reflect.Type`。请用泛型构造函数而不是
   复合字面量创建该错误：

   ```go
   // 之前
   var zero Req
   return &endpoint.TypeAssertError{Got: request, Want: zero}

   // 之后
   return endpoint.NewTypeAssertError[Req](request)
   ```

3. `endpoint.RateLimiterFunc` 改名为 `RateLimiterFuncs`。只有类型名变了，字段
   保持不变：

   ```go
   // 之前
   limiter := endpoint.RateLimiterFunc{AllowFn: bucket.Allow}

   // 之后
   limiter := endpoint.RateLimiterFuncs{AllowFn: bucket.Allow}
   ```

不需要改源码但需要注意的行为变更：

- 端点超时现在编码为 HTTP 504 而不是 500，与 `apperror.KindDeadlineExceeded`
  及 gRPC 适配器一致。客户端断连（`context.Canceled`）现在编码为 499 而不是
  500。此前把这两者当作 500 的客户端与告警规则需要更新。
- `ErrCircuitOpen`、`ErrBulkheadFull`、`ErrBackpressure` 现在编码为 HTTP 503
  而不是 429，因为它们自带 `apperror.KindUnavailable` 分类。`ErrRateLimited`
  仍为 429。在 gRPC 上这四个错误此前都表现为 `Internal`，现在会带上真实的码。
  请更新客户端重试策略以及任何把熔断计为 429 的看板。
- `kit.WithMetrics` 会用路由 pattern 标注每次观测，并运行在链的最外层。
  `metrics.Snapshot()` 仍然给出总量，但现在也会计入被限流或熔断拒绝的请求；
  按路由的数据用 `metrics.SnapshotFor(pattern)` 读取。
- `Fallback` 会在调用方已取消时跳过兜底，并在兜底也失败时合并两个错误，因此
  此前依赖兜底掩盖全部失败的调用方现在会看到主故障原因。

## 升级到 v2.6.0

`v2.6.0` 是经批准的架构进化版本，包含有意的破坏性变更。需要修改源码：

1. 装配层：移除 `kit.Service`、`kit.New`、`kit.MustNew` 与 `Service.Run`。
   拆分为：

   ```go
   // 之前
   svc, err := kit.New(":8080", opts...)
   kit.HandleJSONTyped(svc, "/hello", handler)
   svc.Run(ctx)

   // 之后
   svc, err := kit.NewHTTP(":8080", opts...)
   kit.HandleJSONTyped(svc, "/hello", handler)
   host, err := kit.NewHost(kit.WithLifecycle(svc))
   host.Run(ctx)
   ```

   `Handle`、`HandleFunc`、`HandleSSE` 成为 `*kit.HTTP` 的方法。
2. 错误模型：移除 `server.HTTPError`、`server.NewHTTPError`、
   `server.WrapHTTPError`。用 `apperror` 分类失败（各传输一致映射，包括
   gRPC）；协议专属定制使用 `transporthttp.StatusCoder`、`ErrorCoder`、
   `PublicMessager`、`Headerer`。
3. 移除已废弃的 `log` 门面；直接使用 `log/slog`。
4. SSE：移除 `kit.SSEWriter`。使用 `server.SSEStream`（方法集不变）配合
   `kit.HandleSSETyped`（套用 endpoint 中间件），或用 `HTTP.HandleSSE`
   方法注册原生流。
5. 生成项目（`microgen` `v0.3.0`）：用户持有的 `cmd/custom_routes.go`
   钩子现为纯标准库契约 —— `func registerCustomRoutes(r *http.ServeMux)`
   直接在 mux 上注册路由；把清单条目移入 `customRouteDescriptions() []string`。
   extend 拒绝 v3 之前的清单（`microgen.project.v2`）并给出此迁移提示；
   之后重新完整生成以刷新清单。

值得采用的新能力：`security` 主体契约（`RequireAuthenticated`、
`RequireRole`）、`slogadapter.NewTelemetry`、`kit.NamedLifecycle` /
`kit.ReadinessProvider`、`apperror` 快捷构造函数。

## 升级到 v2.4.1

`v2.4.1` 完全向后兼容：全部为新增能力与行为修复，无需修改源码。

从 `v2.2.0` 跨版本升级时值得复核的行为变化：

- `endpoint.TracingMiddleware` 生成的 trace ID 为 32 位小写十六进制字符
  （W3C Trace Context 格式），不再是 16 位（`v2.3.0` 变更）。把 ID 当作
  不透明字符串的调用方不受影响。
- `endpoint.ErrBackpressure` 与 `endpoint.ErrBulkheadFull` 在 HTTP 中编码为
  429，不再是 500（`v2.3.0` 变更）。

## 兼容性策略

- 补丁版本修复行为；次版本新增能力。两者在 `/v2` 内均向后兼容。
- 不兼容变更需要新的主 module 版本，但经批准并记录的例外除外：
  `v2.1.0` 的直接重构、`v2.2.0` 的指标返回类型修正、`v2.6.0` 的架构进化。
- v2 不保留废弃转发 API。早期版本的文档仍可通过不可变的发布标签获取。
