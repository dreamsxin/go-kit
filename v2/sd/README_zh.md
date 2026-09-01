# sd - 服务发现

[English](README.md) | 简体中文

根 `sd` 包拥有协议无关的 `Event`、`Instancer`、`Registrar`、`Balancer` 和
`ErrNoEndpoints` 契约。具体组件位于职责聚焦的子包中，可以独立使用。

## 快速开始（无需 Consul）

```go
import (
    "github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/client"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
    "github.com/dreamsxin/go-kit/v2/sd/instance"
)

factory := endpointer.Factory(func(instance string) (endpoint.Endpoint, io.Closer, error) {
	return makeClientEndpoint(instance), nil, nil
})

// In-memory instancer — perfect for tests and local dev
cache := instance.NewCache()
cache.Update(sd.Event{Instances: []string{"host1:8080", "host2:8080"}})

ep, closer, err := client.NewEndpoint(cache, factory, logger,
    client.WithMaxAttempts(3),
    client.WithTimeout(500*time.Millisecond),
)
if err != nil {
    return err
}
defer closer.Close()
resp, err := ep(ctx, request)
```

## 使用 Consul

```go
import (
	"github.com/dreamsxin/go-kit/v2/sd/client"
	"github.com/dreamsxin/go-kit/v2/integrations/consul"
)

instancer := consul.NewInstancer(consulClient, logger, "my-service", true)

ep, closer, err := client.NewEndpoint(instancer, factory, logger,
    client.WithMaxAttempts(3),
    client.WithTimeout(500*time.Millisecond),
    client.WithInvalidateOnError(5*time.Second),
)
if err != nil {
    instancer.Stop()
    return err
}
defer instancer.Stop()
defer closer.Close() // runs first: deregister and close endpoint connections
```

## 选项

| 选项 | 默认值 | 说明 |
|--------|---------|-------------|
| `WithMaxAttempts(n)` | 1 | 总尝试次数；必须至少为 1 |
| `WithTimeout(d)` | 500ms | 包含所有重试在内的正数总预算 |
| `WithInvalidateOnError(d)` | disabled | 在 SD 错误宽限期之后清除缓存 |
| `WithBalancer(f)` | 轮询 | 替换选择策略 |

非法的选项以及为 nil 的必需依赖，会在任何后台 goroutine 启动之前返回错误。

## 选择策略

`sd.Balancer` 是扩展点；`sd/balancer` 内置了四种策略。

| 策略 | 构造器 | 依据 | 适用场景 |
|----------|-------------|----------|-------------|
| 轮询 | `balancer.NewRoundRobin` | 原子计数器 | 默认；实例同质、单一客户端 |
| 随机 | `balancer.NewRandom` | 均匀抽样 | 多个客户端共享同一实例集合，计数器会步调一致 |
| 加权随机 | `balancer.NewWeightedRandom` | 调用方为每个实例给出的权重 | 容量不一致、灰度比例、摘除某个实例 |
| 一致性哈希 | `balancer.NewConsistentHash` | 请求键的哈希 | 缓存亲和性或按实体保序 |

```go
// 均匀随机
client.WithBalancer(func(set endpointer.InstanceEndpointer) sd.Balancer {
	return balancer.NewRandom(set)
})

// 加权：容量来自实例元数据，权重为 0 即可摘除实例，
// 无需等待服务发现把它摘掉。
client.WithBalancer(func(set endpointer.InstanceEndpointer) sd.Balancer {
	return balancer.NewWeightedRandom(set, func(instance string) int {
		return weights[instance]
	})
})

// 一致性哈希：只要实例还在集合中，同一租户的请求就始终落到同一实例。
client.WithBalancer(func(set endpointer.InstanceEndpointer) sd.Balancer {
	return balancer.NewConsistentHash(set, func(_ context.Context, request any) string {
		return request.(*GetProfileRequest).TenantID
	}, balancer.WithReplicas(200))
})
```

加权与哈希策略需要知道端点由哪个实例生成，因此它们接收
`endpointer.InstanceEndpointer` 而不是更窄的 `endpointer.Endpointer`。
`endpointer.NewEndpointer` 返回的就是前者。

选型之前有两点行为值得了解：

- 当端点存在但所有权重都不大于 0 时，加权随机返回 `sd.ErrNoEndpoints`。
  这属于"可选实例不足"，默认重试分类器会把它当作临时状况。
- 一致性哈希走 `sd.RequestBalancer`（请求感知契约），`sd/retry` 会自动优先
  使用它。直接调用 `Endpoint()` 没有请求可用于计算键，会退化为随机选择，
  因此无键请求不会被固定到某一个实例。

重试会在每次尝试时重新选择实例，因此失败的实例不会被重试——除非负载均衡器
再次把它选出来。对一致性哈希而言这是有意的：在集合不变的前提下，同一个键
始终解析到同一个实例。

## 架构

```
Instancer  →  Endpointer  →  Balancer  →  Retry  →  Endpoint
```

每一层都可以独立使用：

```go
// Manual assembly (full control)
ep   := endpointer.NewEndpointer(instancer, factory, logger)
defer ep.Close()
lb   := balancer.NewRoundRobin(ep)
call := retry.Retry(3, 500*time.Millisecond, lb)
```

对于底层组装，缓存失效通过 `endpointer.InvalidateOnError` 配置。更高层的
`client.NewEndpoint` 构造器暴露了等价的 `client.WithInvalidateOnError` 选项。

## 重试策略

```go
// Fixed max attempts
retry.Retry(3, time.Second, lb)

// Production calls should provide an explicit retry classifier.
retry.WithClassifier(time.Second, lb,
    func(n int, err error) (keepTrying bool, replacement error) {
        return n < 5, nil
    },
	func(err error) bool {
		var retryable interface{ Retryable() bool }
		return errors.As(err, &retryable) && retryable.Retryable()
	},
)
```

默认分类器会重试显式 `Retryable() == true` 的错误以及临时的无端点状况。
未知错误和协议错误是永久性的。对于 gRPC，通过 `client.WithRetryable` 显式
传入 `integrations/grpc.Retryable`；领域写入的安全性仍由应用自行决策。

`Endpointer.Close` 会等待其更新循环结束，并关闭端点工厂返回的所有资源。
应把 closer 视为构造器契约的一部分，而不是可选的清理钩子。

## Consul 注册

```go
registrar := consul.NewRegistrar(client, logger, "my-service", "10.0.0.1", 8080,
    consul.IDRegistrarOptions("my-service-1"),
    consul.CheckRegistrarOptions(&stdconsul.AgentServiceCheck{
        HTTP:     "http://10.0.0.1:8080/health",
        Interval: "10s",
    }),
)
if err := registrar.Register(); err != nil {
    return err
}
defer func() { _ = registrar.Deregister() }()
```

`Instancer.Stop` 会取消并等待（join）活跃的 Consul 阻塞查询，因此要在端点
持有的资源关闭之后再调用它。

## 另请参阅

- `examples/sd/` — 覆盖每个 sd 组件的可运行演示
- `examples/profilesvc/client/` — 基于 Consul 的客户端示例
