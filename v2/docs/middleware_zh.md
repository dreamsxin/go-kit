# 中间件

[English](middleware.md) | 简体中文

中间件是端点层的主要扩展点；HTTP 边界另有自己的 `http.Handler` 中间件。

## 先选择层级

| 需求 | 使用 |
| --- | --- |
| 已解码请求、业务结果、超时、重试、准入 | `endpoint.Middleware` |
| method、path、header、状态码、字节、raw handler | `http.Handler` middleware |
| 编解码前后的协议元数据 | transport hook |
| 服务启停与健康状态 | `kit.Lifecycle` / `kit.Host` |

第一个添加的中间件位于最外层。下面先看拒绝语义，再看[自定义](customization_zh.md)。

## 组合

`endpoint.Chain` 与 `endpoint.Builder` 按声明顺序组合中间件；第一个中间件位于
最外层：

```go
ep := endpoint.NewBuilder(createUser).
    WithValidation().
    WithTimeout(5 * time.Second).
    WithRecording("POST /users", &metrics).
    WithRateLimit(limiter).
    WithCircuitBreaker(breaker).
    Build()
```

作用于每条路由的服务级中间件通过 `kit.WithEndpointMiddleware` 安装；拥有自己链的单个路由使用 `kit.HandleJSONTypedWithMiddleware`，或用 `endpoint.NewBuilder` 构建后通过 `kit.HandleJSONEndpoint` 注册。

HTTP 中间件是另一个边界：`kit.WithHTTPMiddleware` 与 `security/http.Chain` 组合
标准的 `http.Handler` 中间件，包裹包括健康检查在内的每一条路由。

## 按层安放的横切关注点

日志、追踪、指标与错误处理在每一层都有一个规范位置：

| 关注点 | Service 层 | Endpoint 层 | Transport 层 |
| --- | --- | --- | --- |
| 日志 | 保持纯净；从 context 读关联 ID | `slogadapter.LoggingMiddleware`、`integrations/zap` 或 `slogadapter.NewTelemetry` | `server.AccessLogMiddleware` 记录协议事实；`ServerErrorHandler` 记录失败 |
| 追踪 | context 携带 trace 与请求 ID | `TracingMiddleware`（W3C 关联）、`oteladapter.TracingMiddleware`（span） | `transporthttp.ExtractTraceparent` / `InjectTraceparent` 头传播 |
| 指标 | — | `RecordingMiddleware` 搭配任意 `Recorder`——内存用 `endpoint.Metrics`，OpenTelemetry 用 `oteladapter.NewMetrics` | 访问日志的状态码/字节数；`ServerFinalizer` 钩子 |
| 错误 | 返回 `apperror` 分类 | `ErrorHandlingMiddleware` 附操作名；`ValidationMiddleware` 短路 | `ErrorEncoder` 映射状态码；`ErrorHandler` 观察 |

安放规则：

- Service 层保持纯净：横切行为由包裹它的 endpoint 中间件应用。服务代码从
  context 读关联 ID，返回 `apperror` 分类的失败。
- Endpoint 层是业务级日志、追踪、指标与错误装饰的规范位置——它能看到解码后
  的请求、业务响应与业务错误。
- Transport 层拥有协议事实：访问日志、状态码映射与头传播。它绝不重新解读业务结果。

## 流控

中间件接收被包裹的端点，并按请求决定链的其余部分如何运行。四种模式覆盖实际场景：

| 模式 | 作用 | 内置示例 |
| --- | --- | --- |
| 短路 | 不调用 `next` 直接应答 | validation、bulkhead、backpressure |
| 分支 | 把请求发送到另一个端点 | `Fallback` |
| 重复 | 带退避再次调用 `next` | `RetryMiddleware`、`sd/retry` |
| 替换 | 用不同行为包裹 `next` | `TimeoutMiddleware`、`RecordingMiddleware` |

链在构造时固定；中间件不能在运行时重接路由。这让请求路径保持确定且可测试。

## 内置目录

| 中间件 | 行为 | 拒绝方式 |
| --- | --- | --- |
| `ValidationMiddleware` | 校验 `Validatable` 请求 | 400 `bad_request.validation` |
| `TimeoutMiddleware` | 限制端点执行时长 | 504 gateway timeout |
| `RecordingMiddleware` | 把每次调用上报给一个或多个 `Recorder` | 从不拒绝 |
| `MetricsMiddleware` | 无标签的内存计数 | 从不拒绝 |
| `ErrorHandlingMiddleware` | 用操作名包装端点错误 | 从不拒绝 |
| `TracingMiddleware` | W3C trace context 传播 | 从不拒绝 |
| `BackpressureMiddleware` | 全局在途请求上限（背压） | 503 unavailable |
| `InFlightMiddleware` | 全局在途上限，并对外发布实时计数 | 503 unavailable |
| `CircuitBreaker.Middleware()` | 连续失败使熔断器跳闸，探针使其闭合 | 503 unavailable + `Retry-After` |
| `RateLimitMiddleware` | 拒绝超限请求（限流） | 429 too many requests |
| `DelayRateLimitMiddleware` | 等待令牌而非拒绝 | context 错误 |
| `RetryMiddleware` | 对瞬时失败带退避重试 | 返回最后一次错误 |
| `Fallback` | 失败时用降级兜底端点应答 | 兜底也失败时合并两个错误 |
| `BulkheadMiddleware` | 按 key 的并发**排队**（舱壁隔离） | 调用方的 context 错误（504/499），并包装 `ErrBulkheadFull` |

限流用 429，因为是调用方超出了自己的配额；熔断跳闸与背压用 503，因为此时是服务
在卸载负载。舱壁是例外：它不拒绝而是排队，因此某个 key 饱和先表现为延迟，然后
表现为结束调用方等待的原因——超时仍是 504，断连仍是 499。
`errors.Is(err, endpoint.ErrBulkheadFull)` 仍能查到背后的饱和原因。请把
`WithBulkhead` 与 `WithTimeout` 搭配，让队列有个边界。完整映射见
[错误处理](errors_zh.md)。

`CircuitBreaker` 本身不是 `Middleware`：用 `endpoint.NewCircuitBreaker()` 构造一次，
装上 `breaker.Middleware()`，并保留 `*CircuitBreaker` 以便调用 `State()`。

目录中的每个中间件都有 Builder 快捷方式（`WithValidation`、`WithTimeout`、
`WithRecording`、`WithMetrics`、`WithRateLimit`、`WithDelayRateLimit`、
`WithCircuitBreaker`、`WithRetry`、`WithFallback`、`WithBulkhead`、
`WithBackpressure`、`WithTracing`、`WithErrorHandling`），因此 `Use` 与 `UseNamed`
只在自定义中间件时才需要。`InFlightMiddleware` 没有快捷方式，因为它需要调用方的
计数器；用 `Use` 安装。

## 重试瞬时失败

`RetryMiddleware` 在错误可重试时重复调用。默认分类器重试
`apperror.KindUnavailable`，以及通过 `interface{ Retryable() bool }` 自行分类的
错误；它不重试 context 错误、本地拒绝（熔断跳闸、限流、舱壁、背压）以及未分类
错误。默认退避为指数退避加全抖动；若错误自带重试提示（`RetryAfterReporter`，
HTTP 客户端会从 `Retry-After` 响应头填充），该提示优先于本地退避。

幂等性由调用方负责——只包裹重复调用安全的端点。把重试放在熔断器内侧，这样
依赖故障只会让熔断器跳闸一次，而不是每次尝试都记一次：

```go
ep := endpoint.NewBuilder(callDependency).
    WithCircuitBreaker(breaker).
    WithRetry(3).
    Build()
```

同一套组合也适用于对外调用，因为客户端本身就是 endpoint，见
[错误处理](errors_zh.md)。

## 记录指标

`RecordingMiddleware` 为每次调用计时，并把 `endpoint.Observation`（操作名、
耗时、错误）交给每个 `endpoint.Recorder`。实现 `Recorder` 即可对接 Prometheus
或 OpenTelemetry；内置的 `endpoint.Metrics` 收集器按操作维度保存内存计数并同时
维护总量。

`kit.WithMetrics` 与 `kit.WithRecorder` 会自动完成接线，并用路由 pattern 作为
每条路由的标签：

```go
var metrics endpoint.Metrics
component, err := kit.NewHTTP(":8080",
    kit.WithMetrics(&metrics),
    kit.WithRecorder(promRecorder),
)
// metrics.SnapshotFor("POST /users") 只报告该路由。
```

记录位于最外层，因此被限流或熔断拒绝的请求同样会被计入。

## 调试链路

`Builder.Describe` 按应用顺序返回中间件链，启动日志可以精确打印实际执行
内容：

```go
b := endpoint.NewBuilder(createUser).
    WithValidation().
    UseNamed("auth", authMiddleware).
    WithTimeout(5 * time.Second)
log.Printf("chain: %s -> endpoint", strings.Join(b.Describe(), " -> "))
// chain: validation -> auth -> timeout -> endpoint
```

内置 `With*` 快捷方式自动记录标签；用 `Use` 组合的自定义中间件显示为
`?`，除非用 `UseNamed` 打标签。

## 嵌套

被包裹的端点本身也可以是一条链。一个自带超时与指标的降级兜底：

```go
degraded := endpoint.NewBuilder(cachedResponse).
    WithTimeout(time.Second).
    WithMetrics(&degradedMetrics).
    Build()
ep := endpoint.NewBuilder(primary).WithFallback(degraded).Build()
```

可运行的巡游见 [examples/middleware](../examples/README_zh.md)。
