# endpoint（端点）

[English](README.md) | 简体中文

`endpoint` 包是 `go-kit` 的核心运行时抽象。

业务操作在这里被包装上可复用的运行时策略，例如：

- 超时
- 指标
- 链路追踪
- 背压
- 熔断
- 限流

如果 `service` 是业务层，`transport` 是协议层，那么 `endpoint` 就是位于两者之间的运行时治理层。

## 核心抽象

### `Endpoint`

核心类型是：

```go
type Endpoint func(ctx context.Context, request any) (response any, err error)
```

它是被以下各方共享的可调用单元：

- 传输层
- 中间件
- 服务包装器
- 服务发现与客户端执行流程

### `Middleware`

标准的中间件形式是：

```go
type Middleware func(Endpoint) Endpoint
```

这让运行时策略保持可组合，且与传输层无关。

### `Failer`

`Failer` 允许响应类型在不占用 Go 错误返回值的情况下携带业务错误。

仅当传输层要求即使业务失败也要给出成功的线上级响应时才使用它。

大多数业务逻辑仍然应当优先使用普通的 Go 错误。

## 推荐入口

对大多数服务而言，主要入口是：

- `Endpoint`
- `Middleware`
- `NewBuilder`
- `NewTypedBuilder`
- `Chain`
- `TimeoutMiddleware`
- `MetricsMiddleware`
- `ErrorHandlingMiddleware`
- `TracingMiddleware`
- `BackpressureMiddleware`
- `NewCircuitBreaker`
- `RateLimitMiddleware` / `DelayRateLimitMiddleware`
- `Unwrap`

相关扩展包：

- `observability/slog`
- `integrations/zap`

## Builder API

builder API 是组合端点行为的推荐默认方式。

示例：

```go
var metrics endpoint.Metrics

ep := endpoint.NewBuilder(base).
    WithMetrics(&metrics).
    WithErrorHandling("CreateUser").
    WithTimeout(5 * time.Second).
    Use(endpoint.NewCircuitBreaker().Middleware()).
    Use(endpoint.RateLimitMiddleware(limiter)).
    Build()

snapshot := metrics.Snapshot()
fmt.Println(snapshot.RequestCount, snapshot.ErrorCount, snapshot.AverageDuration())
```

每次并发读取都应使用 `Snapshot`。它返回 `MetricsSnapshot`，一个可以安全复制的无锁值。收集器的导出字段仍保留可用以保持源码兼容，但在中间件更新收集器时读取它们会构成数据竞争。

为什么优先选择 builder：

- 比手工包装多层中间件更清晰
- 在同一处表达运行时策略
- 与框架偏好的组合风格保持一致

builder 契约说明：

- `NewBuilder` 要求非 nil 的基础端点
- `Use(...)` 要求非 nil 的中间件值
- 非法的组合输入会快速失败，而不是把问题推迟到请求时

## `Chain`

`Chain` 是更底层的中间件组合辅助工具。

示例：

```go
ep = endpoint.Chain(
    loggingMiddleware,
    metricsMiddleware,
    authMiddleware,
)(base)
```

中间件的顺序依然重要：

- 传给 `Chain` 的第一个中间件位于最外层

## 中间件流控

链路在构造期固定，但中间件完全掌控接下来的走向：它拿到被包装的
endpoint，并在每次请求时决定链路其余部分如何执行。四种模式覆盖了实际
场景。

**短路** - 终止链路并立即应答。内置的 `ValidationMiddleware`、
`BackpressureMiddleware` 和 `BulkheadMiddleware` 都这样做：

```go
func requireTenant(next endpoint.Endpoint) endpoint.Endpoint {
    return func(ctx context.Context, request any) (any, error) {
        if tenantFromContext(ctx) == "" {
            return nil, endpoint.NewValidationError("tenant", "is required")
        }
        return next(ctx, request)
    }
}
```

**分支** - 把请求送到别处。`Fallback` 正是这个模式：兜底 endpoint 本身
可以是一条带完整中间件的链，因此分支可以拥有自己的超时与指标：

```go
degraded := endpoint.NewBuilder(cachedResponse).
    WithTimeout(time.Second).
    Build()
ep := endpoint.NewBuilder(primary).WithFallback(degraded).Build()
```

**重复** - 再次调用链路的剩余部分。`sd/retry` 包装 `next` 并按尝试次数
反复调用，带有自己的退避与错误分类。

**替换** - 用不同行为包装 `next` 而不是直接调用它：
`TimeoutMiddleware` 在 deadline 下于 goroutine 中运行 `next`，
`MetricsMiddleware` 观察其结果。被包装的 endpoint 保持不透明，中间件
不需要知道链路还有多深。

中间件做不到的一件事，是在运行期改动已构造路由的链路--组装发生在装配
期，这让请求路径确定且可测试。按路由的变化属于装配期
（`kit.WithEndpointMiddleware` 作用于全部路由；某条路由需要自己的链时，
用 `endpoint.NewBuilder` 构建 endpoint 并经 `kit.HandleJSONEndpoint`
注册）。

## 类型化端点

`TypedEndpoint[Req, Resp]` 在保持相同运行时模型的同时，提供编译期的请求与响应类型约束。

示例：

```go
var ep endpoint.TypedEndpoint[HelloReq, HelloResp] =
    func(ctx context.Context, req HelloReq) (HelloResp, error) {
        return HelloResp{Message: "Hello, " + req.Name}, nil
    }

typed := endpoint.Unwrap[HelloReq, HelloResp](
    endpoint.NewTypedBuilder(ep).
        WithTimeout(5 * time.Second).
        Use(endpoint.NewCircuitBreaker().Middleware()).
        Build(),
)
```

在以下情形使用类型化端点：

- 调用点需要类型安全
- 你希望减少运行时类型断言

可以渐进式地采用类型化端点。

类型断言说明：

- 当请求或响应值与期望类型不匹配时，`Wrap()` 和 `Unwrap()` 会返回类型断言错误，而不是在类型不匹配时 panic。

## 内置中间件

`endpoint` 中的核心中间件：

- `MetricsMiddleware`
- `ErrorHandlingMiddleware`
- `TimeoutMiddleware`
- `TracingMiddleware`
- `ValidationMiddleware`
- `BackpressureMiddleware`
- `Fallback`
- `BulkheadMiddleware`
- `CircuitBreaker`
- `RateLimitMiddleware`
- `DelayRateLimitMiddleware`

`CircuitBreaker` 是 endpoint 包内置的无依赖熔断器：连续失败会触发开启，
开启期间用 `ErrCircuitOpen`（HTTP 429）拒绝调用，窗口过后的探测请求决定
是否恢复。限流由 `RateLimitMiddleware`（拒绝）与 `DelayRateLimitMiddleware`
（等待）承载，基于应用自有的 `RateLimiter` 契约；`ErrRateLimited` 同样
编码为 429。

`TracingMiddleware` 使用 W3C Trace Context 格式。它会在同一个 trace ID 下加入传入的
`TraceContext`（由 `transport/http.ExtractTraceparent` 从 `traceparent` 头部提取），
否则铸造一个符合 W3C 的 trace，并通过 `TraceIDFromContext` 暴露相同的 ID。出站
HTTP 调用使用 `transport/http.InjectTraceparent` 转发活跃 trace。

`ValidationMiddleware` 会在业务逻辑运行之前，对实现了 `Validatable` 的请求调用
`Validate() error`。`NewValidationError` 与 `ValidationError.Add` 收集字段级失败；
HTTP 传输层将 `ValidationError` 映射为 400 与稳定代码 `bad_request.validation`，
而没有 `Validate` 方法的请求会原样通过：

```go
type CreateUserRequest struct{ Name string }

func (r CreateUserRequest) Validate() error {
    if r.Name == "" {
        return endpoint.NewValidationError("name", "is required")
    }
    return nil
}

ep := endpoint.NewBuilder(createUser).WithValidation().Build()
```

`Fallback` 在主端点失败时以降级兜底端点应答，在依赖恢复期间把错误挡在调用方之外。
`BulkheadMiddleware` 按资源键（租户、依赖）限制并发，这样一个慢的键无法像全局
`BackpressureMiddleware` 计数那样耗尽共享预算；两种拒绝错误都编码为 HTTP 429。
舱壁隔离的键必须保持有界，因为每个键在端点的生命周期内都占有一个槽位池：

```go
ep := endpoint.NewBuilder(callDependency).
    WithBulkhead(20, func(req any) string { return req.(Request).TenantID }).
    WithFallback(cachedResponse).
    Build()
```

日志与具体提供方相关，位于核心包之外：

```go
ep = zapadapter.LoggingMiddleware(logger, "CreateUser")(ep)
// Or use observability/slog with a standard-library slog.Logger.
```

这使核心 `endpoint` 的导入图仅限于 Go 标准库与零依赖的 `apperror` 包。

熔断与限流已内置于核心包，不持有第三方依赖：

- `NewCircuitBreaker`（`.Middleware()`）在熔断开启时用 `ErrCircuitOpen` 拒绝调用，并通过探测请求恢复。
- `RateLimitMiddleware` 超限时用 `ErrRateLimited` 拒绝；`DelayRateLimitMiddleware` 等待令牌，并遵循 context 取消。限流器实现 `RateLimiter` 契约，由应用持有。

## 什么属于 `endpoint`

适合这一层的职责：

- 运行时超时策略
- 请求计量与指标
- 结构化错误包装
- 请求关联与协议无关的埋点钩子
- 弹性包装器
- 可复用的调用策略

## 什么不属于 `endpoint`

避免把这些关注点放到这里：

- 协议特定的编解码逻辑
- HTTP 或 gRPC 请求映射
- 数据库访问逻辑
- 产品特定的工作流编排
- 无法泛化的一次性应用行为

如果某个关注点是协议特定的，它多半属于 `transport`。
如果是纯领域行为，它多半属于 `service`。

## 扩展点

主要受支持的扩展面是自定义中间件。

推荐的扩展模式：

- 组合自定义 `Middleware`
- 使用 `Builder.Use(...)`
- 通过 `NewTypedBuilder` 包装类型化端点
- 把熔断器或限流器适配器插入中间件链

避免：

- 创建绕过 `Endpoint` 的平行中间件模型
- 除非不可避免，否则不要把传输层特定的关注点编码进中间件

## 稳定性说明

已发布的 `v2.0.0` endpoint API 仍是历史性的稳定基线。获得批准的 `v2.1.0`
SemVer 例外将本包契约重置为 `tools/testdata/api_surface.sha256` 中评审过的
API。在 `v2.1.0` 之后，兼容性将重新覆盖：

- `Endpoint`
- `Middleware`
- builder 风格的组合
- 框架的核心中间件模型

`integrations/circuitbreaker` 和 `integrations/ratelimit` 等专用模块仍是可独立
选择的组件。

## 最佳实践

1. 保持端点中间件跨服务可复用。
2. 优先使用端点中间件，而不是传输层特定的策略代码。
3. 除非确实与策略紧密相关，否则不要把业务逻辑放进端点包装器。
4. 在类型化端点能提升安全性与可读性的地方使用它们。
5. 把端点组合视为运行时治理的默认位置。

## 相关文档

- [README.md](../README_zh.md)
- [ARCHITECTURE.md](../ARCHITECTURE_zh.md)
- [PRODUCTION.md](../PRODUCTION_zh.md)
