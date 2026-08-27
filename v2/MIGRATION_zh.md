# 升级说明

[English](MIGRATION.md) | 简体中文

go-kit v2 遵循语义化版本。本页记录当前版本所需的升级动作；每个版本的
完整变更列表见 [CHANGELOG.md](CHANGELOG_zh.md)。

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
