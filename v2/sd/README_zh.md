# sd - 服务发现

[English](README.md) | 简体中文

根 `sd` 包拥有协议无关的契约——`Instance`、`Event`、`Instancer`、`Registrar`、
`Balancer`、`Match` 与 `ErrNoEndpoints`——以及各层共用的标签读取函数。具体组件
位于职责聚焦的子包中，可以独立使用：`sd/instance`、`sd/endpointer`、
`sd/selector`、`sd/balancer`、`sd/retry`、`sd/feedback`、`sd/client`。

## 快速开始（无需 Consul）

```go
import (
    "github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/client"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
    "github.com/dreamsxin/go-kit/v2/sd/instance"
)

factory := endpointer.Factory(func(inst sd.Instance) (endpoint.Endpoint, io.Closer, error) {
	return makeClientEndpoint(inst.Address), nil, nil
})

// In-memory instancer — perfect for tests and local dev
cache := instance.NewCache()
cache.Update(sd.Event{Instances: sd.Addresses("host1:8080", "host2:8080")})

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
    instancer.Close()
    return err
}
defer instancer.Close() //nolint:errcheck
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

## 实例元数据

一个实例由地址加静态标签组成：

```go
type Instance = struct {
	Address  string
	Metadata map[string]any
}
```

元数据承载的是注册中心本就适合存放的信息——可用区、版本、协议、能力、权重、
租户。它**不是**上报实时负载的通道：每采样一次指标就写一次注册中心会打爆
catalog，而且每个消费者读到的仍然是过期数字。需要实时信号的均衡器应在进程内
自行度量，参见下文的 `NewLeastRequest`。

注册中心交回来的标签都是字符串，而断言通常用带类型的字面量书写。
`sd.Metadata*` 系列读取函数会做类型归一，因此 `5` 与 `"5"` 都能匹配：

```go
weight, ok := sd.MetadataInt(inst.Metadata, "weight")
zone, ok := sd.MetadataString(inst.Metadata, "zone")
tls, ok := sd.MetadataBool(inst.Metadata, "tls")
```

标签同样会传到 factory，因为"如何连接"是传输层的决策，而注册信息可以左右它：

```go
factory := endpointer.Factory(func(inst sd.Instance) (endpoint.Endpoint, io.Closer, error) {
	if secure, _ := sd.MetadataBool(inst.Metadata, "tls"); secure {
		return newTLSClient(inst.Address)
	}
	return newPlainClient(inst.Address)
})
```

在 Consul 中，标签通过服务的 `Meta` 字段往返：`consul.MetaRegistrarOptions`
负责写入，instancer 再把它们还原成 `Instance.Metadata`。Tags 不进入元数据——
它是集合而非键值对，且 `NewInstancer` 已经在服务端按 tag 过滤。

## 过滤实例集合

选择与过滤是两层，所以"就近路由"是组合出来的，而不是给每个策略都做一个
可用区变体。`endpointer` 负责装饰实例集合，任何均衡器都能架在结果之上：

```go
set := endpointer.NewEndpointer(instancer, factory, logger)
defer set.Close()

// Envoy NO_FALLBACK：只用本可用区，没有就报无可用端点。
local := endpointer.Filter(set, sd.MetadataEquals("zone", "cn-north-1a"))

// Envoy ANY_ENDPOINT：优先本可用区，为空时回退到全集——
// 需要就近但又不能因此故障时，通常选这个。
preferred := endpointer.Prefer(set, sd.MetadataEquals("zone", "cn-north-1a"))

lb := balancer.NewRoundRobin(preferred)
```

断言放在根 `sd` 包里，因为两层都要读它：`sd.MetadataEquals`、`sd.MetadataIn`、
`sd.MetadataMatches`、`sd.HasMetadata`，以及 `sd.And` / `sd.Or` / `sd.Not`。
实例层通过 `selector.Filter` 和 `selector.Prefer` 使用同一批断言。

```go
sd.And(
	sd.MetadataEquals("version", "v2"),
	sd.MetadataIn("zone", "a", "b"),
	sd.Not(sd.HasMetadata("draining")),
)
```

`Filter` 与 `Prefer` 在每次选择时重新求值，因此改了标签的实例会进出子集
而不需要重连。关闭子集会连带关闭它包装的来源。

## 选择策略

策略由 `sd/selector` 拥有，`sd/balancer` 负责把它们应用到端点集合上。目前内置
六种，命名对齐 Envoy/gRPC 的同名实现。

| 策略 | 均衡器 | 策略（实例层） | 依据 |
|----------|--------|----------------|------|
| 轮询 | `balancer.NewRoundRobin` | `selector.RoundRobin` | 原子计数器 |
| 随机 | `balancer.NewRandom` | `selector.Random` | 均匀抽样 |
| 加权随机 | `balancer.NewWeightedRandom` | `selector.WeightedRandom` | 每个实例的权重 |
| 评分 | `balancer.NewScored` | `selector.Scored` | 调用方给出的分数，最高者胜 |
| 最少请求 | `balancer.NewLeastRequest` | `selector.LeastRequest` | 在途请求数，二选一（P2C） |
| 一致性哈希 | `balancer.NewConsistentHash` | `selector.ConsistentHash` | 请求键的哈希 |

```go
// 均匀随机
client.WithBalancer(func(set endpointer.InstanceEndpointer) sd.Balancer {
	return balancer.NewRandom(set)
})

// 从注册中心取权重：容量是注册标签，权重为 0 即可摘除实例，
// 无需等待服务发现把它摘掉。
client.WithBalancer(func(set endpointer.InstanceEndpointer) sd.Balancer {
	return balancer.NewWeightedRandom(set, balancer.MetadataWeight(balancer.DefaultWeightKey, 1))
})

// 最少请求：随机取两个候选，在途请求少的胜出。表负责记录调用结果，
// 传 nil 则使用私有表。
client.WithBalancer(func(set endpointer.InstanceEndpointer) sd.Balancer {
	return balancer.NewLeastRequest(set, table, balancer.WithChoices(2))
})

// 评分：跟随本进程并未亲自度量的负载信号。
client.WithBalancer(func(set endpointer.InstanceEndpointer) sd.Balancer {
	return balancer.NewScored(set, func(instance sd.Instance) (float64, bool) {
		report, ok := reports.Latest(instance.Address) // 你的表，你的协议
		if !ok || report.Saturated() {
			return 0, false // false 就是硬过滤
		}
		return report.Score(), true
	})
})

// 一致性哈希：只要实例还在集合中，同一租户的请求就始终落到同一实例。
client.WithBalancer(func(set endpointer.InstanceEndpointer) sd.Balancer {
	return balancer.NewConsistentHash(set, func(_ context.Context, request any) string {
		return request.(*GetProfileRequest).TenantID
	}, balancer.WithReplicas(200))
})
```

`WeightFunc` 的签名是 `func(sd.Instance) int`，所以权重来源不限：
`MetadataWeight` 读注册标签，闭包也可以读本地表。

加权、评分、最少请求与哈希策略需要知道端点由哪个实例生成，因此它们接收
`endpointer.InstanceEndpointer` 而不是更窄的 `endpointer.Endpointer`。
`endpointer.NewEndpointer` 返回的就是前者。

选型之前有四点行为值得了解：

- 当端点存在但所有权重都不大于 0 时，加权随机返回 `sd.ErrNoEndpoints`。
  这属于"可选实例不足"，默认重试分类器会把它当作临时状况。所有实例都被排除时，
  `Scored` 返回同一个错误。
- 最少请求从 `feedback.Table` 读取在途深度，并把结果记回同一张表，
  因此"依据什么选"和"记录了什么"不会脱节。它不向注册中心上报，
  同一张表还能同时供评分和被动健康检查使用。两层都可用：
  `selector.LeastRequest` 接受任意 `LoadFunc`。
- `Scored` 是外部信号的接入点——实例推送的报告、ORCA/LRS 式的带外上报、你自己的
  指标表。这类信号至少陈旧一个上报周期，这是通道的固有属性而非缺陷。本进程在
  数据路径上时，优先用最少请求。
- 所有均衡器都使用 `Pick(ctx, request)` 并返回 `sd.Picked`，其中保留被选中的
  `Instance`、`Endpoint` 以及接收 `sd.Outcome` 的 `Done` 回调。一致性哈希直接
  从这条公共契约读取请求，不再需要第二个请求感知接口。

重试会在每次尝试时重新选择实例，因此失败的实例不会被重试——除非负载均衡器
再次把它选出来。对一致性哈希而言这是有意的：在集合不变的前提下，同一个键
始终解析到同一个实例。

## 只选实例，不建端点

有些调用方只需要一个地址：自己去拨号的代理，或者回答"我该连哪台"的 API。
它们装配 `sd/selector`，完全不碰端点工厂，因此不会为没人调用的实例建连接。

```go
import "github.com/dreamsxin/go-kit/v2/sd/selector"

instances := selector.Subscribe(instancer)   // 或 selector.Static(...)
defer instances.Close()

pool := selector.Prefer(instances, sd.MetadataEquals("zone", "cn-north-1a"))
pick := selector.New(pool, selector.WeightedRandom(selector.MetadataWeight("", 1)))

instance, err := pick.Select(ctx, request)   // sd.Instance：地址加标签
```

请求始终会传给 `selector.Select` 与策略，因此按键和不按键策略共用一条路径。

`Subscribe` 维护最新快照，并且和 endpointer 一样，在服务发现故障期间继续提供
最后一份可用快照；想让它在宽限期后丢弃，加上 `selector.InvalidateOnError(d)`。

## 自定义策略

实现 `selector.Strategy`，两层都能用它：`selector.New` 做实例选择，
`balancer.New` 做端点选择。

```go
type Strategy interface {
	Pick(ctx context.Context, request any, instances []sd.Instance) (index int, done sd.Done, err error)
}
```

快照由外部传入，因此策略只持有自己的状态——计数器、哈希环、评分表——绝不会对
调用方没见过的快照做选择。实现必须支持并发使用：一个策略服务于所有调用方，
而 `sd/retry` 每次尝试都会在新的 goroutine 上做选择。没有可选实例时返回
`sd.ErrNoEndpoints`。如果策略维护本地反馈状态，返回 `done` 回调接收调用的
`sd.Outcome`。

```go
lb := balancer.New(set, myStrategy{})         // 可直接被 sd/client 与 sd/retry 使用
pick := selector.New(instances, myStrategy{}) // 不需要端点
```

若要从调用本身采集反馈——时延、失败、在途深度——请使用 `sd/feedback.Table`。
一张进程内的表服务于所有策略：既能评分，也能统计在途，还能摘除：

```go
table := feedback.NewTable()

// 让表的规模等于服务规模，而不是部署历史的规模：
// 离开服务发现的地址，其测量数据一并丢弃。
following := table.Follow(instancer)
defer following.Close()

healthy := table.Healthy(feedback.HealthPolicy{MaxErrorRate: 0.5, MinSamples: 5})
strategy := table.Wrap(selector.Filtered(selector.Scored(table.Score()), healthy))

lb := balancer.New(set, strategy)
defer lb.Close()

picked, err := lb.Pick(ctx, request)
if err != nil { return err }
started := time.Now()
response, err := picked.Endpoint(ctx, request)
picked.Done(sd.Outcome{Err: err, Latency: time.Since(started)})
_ = response
```

`sd.Match` 是针对单个实例的静态标签谓词；被动健康检查是 `sd.InstanceFilter`，
它拿到的是整个候选集——因为"某个实例能不能摘"取决于"还有多少个也在失败"。
用 `sd.Keep(match)` 可以把标签谓词放进同一个过滤器列表。`selector.Filtered`
每次选择都会执行过滤器，并把结果映射回调用方自己的快照；而 `endpointer.Filter`
仍然是端点集合静态视图的正确工具。

`HealthPolicy.MaxEjectionPercent` 默认 50：当超过一半候选看起来不健康时，
一个都不摘——整池同时失败，通常意味着共享依赖出问题或阈值设得太紧，
而不是多数实例真的坏了。Envoy 称之为 panic 模式。

## 架构

```
Instancer → [selector.Filter] → Selector                → 实例
Instancer → Endpointer → [Filter] → Balancer → Retry    → 端点
                                        ↑ Outcome ↓
                                     feedback.Table
```

两条装配路径，共用一套策略。每一层只有一件事要做：instancer 监听注册中心，
endpointer 把实例变成端点（factory 在这里决定如何连接），可选的过滤器收窄
集合，策略挑一个，retry 决定要不要再挑一次，调用的结果再回灌进下一次选择
所读的表。不自己发请求的调用方到第一行为止。

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
    consul.MetaRegistrarOptions(map[string]string{
        "zone":    "cn-north-1a",
        "version": "v2",
        "weight":  "5",
    }),
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

`MetaRegistrarOptions` 是合并而非覆盖，因此可以多次调用、由不同配置来源各自
贡献标签。注意不要把实时负载放进来：这里上报静态标签，负载交给均衡器度量。

`Instancer.Close` 会取消并等待（join）活跃的 Consul 阻塞查询，因此要在端点
持有的资源关闭之后再调用它。

## 另请参阅

- `examples/sd/` — 覆盖每个 sd 组件的可运行演示
- `examples/profilesvc/client/` — 基于 Consul 的客户端示例
