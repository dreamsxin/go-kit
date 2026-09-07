# 生命周期

[English](lifecycle.md) | 简体中文

`kit.Host` 编排生命周期组件，不拥有任何传输；`kit.HTTP` 组件拥有 HTTP 监听器。
启动成功后由 Host 拥有优雅停机。本页覆盖启动、停机、后台任务与可选服务器。

## 生命周期一览

```text
main 创建信号 context
  -> 组装组件
  -> Host.Start
  -> 服务运行并监听组件错误
  -> Host.Drain：readiness 开始失败，Draining 组件被通知
  -> 等待 drain 延迟
  -> 按逆序在期限内优雅停机
```

进程入口拥有信号，组件拥有自己创建的资源。生成项目有等价的独立主循环，
不使用 `kit.Host`。

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
	kit.WithDrainDelay(5*time.Second),
)
if err != nil {
	return err
}

if err := host.Run(ctx); err != nil {
	return err
}
```

- `kit.NewHTTP` 与 `kit.NewHost` 校验配置；启动失败是同步的。
- `host.Run(ctx)` 阻塞直到 context 被取消或某个组件失败，然后分三步停止：宣布、
  等待、拆解。
- Host 在停机后不能重启。

## Draining（摘流）

停止从一次宣布开始。`Host.Drain`——`Run` 会替你调用，你没调用时 `Shutdown` 也会
调用——让 readiness 以 `kit.ErrDraining` 失败，并按反向挂载顺序通知每个
`kit.Draining` 组件，且发生在任何东西被拆解之前。liveness 保持通过：一个正在收尾
在途工作的进程不该被重启。

`kit.WithDrainDelay` 是宣布与拆解之间的等待。它默认为零，即立即停止。把它设成略大于
"上游重新读取 readiness 或服务发现的间隔"——否则监听关闭时宣布还没被听到，已经在飞向
这个实例的请求就会失败。

用 `kit.WithRegistrar` 挂载的注册会在"宣布"阶段注销，而不是在"拆解"阶段：实例先离开服务
发现，然后 drain 延迟给注册中心以及每个缓存过它的对端时间去察觉，之后监听才关闭。服务端
唯一无法承诺的是"对端已经读走的那条记录"——这正是这个顺序选择等待、而不是假设的原因。

对有自己工作来源的组件实现 `Draining`：

```go
func (c *Consumer) Drain(ctx context.Context) error {
	c.stopPulling() // 只宣布；不要在这里等在途工作
	return nil
}
```

`Drain` 负责宣布，`Shutdown` 负责收尾：宽限期属于 `Shutdown`，而一个阻塞的 `Drain`
会花掉排在它后面所有组件的预算。`Drain` 的错误会被上报，流程继续——一个无法停止接活的
组件仍然必须被关闭。

## 长连接响应

流是没法"礼貌地摘掉"的：`http.Server.Shutdown` 会等 handler 返回，而每秒写一个事件的
handler 永远不会返回。所以 HTTP 组件改为告诉它的 handler：

```go
component.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
	for {
		select {
		case <-kit.Stopping(r.Context()):
			return // 进程要走了；把流收尾
		case <-r.Context().Done():
			return // 这个客户端走了
		case event := <-events:
			// 写事件
		}
	}
})
```

`kit.Stopping(ctx)` 在 draining 开始时关闭——早于任何连接被关闭，于是响应可以自己收尾，
客户端看到的是"流结束"，而不是"连接断了"。在 kit HTTP 组件之外它返回 nil，而从 nil
channel 接收会永久阻塞，所以上面的 select 两种情况下都是对的。

对既不看信号、也不看 context 的 handler，宽限期同样会结束。`Shutdown` 先尝试
`http.Server.Shutdown`；预算用尽时它取消请求 context，给 handler 一小段时间收尾，关闭
剩下的连接，并返回一个包着 `kit.ErrShutdownIncomplete` 的错误，说明打断了多少个请求。
它不会在自己拥有的连接仍然打开时返回。

## 被 hijack 的连接不会被 drain

有一类连接在这一切之外。一旦 handler 调用了 `Hijack`——WebSocket，或任何其它协议升级——
服务器就不再跟踪这条连接：优雅等待不包含它，关闭监听器不会关闭它，硬关闭也够不到它。
`Shutdown` 会很快返回 `nil`，而那条升级后的连接照样在传字节。

这不是框架该去补的窟窿，这就是 hijack 的含义。缝隙留在做了升级的那个 handler 上，信号
和流式响应用的是同一个：

```go
component.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	go readFrames(conn)
	<-kit.Stopping(r.Context()) // 进程要走了；关掉这个 socket
})
```

注意它依赖的次序：`Stopping` 在 draining 开始时关闭，早于 `Shutdown` 执行，所以看这个
信号的升级 handler 是在宽限期之内收尾，而不是之后。请把 `kit.WithDrainDelay` 设得足够
让这件事发生。

在 HTTP/2 上没有升级可以 hijack —— h2 没有 `101 Switching Protocols`。因此基于 hijack
的协议只在明文监听器上可用；配置了证书之后还会变什么，见
[提供 TLS](configuration_zh.md#提供-tls)。

## gRPC 以同样的方式 drain

`kit/grpc.Component` 被同一套时序 drain，gRPC handler 读的也是同一个信号：

```go
func (s *server) Watch(req *pb.WatchRequest, stream pb.Watcher_WatchServer) error {
	for {
		select {
		case <-kit.Stopping(stream.Context()):
			return nil // 进程要走了；结束这个流
		case <-stream.Context().Done():
			return stream.Context().Err()
		case event := <-events:
			// stream.Send(event)
		}
	}
}
```

drain 一个 gRPC 组件是"通告"，不是"开始拒绝调用"。在 drain 延迟里收到 `UNAVAILABLE` 的客户端
会重试，而且可能重试到同一个实例上——因为路由层还没跟上；真正让流量挪走的信号是 readiness
失败，而 gRPC 健康服务报告的正是它。

限制与 HTTP 那边正好互为镜像。`GracefulStop` 确实会等在途 RPC，所以一个什么都不看的流会把
进程占到停机预算用尽；然后服务器被停掉，`Shutdown` 返回那个 deadline 错误，而不是报告成功。
和被 hijack 的 HTTP 连接不同，这里没有任何东西逃出停机流程——它只是花掉了整份预算。

如果你自己写传输，`kit.WithStopping(ctx, ch)` 就是那个接缝：把它带进你的 handler context，
`kit.Stopping` 在那里同样有效。

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
