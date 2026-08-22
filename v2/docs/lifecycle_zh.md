# 生命周期

[English](lifecycle.md) | 简体中文

`kit.Service` 在启动成功后拥有 HTTP 监听器与优雅停机。本页覆盖启动、停机、后台
任务与可选服务器。

## 启动与停机

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

svc, err := kit.New(":8080", kit.WithShutdownTimeout(10*time.Second))
if err != nil {
	return err
}
// register routes...

if err := svc.Run(ctx); err != nil {
	return err
}
```

- `kit.New` 校验配置；启动失败是同步的。
- `svc.Run(ctx)` 阻塞直到 context 被取消或某个服务器失败，然后在配置的截止时间
  内完成停机。
- 服务在停机后不能重启。

## 可选服务器

gRPC 监听器通过 `kit.Lifecycle` 挂载，并共享同一段有边界的停机：

```go
grpcComponent, err := kitgrpc.New(":8081")
if err != nil {
	return err
}
pb.RegisterGreeterServer(grpcComponent.Server(), greeter)

svc, err := kit.New(":8080", kit.WithLifecycle(grpcComponent))
```

组件按顺序启动，按相反顺序停机。

## 后台任务

周期性工作放在服务层旁边的独立包中，并通过同一个生命周期挂载，因此 `SIGTERM`
会让任务随进程一起停止：

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
svc, err := kit.New(":8080", kit.WithLifecycle(runner))
```

让任务在流量服务旁边安全运行的规则：

- 每次 `Run` 收到的 context 派生自停机 context，因此取消信号会经由服务层传到
  数据库与 HTTP 客户端；
- 失败的运行通过 `Errors()` 上报，并在下一个 tick 重试；runner 绝不会并发启动
  同一个任务的重叠运行；
- 间隔与任务开关属于配置，在启动时校验。

完整的参考实现见 [PRODUCTION：后台任务](../PRODUCTION_zh.md)。
