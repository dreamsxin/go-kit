# 服务发现与路由

[English](service-discovery.md) | 简体中文

本章说明一个服务存在多个后端时，出站请求如何从服务发现走到具体实例。
各包指南回答“包提供什么”，本章回答如何把发现、端点创建、选择、反馈、健康检查、
重试与停机装配成一个整体。

下文的核心进程内装配在 `examples/sd` 有可运行示例（`go run ./examples/sd`），
不依赖注册中心。Provider 与长连接 bridge 是集成模板，provider 和传输层由应用拥有。

## 选择最小装配

| 需求 | 装配 |
| --- | --- |
| 固定地址或测试池 | `selector.Static` + `selector.New` |
| 动态端点与连接复用 | `Instancer` -> `Endpointer` -> `Balancer` |
| 带明确策略的重试 | 加入 `sd/retry` 或 `sd/client` |
| 最少请求或被动摘除 | `feedback.Table` + `feedback.Follow` |
| 主动存活检查 | 在 endpointer/selector 之前使用 `health.Check` |
| 调用方拥有的长连接 | 选择一次，连接结束时调用 `Done(Outcome)` |

核心不变量很简单：`Pick` 返回身份和 `Done(Outcome)`；先关闭消费者，再关闭它使用的源。

## 请求链路

```text
Consul / etcd / 其他注册中心
    -> Instancer 快照
    -> 可选的 health.Check
    -> Endpointer + Factory
    -> selector 过滤与策略
    -> Balancer.Pick
    -> 端点调用
    -> Picked.Done(Outcome)
    -> 重试或下一次选择
```

注册中心发布的是快照，不是实时负载指标。`sd.Instance` 由地址和静态元数据组成，
元数据可以是可用区、协议、版本、权重或排空状态。动态观测保留在发起调用的进程内。

各层的所有权如下：

| 层 | 负责 | 不负责 |
| --- | --- | --- |
| `sd.Instancer` | 快照与 provider watch 生命周期 | 端点连接 |
| `sd/health` | 主动探测与探测状态 | 注册中心生命周期 |
| `sd/endpointer` | endpoint factory 与资源 | 选择策略 |
| `sd/selector` | 过滤与选择策略 | 端点创建 |
| `sd/balancer` | 保留身份的端点选择 | source 所有权 |
| `sd/feedback` | 本地测量与摘除策略 | 写回注册中心 |
| `sd/retry` | 尝试策略与退避 | 业务重试安全性 |

## 核心契约

Instancer 发布按约定不可变的 `sd.Event`，并且必须实现 `Close`：

```go
type Instancer interface {
    Register(chan Event) Event
    Deregister(chan Event)
    Close() error
}
```

端点均衡器在每次选择时接收 request，并返回实例身份与完成回调：

```go
type Balancer interface {
    Pick(ctx context.Context, request any) (Picked, error)
    Close() error
}

type Picked struct {
    Instance Instance
    Endpoint endpoint.Endpoint
    Done     Done
}

type Outcome struct {
    Err     error
    Latency time.Duration
    Bytes   int64
}
```

端点返回后必须调用一次 `Done`。go-kit 提供的回调都是幂等的，因此适配层可以安全地
使用 `defer`。`Bytes` 是调用方定义的总字节数；通用 retry 无法从不透明的 request 和
response 推断它，需要由协议适配器或 bridge 填写。

只选择实例的 selector 也保留反馈契约。`selector.New` 把 strategy 绑定到 source
并返回 `Selector`；用完后关闭它，这只释放 strategy，不释放其他东西——source 以及
它背后的 instancer 仍归你所有：

```go
type Selector interface {
    Select(ctx context.Context, request any) (sd.Instance, sd.Done, error)
    Close() error
}

pool := selector.Subscribe(instancer)
defer pool.Close()

pick := selector.New(pool, selector.RoundRobin())
defer pick.Close()

instance, done, err := pick.Select(ctx, request)
if err != nil {
    return err
}
started := time.Now()
err = dial(instance.Address)
done(sd.Outcome{Err: err, Latency: time.Since(started)})
```

这对使用反馈表的策略很重要。丢弃 `done` 会让实例永久处于 in-flight 状态，导致最少
请求和健康判断随着时间变错。

## 最小装配

对于已经创建好的 Instancer，`sd/client` 是最短路径：

```go
factory := endpointer.Factory(func(instance sd.Instance) (endpoint.Endpoint, io.Closer, error) {
    return makeClientEndpoint(instance.Address), nil, nil
})

call, resources, err := client.NewEndpoint(instancer, factory, logger,
    client.WithMaxAttempts(3),
    client.WithTimeout(500*time.Millisecond),
)
if err != nil {
    return err
}
defer resources.Close()

response, err := call(ctx, request)
```

`NewEndpoint` 默认使用轮询。`resources.Close` 先关闭 balancer，再关闭 endpointer。
Instancer 仍由调用方持有，应在消费者关闭之后再关闭：

```go
defer instancer.Close() //nolint:errcheck
defer resources.Close() // 先执行
```

需要完全控制时可以手工装配：

```go
set := endpointer.NewEndpointer(instancer, factory, logger)
defer set.Close()

strategy := selector.RoundRobin()
lb := balancer.New(set, strategy)
defer lb.Close()

call := retry.Retry(3, 500*time.Millisecond, lb)
```

`retry.Retry`同时接受尝试次数上限与墙钟预算，预算优先：超时的调用会在计划中途停止。
此时的失败是一个 `retry.Error`，携带至此为止的全部尝试记录，`Final` 为 context 错误，
因此 `errors.Is(err, context.DeadlineExceeded)` 仍然匹配，而错误消息会指出失败的实例。
只有在任何一次尝试完成之前预算就到期时，才返回裸的 context 错误。

## 过滤与生命周期状态

静态元数据过滤放在 source 或 endpoint-set 边界：

```go
local := endpointer.Filter(set, sd.MetadataEquals("zone", "cn-north-1a"))
preferred := endpointer.Prefer(set, sd.MetadataEquals("zone", "cn-north-1a"))
```

`Filter` 在子集为空时失败，`Prefer` 则回退到完整集合。两者都会在读取端点集合时重新求值。

动态策略使用 `selector.Filtered`，它在每次选择时接收当前候选集：

```go
strategy := selector.Filtered(
    selector.RoundRobin(),
    sd.Keep(sd.Serving()),
)
lb := balancer.New(set, strategy)
```

`sd.StateKey` 是 `"state"`；`sd.StateDraining` 表示实例仍在注册，但不再接受新工作。
服务发现不会关闭已有连接。排空是注册属性，不是反馈采样。

## 选择策略

三种形状不同的问题，对应三套契约。路由代码出错，通常就出在把它们混为一谈。

单实例选择——`selector.Strategy`，返回一个下标以及这次调用的 `Done` 回调：

| 需求 | 策略 | 说明 |
| --- | --- | --- |
| 均匀分布 | `selector.RoundRobin` | 一个原子计数器 |
| 独立随机分布 | `selector.Random` | 避免多客户端步调一致 |
| 静态容量权重 | `selector.WeightedRandom` | 0 权重可排空实例 |
| 测量或上报分数 | `selector.Scored` | 最高分胜出 |
| 本地在途公平 | `table.LeastRequest` | power of two choices；裸的 `selector.LeastRequest` 接受任意 `LoadFunc` |
| 请求亲和 | `selector.ConsistentHash` | 每次选择都能拿到 request |

候选排序——`selector.Ranker`，返回有序的 `[]sd.Instance`：

| 需求 | 组件 | 说明 |
| --- | --- | --- |
| 给自行拨号的调用方一份候选清单 | `selector.NewRanker` | 最优在前，同分按地址排序 |

`Scored` 与 `NewRanker` 共用同一个带请求的评分契约：

```go
type ScoreFunc func(
    ctx context.Context,
    request any,
    instance sd.Instance,
) (score float64, ok bool)
```

返回 `ok == false` 会排除实例。纯实例评分可以忽略 `ctx` 和 `request`；也可以用它们
表达按租户、地域或操作类型变化的路由策略。
`ScoreFunc` 在选择热路径上对每个候选执行一次，因此应保持有界且只做本地计算：不要执行
网络 I/O，也不要在持有应用锁时等待。
`NaN` 会被视为不可用分数；正负无穷仍是有效值。

装饰器——它们包装上面的组件，本身不回答"选哪个"：

| 需求 | 组件 | 说明 |
| --- | --- | --- |
| 每次选择前排除候选 | `selector.Filtered` | 接收 `sd.InstanceFilter` |
| 让冷实例逐步承接流量 | `selector.SlowStart` | 装饰 `WeightFunc` |

`Ranker` 有意不做成 `Strategy`。策略为一次调用指名实例，并通过 `Done` 拥有这次
调用的生命周期；ranker 返回的是候选，什么都不拥有，因为没有单次调用可归属。
硬塞成 `Pick(...) (index, done, error)` 会让 `Done` 失去定义——它对应整个列表、
其中某一个，还是后续每一次拨号？另外轮询与一致性哈希也无法排序：它们定义的是
"下一个是谁"，而不是所有实例上的全序。

端点构造器是同一批 selector 策略的薄封装：`balancer.NewRoundRobin`、`NewRandom`、
`NewWeightedRandom`、`NewScored` 与 `NewConsistentHash`。组合过滤、反馈或自定义策略时，
使用 `balancer.New(set, strategy)`。

## 自定义中间件与组件

内置组件都是组合点，不是封闭实现。调用方可以通过各自最窄的契约包装或替换它们：

| 层 | 扩展点 | 常见中间件 |
| --- | --- | --- |
| selector | `selector.Strategy` | 日志、配额、自定义亲和 |
| 排序 | `selector.Ranker` | 缓存、候选数量限制、请求策略 |
| 候选集合 | `sd.InstanceFilter` | 标签、租户、摘除 |
| 端点选择 | `sd.Balancer` | 追踪、指标、准入控制 |
| 端点创建 | `endpointer.Factory` | 传输配置、TLS、连接池 |
| 主动健康 | `health.Probe` | 认证、探测指标、自定义协议 |
| 重试 | `retry.Callback` / `retry.Classifier` | 幂等性与协议策略 |

Strategy 中间件必须原样转发被选下标、内部回调以及 `Close`——一行写法是
`selector.CloseStrategy(inner)`；漏掉它内部策略就永远关不掉，因为
`selector.New` 与 `balancer.New` 只看得见最外层策略。Balancer 中间件必须转发
`Picked.Instance` 与 `Picked.Endpoint`，最多包装一次 `Picked.Done`，并在 `Close`
时委托内部 balancer。Factory 或 Probe 包装器应保留原始错误与 closer 语义。这样自定义
组件仍可与同一张反馈表和 retry executor 协作。

需要按请求评分时，实现统一的 `selector.ScoreFunc`，不要再造第二套请求感知策略接口：

```go
score := selector.ScoreFunc(func(ctx context.Context, request any, instance sd.Instance) (float64, bool) {
    tenant, _ := request.(Request)
    if tenant.Region != "" && instance.Metadata["region"] != tenant.Region {
        return 0, false
    }
    return localLoadScore(ctx, instance), true
})
strategy := selector.Filtered(selector.Scored(score), sd.Keep(sd.Serving()))
lb := balancer.New(set, strategy)
```

只按实例评分时忽略 `ctx` 与 `request` 即可；自定义代码不需要重新实现所有内置策略，
也能使用外围装饰器。
请求/响应级 endpoint 中间件使用 `endpoint.Middleware` 与 `endpoint.Builder`；本节契约
用于包装服务发现组件，必须保留回调与 `Close` 语义。

## 反馈与被动摘除

`feedback.Table` 是进程内的记分板，记录 EWMA 时延、错误率、字节数、在途请求，以及
地址进入表的时间。它不会把采样写回注册中心。

需要多个策略对同一份流量达成一致时，让它们共享一张表：

```go
table := feedback.NewTable()
ejector := feedback.NewEjector(table, feedback.EjectionPolicy{
    MaxErrorRate: 0.5,
    MinSamples:   5,
})

following := feedback.Follow(instancer, table, ejector)
defer following.Close()

strategy := table.Wrap(selector.Filtered(
    selector.Scored(table.Score()),
    ejector.Filter(),
))
lb := balancer.New(set, strategy)
```

真正把调用写进表里的是 `table.Wrap`：它在 `Pick` 时累加在途、在 `Done` 时记录结果。
少了它，表里永远没有数据，所有实例分数相同，`selector.Scored` 退化成随机选择。没有
采样的实例拿到的是最高分——它得先拿到调用才能被测量；如果这一批冷启动流量太猛，配套
使用基于 `table.FirstSeen()` 的 `selector.SlowStart`。

`feedback.Follow` 必须针对完整 discovery 快照保留状态，不要针对已经过滤过的候选集保留；
否则会抹掉导致实例被摘除的测量值。

这里也包括包裹同一个 instancer 的 `health.Check`——要传 `instancer`，不要传 `checked`。
checker 会撤下它判定为不健康的实例，而 retainer 分不清"被撤下"和"已注销"：ejector 会
丢掉它的摘除记录、table 会丢掉它的测量值，于是探测一恢复，实例就以一份干净的记录回到
池里，主动与被动健康检查互相抵消。`health.Check` 仍然装饰喂给 endpointer 或 selector
的那个 instancer，只有 `Follow` 需要未装饰的原始 instancer。

Ejector 按错误率、时延和在途阈值移除不健康候选。摘除在 `BaseDuration` 后到期，连续
违规地址的窗口会翻倍，直到 `MaxDuration`，并清理导致摘除的测量。`MaxEjectionPercent`
默认是 50%；整池看起来都坏时进入 panic 模式，保留未摘除的候选集合。

最少请求也是同样的组合，只是没有 endpoint 专用构造器：

```go
strategy := table.LeastRequest(selector.WithChoices(2))
lb := balancer.New(set, strategy)
```

外部负载报告直接使用 `selector.Scored`。需要候选短名单时使用 `selector.NewRanker`；
两者都会通过统一的 `ScoreFunc` 接收 request：

```go
pool := selector.Subscribe(instancer)   // 或 selector.Static(...)
defer pool.Close()

ranker := selector.NewRanker(pool, table.Score(), ejector.Filter())
top, err := ranker.Rank(ctx, request, 3)
```

## 慢启动

新实例通常是冷的：缓存为空、JIT 未预热、连接池也未建立。`selector.SlowStart` 装饰
权重函数，在一个时间窗口内逐步恢复到完整权重：

```go
weight := selector.SlowStart(
    selector.MetadataWeight("weight", 1),
    table.FirstSeen(),
    30*time.Second,
)
strategy := table.Wrap(selector.WeightedRandom(weight))
```

权重至少为 1，因此预热实例不会完全饿死；权重为 0 不会被拉升，因为 0 的含义就是
"永远不要选我"。`table.FirstSeen()` 报告实例进入表的时间，所以让表跟随 discovery——
`feedback.Follow`——爬坡就从实例加入服务的那一刻开始。否则表只会在首次调用时才知道
这个实例，而没有被调用过的实例是未知的，慢启动会一直按全新实例处理。

## 主动健康检查

被动反馈看不见没有流量的实例。`health.Check` 装饰 Instancer，并以有界并发直接探测地址：

```go
checked := health.Check(instancer,
    health.TCPProbe(2*time.Second),
    health.WithInterval(10*time.Second),
    health.WithUnhealthyThreshold(3),
)
defer checked.Close()

set := endpointer.NewEndpointer(checked, factory, logger)
defer set.Close()
```

`health.HTTPProbe` 将小于 400 的响应视为健康。阈值按连续结果计算。默认情况下，实例在
首次探测完成前按健康处理；`WithInitiallyHealthy(false)` 会让每个尚未完成首次探测的实例
暂不发布，已经通过探测的实例仍可继续服务。如果所有实例都已经产生结果但没有一个通过，
checker 会发布未检查集合，而不是因为探针故障让服务整体失效；`WithFailOpen(false)` 改为
发布空集合，适用于"打到死实例比打不到实例更糟"的调用方。自定义 Probe 必须响应 context
取消，因为 `Checker.Close` 会等待正在执行的探测。

往哪个方向失败是需要明确做的决策，而不是靠不配置默认接受下来的：

- 读路径、缓存、容错代理：fail open。探针坏掉的可能性大于所有后端同时挂掉，而发布
  空集会把一次监控故障变成一次服务故障。
- 写请求、非幂等操作、正在迁移的后端：fail closed。调用方拿到 `sd.ErrNoEndpoints`，
  可以重试或降载，这是可恢复的；而写入被执行两次不可恢复。

同一个取舍在下一层表现为 `feedback.EjectionPolicy.MaxEjectionPercent`，两处的答案应当
一致：在这里选择 fail closed 的服务，也没有理由允许被动摘除把实例池清空。

## Provider

Consul 与 etcd 都满足 `sd.Instancer` 契约——各自的 `contract.go` 在编译期断言这一点——并且不会进入通用服务发现包的
依赖闭包：

```go
consulInstancer := consul.NewInstancer(consulClient, logger, "users", true)
defer consulInstancer.Close() //nolint:errcheck

etcdInstancer := etcd.NewInstancer(etcdClient, logger, "users")
defer etcdInstancer.Close() //nolint:errcheck
```

Consul 使用通过健康检查的服务条目，并可通过服务 metadata 携带静态标签。etcd 为每个实例
保存一个带租约的注册键，负责续租，并在 watch 断开后从上次 revision 恢复。注册选项与运维
要求见各 provider 指南。

## 长连接

L4 均衡器在连接建立时选择一次，长连接隧道上的后续请求都会固定到同一个后端。对于 bridge
或 gateway，更有价值的 Outcome 是连接持续时间、拨号错误和转发字节数，而不只是一次 RPC 时延：

```go
picked, err := lb.Pick(ctx, dialRequest)
if err != nil {
    return err
}
started := time.Now()
conn, err := dial(picked.Instance.Address)
if err != nil {
    picked.Done(sd.Outcome{Err: err, Latency: time.Since(started)})
    return err
}

bytes, err := proxy(conn)
picked.Done(sd.Outcome{
    Err:     err,
    Latency: time.Since(started),
    Bytes:   bytes,
})
```

端点层不假设连接语义；bridge 适配层负责判断连接何时结束并提交 Outcome。

## 停机与排障

关闭顺序应当是先关闭消费者，再关闭它们的数据源：

```text
retry 停止调用
 -> balancer.Close
 -> endpointer.Close
 -> selector / feedback follower 关闭
 -> health.Check.Close
 -> Instancer.Close
```

常见症状：

| 症状 | 检查 |
| --- | --- |
| 没有端点 | discovery 快照、factory 错误、过滤器、0 权重 |
| 一个实例收到全部流量 | 是否丢弃 `Done`、in-flight 表是否过期、分数是否排除了其他实例 |
| 所有实例消失 | 主动探针配置、摘除 panic 阈值、服务发现错误处理 |
| 重试始终落到同一后端 | 一致性哈希本来就是固定的；需要故障转移时换策略 |
| 停机卡住 | 自定义 Probe 或 endpoint closer 是否忽略取消或耗时过长 |
| 表随部署持续增长 | 用完整 discovery instancer 调用 `feedback.Follow` |

重试分类仍由应用决定。`sd/retry` 会为已完成的失败记录
`retry.Attempt{Address, Err, Latency}`，但不会替应用判断业务操作是否可以重复执行。
