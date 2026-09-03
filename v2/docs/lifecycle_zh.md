# 生命周期

[English](lifecycle.md) | 简体中文

`kit.Host` 编排生命周期组件，不拥有任何传输；`kit.HTTP` 组件拥有 HTTP 监听器。
启动成功后由 Host 拥有优雅停机。本页覆盖启动、停机、后台任务与可选服务器。

## 启动与停机

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

svc, err := kit.NewHTTP(":8080")
if err != nil {
	return err
}
// register routes...

host, err := kit.NewHost(
	kit.WithLifecycle(svc),
	kit.WithShutdownTimeout(10*time.Second),
)
if err != nil {
	return err
}

if err := host.Run(ctx); err != nil {
	return err
}
```

- `kit.NewHTTP` 与 `kit.NewHost` 校验配置；启动失败是同步的。
- `host.Run(ctx)` 阻塞直到 context 被取消或某个组件失败，然后在配置的截止时间
  内完成停机。
- Host 在停机后不能重启。

## 健康探针

`kit.NewHTTP` 无条件注册三条路由：

- `/livez` —— 存活。进程还在。失败意味着重启容器。
- `/readyz` —— 就绪。服务可以接流量。失败意味着把实例从负载均衡里摘掉，但不要
  重启它。
- `/health` —— 两个范围合一，给只认一个 URL 的工具用。

任一检查在其范围内失败即返回 503，每个检查默认 2s 超时。请让 Kubernetes 指向
`/livez` 与 `/readyz`，而不是 `/health`——这样依赖故障会摘流，而不是重启 Pod。

生成的项目（`microgen`）暴露同样的三条路由，但跑自己的 `main` 循环——它们不使用
`kit.Host`。其就绪状态在监听器开始服务后置为真，收到第一个停机信号时置为假。

## 可选服务器

gRPC 监听器通过 `kit.Lifecycle` 挂载，并共享同一段有边界的停机：

```go
grpcComponent, err := kitgrpc.New(":8081")
if err != nil {
	return err
}
pb.RegisterGreeterServer(grpcComponent.Server(), greeter)

host, err := kit.NewHost(kit.WithLifecycle(svc, grpcComponent))
```

组件按顺序启动，按相反顺序停机。

## 后台任务

周期性工作放在服务层旁边的独立包中，并通过同一个生命周期挂载，因此 `SIGTERM`
会让任务随进程一起停止。下面的 `Runner` 是你自己 `jobs` 包的模板——框架提供的
是 `kit.Lifecycle` 与 host，而不是这个 runner：

```go
type Job struct {
	Name     string
	Interval time.Duration
	Run      func(ctx context.Context) error
}

type Runner struct {
	Jobs []Job
	// ticker bookkeeping
}

func (r *Runner) Start() error                       { /* one goroutine per job */ }
func (r *Runner) Errors() <-chan error               { /* job failures */ }
func (r *Runner) Shutdown(ctx context.Context) error { /* stop tickers, wait in-flight */ }

runner := &jobs.Runner{Jobs: []jobs.Job{
	{Name: "cleanup-expired", Interval: time.Hour, Run: svc.CleanupExpired},
}}
host, err := kit.NewHost(kit.WithLifecycle(httpComponent, runner))
```

让任务在流量服务旁边安全运行的规则：

- `kit.Lifecycle.Start()` 不接收 context，因此 runner 自己创建一个——在 `Start`
  里 `context.WithCancel(context.Background())`，在 `Shutdown` 里取消——并传给
  每次 `Run`，这样取消信号会经由服务层传到数据库与 HTTP 客户端；
- 失败的运行通过 `Errors()` 上报，并在下一个 tick 重试；runner 绝不会并发启动
  同一个任务的重叠运行；
- 间隔与任务开关属于配置，在启动时校验。

同一份骨架加上周边的包布局见 [PRODUCTION：后台任务](../PRODUCTION_zh.md)。两者
都不是本框架发布的包。
