# 排障指南

[English](troubleshooting.md) | 简体中文

按症状组织的指南：一个失败在本框架中意味着什么、该去哪里查。后面各节都假设
第一节的关联配置已经就位。

## 第一步：把一次请求端到端关联起来

后面的所有手段都按请求工作。先一次性接好关联能力：

```go
svc, err := kit.NewHTTP(":8080",
    kit.WithRequestID(),                       // X-Request-ID 进出
    kit.WithEndpointMiddleware(
        endpoint.TracingMiddleware(),          // W3C trace 上下文
        slogadapter.LoggingMiddleware(logger, "myop"), // 业务日志行
    ),
    kit.WithJSONServerOptions(
        server.ServerErrorEncoder(server.JSONErrorEncoder),
        server.ServerErrorHandler(slogadapter.NewErrorHandler(logger)),
    ),
)
host, _ := kit.NewHost(kit.WithLifecycle(svc))
// 传输层访问日志行：
// 经 kit.WithHTTPMiddleware(server.AccessLogMiddleware(logger)) 安装
```

之后，一次失败请求可以通过响应头 `X-Request-ID` 以及业务日志与访问日志中的
`request_id` / `trace_id` 字段追踪。注意：默认的 `ServerErrorHandler` 是
空操作——传输与端点错误在被安装处理器之前只会被**映射**，不会被**记录**。

## 服务启动失败

启动失败是同步的，并且会指明原因：

| 症状 | 含义 | 处理 |
| --- | --- | --- |
| `http listen: ...` | 端口已被占用 | 检查端口；可能有旧实例仍在运行 |
| 配置校验错误 | 生成的 `Config.Validate` 在一切启动前失败 | 错误消息指明出错的键；修正配置/环境变量 |
| `connect database failed`（生成服务） | 启动时 DSN 不可达 | 核对 `database.dsn` / `APP_DB_DSN`、驱动、网络 |
| `start lifecycle component <name>` | 某个 `kit.Lifecycle` 组件的 `Start` 失败 | 实现 `kit.NamedLifecycle` 的组件会带名字 |

配置校验先于日志器、数据库、中间件与服务器创建，因此配置问题不会藏在运行时
日志背后。

## 健康检查失败

- `/readyz` 不可用：响应体会指明失败的检查项
  （`{"checks":[{"name":"db","status":"error"}]}`）。原因：注册的就绪检查失败
  （依赖宕机）、实现 `kit.ReadinessProvider` 的生命周期组件尚未预热完成
  （检查名为 `lifecycle:<name>`）、或检查超过 `WithHealthCheckTimeout`
  （`"check timed out"`）。
- `/livez` 失败：存活检查是独立集合；存活检查失败意味着进程本身应当重启。
- `/health` 同时显示两组检查；设置 `kit.WithMetrics` 时还会显示请求数——请求数
  停滞说明流量没有进入处理链。

## 按状态码排查请求失败

| 状态码 | 典型原因 | 查哪里 |
| --- | --- | --- |
| 400 `bad_request.validation` | `Validatable` 请求被拒 | 错误体列出非法字段 |
| 400（解码） | 严格 JSON 解码拒绝请求体 | 未知字段、重复键或尾部多余数据——设计如此 |
| 401 | 认证拒绝（`apperror.KindUnauthenticated`） | 协议边界的凭据提取 |
| 403 | 授权拒绝（`KindPermissionDenied`） | `security.RequireRole` / 应用策略 |
| 404 | 路由或方法不匹配 | Go mux 模式区分方法（`"POST /x"` ≠ `"GET /x"`）；生成项目中可查 `/debug/routes` |
| 409 / 412 | `KindConflict` / `KindFailedPrecondition` | 业务状态冲突 |
| 429 | 限流拒绝了调用方 | 核对限流器预算；限流器已知等待时长时会带 `Retry-After` |
| 499 | 客户端断连或取消 | `KindCanceled` 或未分类的 `context.Canceled`——不是服务端故障 |
| 500 | 未分类错误 | 响应中内部细节已脱敏——经错误处理器查日志 |
| 501 | `KindUnimplemented` | 路由存在但操作尚未实现 |
| 503 | `KindUnavailable`，或三种卸载负载的保护之一 | 见下一节 |
| 504 | `KindDeadlineExceeded` 或 `TimeoutMiddleware` | 下游依赖过慢 |

## 请求是被哪种保护拒绝的？

限流返回 429（调用方超出了自己的配额）。另外三种保护返回 503，因为此时是服务
在卸载负载。用配置内容与指标区分：

| 来源 | 中间件 | 状态码 | 判别信号 |
| --- | --- | --- | --- |
| 限流 | `RateLimitMiddleware` | 429 | 持续超量流量；核对限流器预算 |
| 熔断 | `breaker.Middleware()` | 503 | 连续失败后开启——真正的问题是失败的依赖；熔断器 `State()` 显示 open/half-open，`Retry-After` 给出开窗剩余时间（半开状态下的拒绝不带提示） |
| 舱壁 | `BulkheadMiddleware` | 调用方的 context 错误（504/499） | 某个 key 占满槽位后请求会**排队**；先是延迟上升，然后调用方超时。用 `errors.Is(err, endpoint.ErrBulkheadFull)` 确认原因 |
| 背压 | `BackpressureMiddleware` | 503 | 全局在途上限；整个服务已饱和 |

`endpoint.Metrics.SnapshotFor(pattern)` 按路由统计错误，`Snapshot()` 给出总量。
`endpoint.Builder.Describe()` **返回**链上的标签（`[]string`，最外层在前）——需要你
自己在启动时打印，才能确认某条路由装配了哪些保护；配合 `UseNamed` 使用，否则标签
全是 `"?"`。


## 数据库问题（生成服务）

- 连接配置：`database.driver`、`database.dsn`（`APP_DB_DSN` 覆盖）、
  `database.auto_migrate`（`APP_DB_AUTO_MIGRATE`）。
- 连接池调优：`database.max_open_conns`、`database.max_idle_conns`、
  `database.conn_max_lifetime`。连接池耗尽会让请求阻塞、随后以超时错误失败——
  先调大 `max_open_conns` 或修慢查询，再考虑调大超时。
- 迁移：`AutoMigrate` 默认关闭，仅在显式开启时于启动期运行。生产 schema
  变更应使用专门的迁移流程。
- 把数据库健康接入就绪检查，让 `/readyz` 在失败暴露给用户之前先摘除流量。

## 日志与可观测性

- 生成服务的日志器由 `logging.level` 与 `logging.format`（`json` 或
  `console`）构建；部署时用 `APP_LOG_LEVEL` / `APP_LOG_FORMAT` 覆盖。
  级别是标准 `log/slog` 级别。
- 日志按设计输出到 **stdout**——没有日志路径配置项。需要文件存储时自建 `slog.Handler`（见[生产指南：日志输出目标](../PRODUCTION_zh.md#日志输出目标与文件存储)）；轮转由应用持有。
- 业务日志行来自 `slogadapter.LoggingMiddleware`（或 `integrations/zap`）；
  协议日志行来自 `server.AccessLogMiddleware`。对应中间件运行时两者都携带
  `request_id` / `trace_id`。
- `endpoint.Metrics.Snapshot()` 无需任何导出器即可读取请求/成功/错误计数与
  平均耗时；`/health` 回显请求数。
- 生产指标导出用 `oteladapter.NewMetrics`；span 用
  `oteladapter.TracingMiddleware`。

## 调试开关

| 开关 | 作用 |
| --- | --- |
| `debug.routes_enabled: true` | 提供 `GET /debug/routes` 列出已注册路由（仅生成项目——框架本身没有这个路由） |
| `debug.print_routes: true` | 启动时打印全部路由（仅生成项目） |
| `endpoint.Builder.Describe()` | 返回单个端点的中间件链标签，供你自行打印 |
| `microgen extend -check` | 校验生成项目的清单漂移 |
