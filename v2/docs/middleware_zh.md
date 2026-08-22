# 中间件

[English](middleware.md) | 简体中文

中间件是端点层唯一的扩展点。本页覆盖组合顺序、内置目录与四种流控模式。

## 组合

`endpoint.Chain` 与 `endpoint.Builder` 按声明顺序组合中间件；第一个中间件位于
最外层：

```go
ep := endpoint.NewBuilder(createUser).
    WithValidation().
    WithTimeout(5 * time.Second).
    WithMetrics(&metrics).
    Use(endpoint.RateLimitMiddleware(limiter)).
    Use(breaker.Middleware()).
    Build()
```

作用于每条路由的服务级中间件通过 `kit.WithEndpointMiddleware` 安装；拥有自己链的
单个路由用 `endpoint.NewBuilder` 构建，并通过 `kit.HandleJSONEndpoint` 注册。

HTTP 中间件是另一个边界：`kit.WithHTTPMiddleware` 与 `security/http.Chain` 组合
标准的 `http.Handler` 中间件，包裹包括健康检查在内的每一条路由。

## 流控

中间件接收被包裹的端点，并按请求决定链的其余部分如何运行。四种模式覆盖实际场景：

| 模式 | 作用 | 内置示例 |
| --- | --- | --- |
| 短路 | 不调用 `next` 直接应答 | validation、bulkhead、backpressure |
| 分支 | 把请求发送到另一个端点 | `Fallback` |
| 重复 | 带退避再次调用 `next` | `sd/retry` |
| 替换 | 用不同行为包裹 `next` | `TimeoutMiddleware`、`MetricsMiddleware` |

链在构造时固定；中间件不能在运行时重接路由。这让请求路径保持确定且可测试。

## 内置目录

| 中间件 | 行为 | 拒绝方式 |
| --- | --- | --- |
| `ValidationMiddleware` | 校验 `Validatable` 请求 | 400 `bad_request.validation` |
| `TimeoutMiddleware` | 限制端点执行时长 | 500 deadline exceeded |
| `MetricsMiddleware` | 请求计数与耗时 | 从不拒绝 |
| `ErrorHandlingMiddleware` | 用操作名包装端点错误 | 从不拒绝 |
| `TracingMiddleware` | W3C trace context 传播 | 从不拒绝 |
| `BackpressureMiddleware` | 全局在途请求上限（背压） | 429 |
| `CircuitBreaker` | 连续失败使熔断器跳闸，探针使其闭合 | 429 |
| `RateLimitMiddleware` | 拒绝超限请求（限流） | 429 |
| `DelayRateLimitMiddleware` | 等待令牌而非拒绝 | context 错误 |
| `Fallback` | 失败时用降级兜底端点应答 | 从不拒绝 |
| `BulkheadMiddleware` | 按 key 的并发隔离（舱壁隔离） | 429 |

`Fallback` 与 `BulkheadMiddleware` 是 Builder 快捷方式（`WithFallback`、
`WithBulkhead`）；其余的用 `Use` 组合。

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
