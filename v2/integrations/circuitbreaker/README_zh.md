# 熔断器

[English](README.md) | 简体中文

熔断器在 v2.4.0 已并入核心 `endpoint` 包。`integrations/circuitbreaker`
模块已废弃；请改用核心实现。

## 安装

无需额外模块。endpoint 包自带中间件：

```go
breaker := endpoint.NewCircuitBreaker(
	endpoint.WithBreakerFailureThreshold(3),
	endpoint.WithBreakerOpenTimeout(5*time.Second),
)
ep := endpoint.NewBuilder(callDependency).Use(breaker.Middleware()).Build()
```

熔断器开启时用 `ErrCircuitOpen` 拒绝调用，窗口过后放行一个探测请求。
`ErrCircuitOpen` 在 HTTP 中编码为 429。

## 设置

| 设置 | 默认值 | 含义 |
| --- | --- | --- |
| FailureThreshold | 5 | 触发熔断的连续失败次数 |
| SuccessThreshold | 1 | 关闭熔断的连续探测成功次数 |
| OpenTimeout | 1 分钟 | 熔断器保持开启的时长 |

## 指标

通过 `breaker.State()` 观察熔断器状态。可将其导出为 gauge，或在
`BreakerOpen` 持续超过一分钟时告警。
