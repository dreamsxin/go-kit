# Rate Limiting

English | [简体中文](README_zh.md)

Rate limiting moved into the core `endpoint` package in v2.4.0. The
`integrations/ratelimit` module is deprecated; use the core implementation.

## Install

No extra module is needed. The endpoint package ships the middleware and the
`RateLimiter` contract:

```go
limiter := ratelimit.New(100) // application-owned bucket
ep := endpoint.NewBuilder(createUser).Use(endpoint.RateLimitMiddleware(limiter)).Build()
```

`RateLimitMiddleware` rejects over-limit requests with `ErrRateLimited`
(HTTP 429). Use `DelayRateLimitMiddleware` when requests should wait for a
token instead of failing; context cancellation aborts the wait.

## The Contract

`RateLimiter` has two methods:

```go
type RateLimiter interface {
	Allow() bool
	Wait(ctx context.Context) error
}
```

A fixed-window bucket or token bucket is application owned. The
`RateLimiterFunc` adapter lets plain functions act as a limiter.

## Distributed Rate Limiting

For multi-replica deployments, a local limiter scales the allowed rate by the
replica count. Add a Redis-backed limiter as an optional integration when the
deployment needs a shared budget.
