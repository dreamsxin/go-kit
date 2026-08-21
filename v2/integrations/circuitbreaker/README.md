# Circuit Breaker

English | [简体中文](README_zh.md)

Circuit breaking moved into the core `endpoint` package in v2.4.0. The
`integrations/circuitbreaker` module is deprecated; use the core
implementation instead.

## Install

No extra module is needed. The endpoint package ships the middleware:

```go
breaker := endpoint.NewCircuitBreaker(
	endpoint.WithBreakerFailureThreshold(3),
	endpoint.WithBreakerOpenTimeout(5*time.Second),
)
ep := endpoint.NewBuilder(callDependency).Use(breaker.Middleware()).Build()
```

The breaker rejects calls with `ErrCircuitOpen` while open and lets one probe
through after the window. `ErrCircuitOpen` encodes as HTTP 429.

## Settings

| Setting | Default | Meaning |
| --- | --- | --- |
| FailureThreshold | 5 | consecutive failures that trip the breaker |
| SuccessThreshold | 1 | consecutive probe successes that close it |
| OpenTimeout | 1 minute | time the breaker stays open before probing |

## Metrics

The breaker state is observable through `breaker.State()`. Export it as a
gauge or alert on `BreakerOpen` staying true for more than a minute.
