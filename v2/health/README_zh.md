# health

进程健康检查：一个注册表、两个作用域和探针路由。

[English](README.md) | 简体中文

本包是**本进程**的健康注册表：带名字的存活与就绪检查、按请求求值，以及探针工
具可以依赖的 JSON 报告形态。它就是 `kit.NewHTTP` 用来挂载 `/health`、`/livez`、
`/readyz` 的东西，也可以挂载到你自己的任何 mux 上——纯 gRPC 二进制、worker，或
第二个管理端口。

不要与以下两者混淆：

- kit 的探针选项（`WithLivenessCheck`、`WithReadinessCheck`）——它们在 HTTP 组
  件上填充本注册表；
- [`sd/health`](../sd/README_zh.md)——**远端实例**是否通过主动探测。两者互不回
  答对方的问题。

## 注册表

```go
registry := health.NewRegistry(
	health.WithTimeout(2*time.Second),                     // 每检查预算
	health.WithRequestCount(func() int64 { return served.Load() }), // 可选
)

registry.AddLiveness("process", health.Healthy)            // 永远通过
registry.AddReadiness("database", pingDatabase)            // 运行时依赖
```

- `Check` 是 `func(context.Context) error`；实现必须在 context 取消时立即停止。
  失败、超时、panic 或仍在运行的检查会把所在作用域标记为 `unavailable`。
- 注册表挂载之后仍可添加检查；下一次探针即可看到。
- `WithRequestCount` 把已服务请求数加入报告，给运维一个数字说明进程在做事而不
  只是在应答。

## 挂载

`Mount(mux, paths)` 注册路由：

```go
paths := health.DefaultPaths() // /health, /livez, /readyz
registry.Mount(mux, paths)
```

路由注册时不带方法前缀，因此响应所有方法。`paths` 中的空路径省略该路由——可以
只暴露就绪，或者把探针彻底移出流量端口，把注册表挂到你自己的第二个监听器上：

```go
admin := http.NewServeMux()
registry.Mount(admin, health.Paths{
	All:       "",
	Liveness:  "/healthz/live",
	Readiness: "/healthz/ready",
})
go http.ListenAndServe(":8081", admin)
```

报告体是 `{"status","requests"?,"checks":[...]}`——字段名是稳定契约
（`health.report-shape`），空路径规则同理（`health.mount-skips-empty-path`）。
若需要在 `Mount` 之外使用，`Handler(scope)` 按作用域
（`ScopeAll` / `ScopeLiveness` / `ScopeReadiness`）交出底层 handler。

## 接下来去哪

- [生命周期](../docs/lifecycle_zh.md)：kit 如何挂载探针，以及停机期间就绪失败
  意味着什么
- [生产指导](../PRODUCTION_zh.md)：部署的探针配置
