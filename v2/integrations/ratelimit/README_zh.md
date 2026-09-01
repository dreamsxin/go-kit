# 限流

[English](README.md) | 简体中文

限流在 v2.4.0 已并入核心 `endpoint` 包。`integrations/ratelimit` 模块已废弃；
请改用核心实现。

## 安装

无需额外模块。endpoint 包自带中间件与 `RateLimiter` 契约：

```go
limiter := ratelimit.New(100) // 应用持有的桶
ep := endpoint.NewBuilder(createUser).Use(endpoint.RateLimitMiddleware(limiter)).Build()
```

`RateLimitMiddleware` 超限请求用 `ErrRateLimited` 拒绝（HTTP 429）。
需要等待令牌时用 `DelayRateLimitMiddleware`；context 取消会中止等待。

## 契约

`RateLimiter` 有两个方法：

```go
type RateLimiter interface {
	Allow() bool
	Wait(ctx context.Context) error
}
```

固定窗口桶或令牌桶由应用持有。`RateLimiterFuncs` 适配器让普通函数也能
作为限流器。

## 分布式限流

多副本部署时，本地限流会把允许速率按副本数放大。需要共享预算时，添加
基于 Redis 的限流作为可选集成。
