# 生产指导

[English](PRODUCTION.md) | 简体中文

本指南覆盖部署服务前需要完成的框架级检查。应用特有的认证、授权、数据
治理与运维仍由应用负责。

## 发布前闸门

上线前确认六个方面：

1. [生命周期](#生命周期)：启动有界，停机按逆序执行。
2. [HTTP 服务端](#http-服务器)与[HTTP 客户端](#http-客户端)：限制、deadline 和长连接超时。
3. [服务发现与重试](#服务发现与重试)：明确且考虑幂等性的重试策略，以及资源关闭。
4. [认证与授权](#认证与授权)：协议认证加业务授权。
5. [日志](#日志)、[指标](#指标)和[链路追踪](#链路追踪)：有界、不含敏感信息，并带请求关联。
6. [部署前检查清单](#部署前检查清单)：在 CI 和部署环境各运行一次。

## 生命周期

进程入口持有信号与根 context：

```go
ctx, stop := signal.NotifyContext(
	context.Background(),
	os.Interrupt,
	syscall.SIGTERM,
)
defer stop()

if err := host.Run(ctx); err != nil {
	return err
}
```

使用有界的优雅停机。把监听器绑定失败与异步服务器错误当作启动/运行期
故障处理，而不是记录日志后继续运行。

## 后台任务

周期性工作（清理、对账、缓存预热）放在服务层旁边自己的包中，接入与
HTTP 服务器相同的生命周期，使 `SIGTERM` 随进程一起停止任务：

```text
service/
├── cmd/main.go        # 接线：kit.NewHost(kit.WithLifecycle(http, runner))
├── service/           # 业务逻辑；任务调用这些方法
├── repository/        # 存储
├── transport/         # HTTP handler
└── jobs/              # 每个任务一个文件，外加生命周期 runner
    ├── cleanup.go
    └── runner.go
```

任务是服务层的调用方，不是 endpoint：它们不得导入传输包，其业务逻辑留在
`service/` 中，使 HTTP handler 与任务共享同一份实现。Runner 实现
`kit.Lifecycle`。

下面的 `jobs.Runner` 是你在自己服务里要写的骨架，不是本框架提供的包——框架
提供的是 `kit.Lifecycle` 与驱动它的 host：

```go
type Job struct {
    Name     string
    Interval time.Duration
    Run      func(ctx context.Context) error
}

type Runner struct {
    Jobs []Job
    // ... ticker 记账
}

func (r *Runner) Start() error                         { /* 为每个任务启动一个 goroutine */ }
func (r *Runner) Errors() <-chan error                 { /* 任务 panic 与硬故障 */ }
func (r *Runner) Shutdown(ctx context.Context) error   { /* 停止 ticker，等待在途任务 */ }
```

在 `main` 中接入：

```go
runner := &jobs.Runner{Jobs: []jobs.Job{
    {Name: "cleanup-expired", Interval: time.Hour, Run: svc.CleanupExpired},
}}
host, err := kit.NewHost(kit.WithLifecycle(httpComponent, runner))
```

让任务与线上流量安全共存的规则：

- 每次 `Run` 收到从停机 context 派生的 context，取消会经服务层传导到
  数据库与 HTTP 客户端；
- 失败的运行通过 `Errors()` 上报，并在下一个 tick 重试；runner 绝不
  启动同一任务的重叠运行；
- 间隔与任务开关放在配置中并在启动期校验，使部署无需改代码即可停用
  行为异常的任务；
- 在生成项目中，把包放在相同拓扑下（例如 `service/` 旁的 `jobs/`），
  并在用户持有的 `cmd/main.go` 中接线；重新生成会保留用户文件。

## HTTP 服务器

为部署显式配置以下全部项：

- 读头部超时；
- 读超时；
- 写超时；
- 空闲超时；
- 最大头部字节数；
- 最大 JSON 请求体字节数；
- 优雅停机时限。

严格 JSON endpoint 拒绝未知字段与尾部多余 JSON 值。除非某条路由有明确
记录的理由需要接受更大载荷，否则保持请求体限制开启。

流式协议需要不同的超时选择。MCP SSE 响应是长生命周期的，HTTP 写超时
必须为 `0` 或长于支持的会话时长。

启用生成契约支持时，`/openapi.json`、`/schema.json` 与 `/swagger/`
会暴露服务契约。仅在这是明确的产品决策时才保持公开；否则在部署边界
限制或禁用它们。

## HTTP 客户端

始终设置客户端超时或请求 deadline。JSON 客户端对非 2xx 响应返回
`HTTPStatusError`，并限制捕获的错误响应体大小。

`NewJSONClientWithTimeout` 为每次调用增加 context 超时。确实需要重试时，
使用 `sd/client.NewEndpoint` 配合显式重试策略。

只重试幂等性与错误分类已知的操作。不要假设未知业务错误是瞬态的。

## gRPC

- 启动监听器之前注册服务。
- 每个客户端请求使用新的响应值。
- 保留 context 的 deadline 与取消。
- 在应用组装处配置消息上限与传输凭证。
- 流式行为与一元 RPC 行为分别验证。

## 服务发现与重试

发现订阅者收到不可变快照。消费者应使用缓冲更新 channel，并必须在停机时
注销或关闭自己的 endpointer。`sd/client.NewEndpoint` 返回
`(endpoint, closer, error)`；把 closer 当作持有的运行时状态，在停止
Instancer.Close 之前关闭它，使订阅被移除、工厂创建的客户端连接被释放。
每次 `Balancer.Pick` 都必须在端点返回后配对调用 `Picked.Done`。

内置默认重试分类器（`sd/retry.DefaultClassifier`）只重试通过
`interface{ Retryable() bool }` 自我分类的错误以及 `sd.ErrNoEndpoints`；
context 取消与超时永不重试，未分类错误视为永久错误。它本身不认识 gRPC 或
HTTP 状态码：gRPC 客户端用 `sdclient.WithRetryable(grpc.Retryable)` 补上这层
知识，其他协议同理——在 `sd/client` 上用 `client.WithRetryable(...)`，
手工装配时用 `retry.WithClassifier(...)`。

退避与调用遵循 context 取消。总超时必须覆盖所有尝试与等待，而不是每个
尝试独立计算。

无效的尝试次数、非正超时、负失效时长与 nil 必需依赖会在 Endpointer 启动
前同步失败。

## 配置与密钥

生成配置按 本地 YAML -> 可选远程配置 -> 最终环境变量覆盖 -> 完整结果校验
的顺序解析。

- 不要提交凭据或生产 DSN。
- 密钥通过环境/部署注入。
- 畸形的时长、地址、必需数据库、日志、中间件或远程配置项导致启动失败。
- 除非有意进行启动期 schema 变更，保持数据库迁移关闭。
- 记录脱敏的配置摘要，绝不记录携带密钥的完整配置。

## 认证与授权

认证与授权是集成关注点，不是框架核心特性。在应用边界添加它们：

- 在 HTTP/gRPC 中间件中认证协议凭据；
- 把已验证的主体放入 context；
- 在 service 或 endpoint 策略中执行业务授权；
- 返回协议安全的错误，不泄露内部细节。

除非部署有显式的可信代理策略，否则不要把可信代理头部当作身份。

## 面向浏览器的 HTTP

使用可选的 [`security/http`](security/http/README_zh.md) 包处理 CORS、
签名双提交 CSRF、安全响应头、可信代理解析与客户端 IP 策略。每个中间件
仅在具备部署专用策略时启用。

至少审查：

- 允许的 origin、方法、头部与凭据；
- cookie 认证状态变更的 CSRF 保护；
- 转发头部的信任边界；
- TLS 终止与重定向行为；
- 缓存与 Content-Type 头。

先安装可信代理解析，再安装 IP 策略与依赖 HTTPS 的响应头。只有配置的
直连对端可以影响转发的客户端 IP 或协议。把 CORS 放在 CSRF 之外，使浏览器
预检无需携带令牌。CSRF 只作用于 cookie 认证的浏览器路由；不要覆盖仅
Bearer 的 API 或 MCP POST 路由，除非这些路由有意使用浏览器 cookie 且能回显
令牌头部。这些中间件不包装 `http.ResponseWriter`，因此 SSE 刷新和其他
流式接口保持可用。

`kit` 应用可以用 `kit.WithHTTPMiddleware` 一次性安装已编译的策略；更低层的
应用可以在根 handler 外使用 `httpsecurity.Chain`。

## 日志

使用具备稳定字段的结构化日志：

- 服务与版本；
- 请求/追踪 ID；
- 路由或 RPC 方法；
- 耗时；
- 最终状态/错误类别；
- 有需要时记录发现调用选中的后端。

库返回错误，不调用 `Fatal`。只有 `main` 决定错误是否终止进程。

标准库日志使用可选的
[`observability/slog`](observability/slog/README_zh.md) 适配器。它记录
endpoint 结果、耗时与关联 ID，不记录载荷；应用仍自行选择 `slog.Handler`
与级别。

### 日志输出目标与文件存储

生成服务按设计将结构化日志写到 stdout（12-factor）：容器或节点采集器负责后续处理。框架故意不提供日志路径配置项——文件路径、轮转与留存属于应用或部署决策。

需要文件存储时，自建 `slog.Handler` 并把日志器传给适配器——任何 `slog.Handler` 实现都可用：

```go
file, err := os.OpenFile("service.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
if err != nil {
	return err
}
logger := slog.New(slog.NewJSONHandler(file, &slog.HandlerOptions{Level: slog.LevelInfo}))
// 接入适配器：
//   slogadapter.LoggingMiddleware(logger, op)
//   slogadapter.NewErrorHandler(logger)
```

生成项目中 `cmd/main.go` 归用户所有：在那里修改日志器构造（或在 `config/custom.go` 钩子里构建日志器），而不是修改生成文件。轮转与投递仍由应用持有。

## 指标

业务调用在 endpoint 边界测量，协议细节在传输边界测量。推荐信号包括：

- 按操作/状态类别统计的请求数与耗时；
- 在途请求数；
- 解码与编码失败；
- 限流与熔断拒绝；
- 重试次数与耗尽的重试；
- 发现实例数与更新错误；
- MCP 会话与流数量。

避免无界标签，例如原始 URL、用户 ID、请求 ID 或错误文本。

## 链路追踪

OpenTelemetry 支持属于可选的
[`observability/otel`](observability/otel/README_zh.md) 模块。应用持有
provider 设置、资源、导出器、采样与停机。让 context 贯穿 service、
endpoint、transport、发现与交互调用；在有意义的边界创建 span，而不是为
每个小函数都创建。

无追踪后端时的请求关联，核心包提供端到端的 W3C `traceparent` 头部传播：

- 服务端用 `transport/http.ExtractTraceparent` 作为 `ServerBefore` 钩子
  提取入站头部；
- `endpoint.TracingMiddleware` 加入调用方的追踪，否则生成符合 W3C 的
  追踪；
- 客户端用 `transport/http.InjectTraceparent` 作为 `Before` 钩子转发
  活跃追踪。

完整 span 树仍应使用 `observability/otel`；核心辅助函数以零依赖的方式
让 trace ID 跨服务边界保持连通。

## 健康检查

- 存活探针回答进程是否还能继续运行。
- 就绪探针回答是否应该给它分发流量。
- 依赖检查需要短而独立的超时。
- 健康检查必须在其 context 取消时立即返回。
- 不要在公开的健康响应中暴露密钥、堆栈或完整依赖错误。

`kit` 提供 `/health`、`/livez` 和 `/readyz`。生成项目提供 `/health`；
按需添加部署专用的就绪行为。注册的 `kit` 检查在同一个请求预算内并发执行。
同名检查绝不与上一次调用重叠，这限制了不遵循取消的依赖探测造成的损害。

启用 `kit.WithRequestID` 时，调用方提供的 ID 会先经过校验，再复制进
context、响应与日志。默认接受最长 128 字节的常见 ASCII 令牌字符。仅当
部署使用不同的受信 ID 格式时才使用 `WithRequestIDValidator`。

## 部署

运行时是静态二进制。框架包与纯 Go SQLite 驱动无需 CGO，因此两段式容器
构建可以落在极小的基镜像上：

```dockerfile
FROM golang:1.25 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /out/service ./cmd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/service /service
USER nonroot
ENTRYPOINT ["/service"]
```

在 Kubernetes 上，把健康端点接入探针，并让终止预算与框架停机超时对齐：

- readinessProbe -> `/readyz`，livenessProbe -> `/livez`，两者都用较短的
  周期与超时；就绪在滚动发布期间控制流量；
- `terminationGracePeriodSeconds` 必须大于 kit 停机超时
  （`kit.WithShutdownTimeout`，默认 10 秒）加上平台负载均衡器注销延迟，
  否则在途请求会在停机途中被切断；
- 入口已经在 `SIGTERM` 时取消 `Host.Run`（见生命周期）；仅当服务网格
  或 ingress 在端点注销后仍向 Pod 路由流量时才添加 `preStop` sleep；
- 滚动更新优先使用 `maxUnavailable: 0`，让就绪状态而非 Pod 删除控制
  流量切换。

通过生成的优先级链注入配置（默认值 -> 本地 YAML -> 可选远程配置 ->
环境变量覆盖 -> 校验）。环境变量是容器原生层：每个生成配置项都有
`APP_` 前缀的变量。`Config.Validate` 在监听器绑定之前运行，配置错误的
部署会快速失败，而不是带着降级配置服务流量。

## 告警

在扩容事故之前，而不是之后，把上面的指标信号变成告警。起步集合：

- **错误率**：5 分钟窗口内 5xx 响应（或分类为 `internal` 的错误）占总
  请求的比例；连续两个窗口超过 1% 时触发 page。对比例而不是计数告警，
  避免扩容制造误报。
- **延迟**：p99 endpoint 耗时超过产品声明的预算持续 10 分钟；p95 预警，
  在触发 page 前捕捉劣化趋势。
- **重试耗尽**：对某依赖的重试耗尽说明是依赖而非调用方在故障；对持有
  该依赖的服务发出 page。
- **熔断器开启**：熔断器持续开启超过一分钟，意味着降级路径正在承载生产
  流量。
- **发现抖动**：实例数下降或反复的发现更新错误，指向注册器、网络或健康
  检查配置错误。
- **健康翻转**：就绪间歇性失败而存活保持绿色，可以把依赖故障与进程故障
  区分开。

告警基于比例与时长，标签保持有界（见指标），依赖归属的信号（重试耗尽、
熔断开启）路由到依赖方的值班，而不是调用方的。

## 部署前检查清单

- 配置在部署环境中通过校验。
- HTTP/gRPC 限制与超时匹配工作负载。
- 启用 MCP 时写超时支持长时响应。
- 用 `SIGTERM` 演练过停机。
- 终止宽限期大于 kit 停机超时加注销延迟。
- 就绪与存活探针指向 `/readyz` 与 `/livez`。
- 起步告警（错误率、延迟、重试耗尽、熔断状态）已定义并路由。
- 重试仅限于已分类的安全操作。
- 数据库迁移行为是显式的。
- 认证与授权在协议层和业务层都经过测试。
- 日志、指标、链路追踪不包含密钥与无界维度。
- `go test ./...` 与针对性竞态测试通过。
