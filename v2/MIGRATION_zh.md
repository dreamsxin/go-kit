# 升级说明

[English](MIGRATION.md) | 简体中文

go-kit v2 遵循语义化版本。本页记录当前版本所需的升级动作；每个版本的
完整变更列表见 [CHANGELOG.md](CHANGELOG_zh.md)。

## 从旧版 go-kit（v0/v1 风格）迁移

基于旧版 go-kit 风格（`github.com/go-kit/kit`：每路由
`transport/http.NewServer` + 解码/编码函数对、`endpoint.Set` 结构、
`NewGRPCServer`/`NewGRPCClient` 装配块）的代码库，构造映射如下：

| 旧版构造 | go-kit v2 等价物 |
| --- | --- |
| 每路由 `httptransport.NewServer(ep, decode, encode, opts...)` | `kit.HandleJSONTyped` / `kit.HandleJSONWithMiddleware`；自定义编解码用 `server.NewServer` |
| 每路由解码/编码函数对 | 类型化处理器无需编解码；自定义线上格式用 `server.RawBodyCodec` |
| `endpoint.Set` 结构装配 | 在 `kit.HTTP` 组件上注册路由；microgen 生成的 `endpoints.go` |
| `NewGRPCServer` / grpc handler 结构 | `integrations/grpc` 服务端 + `transport.Binding`（一个端点同时服务 HTTP 与 gRPC） |
| `NewGRPCClient` 端点块 | `integrations/grpc/client`；`sd/client` 一次装配发现、均衡与重试 |
| `jwt.HTTPToContext()` / `jwt.GRPCToContext()` | `security` 主体契约 + 应用在协议边界自有的凭据提取 |
| `opentracing.HTTPToContext` / `TraceClient` | `endpoint.TracingMiddleware`（W3C）或 `observability/otel` |
| `ratelimit.NewErroringLimiter` | 内置 `endpoint.RateLimitMiddleware` |
| `switch err.Error() {...}` 状态码映射 | `apperror` 分类；各传输映射种类；自定义状态用 `JSONErrorEncoderWithKindMapper` |
| 响应中的 `Err string` 字段（错误字符串化传输） | 直接返回错误；proto 消息必须携带失败时用 `endpoint.Failer` |
| 手写 proto↔领域映射器 | 生成传输（`microgen` 从 `.proto` 生成） |

旧版风格易犯、而 v2 从设计上消除的坑：

- **中间件静默漂移**：每方法中间件靠复制粘贴或注释开关应用，会让路由在毫无信号的情况下丢失限流或追踪。v2 中间件在组件级一次声明或按路由显式声明，链可经 `endpoint.Builder.Describe()` 自省。
- **错误分类丢失**：字符串化的错误跨线丢失种类；`apperror` 种类在 HTTP 与 gRPC 间保留。
- **映射器漂移**：手写 proto↔领域转换器随时间与 schema 漂移；生成的编解码与契约保持同步。

推荐迁移顺序：逐个服务迁移；先从类型化 JSON 处理器（`kit`）入手，再把错误字符串约定替换为 `apperror`，然后把重复的 HTTP/gRPC 装配收敛到 `transport.Binding`，最后让 `microgen` 接管再生成的传输。

## 升级到未发布版本（传输层与 JSON 构造器）

HTTP/gRPC 服务端路径上有两项破坏性变更。

### `endpoint.Failer` 现在生效

HTTP 与 gRPC 服务器会检查响应是否有 `Failed() error`。若它返回非 nil，响应被丢弃、
改为编码该错误——获得与正常返回错误完全相同的状态码、错误码与日志。

只有当你的响应类型恰好已经带有 `Failed() error` 方法时，这才影响你。此前该方法被
忽略、响应会被序列化成一次成功；现在它决定响应内容。如果你并不是想实现 go-kit 的
这个契约，请给方法改名：

```go
// 这个类型现在会让每个 Err 非 nil 的响应短路。
type CreateResponse struct {
	ID  string `json:"id"`
	Err error  `json:"-"`
}

func (r CreateResponse) Failed() error { return r.Err }
```

`Failer` 存在的意义是服务那些无法返回 error 的签名——生成的批处理 handler、回调
适配器。它不是"用 200 附带错误字段"的手段：那种场景请正常返回 error。

### `NewStrict*` JSON 构造器更名

```go
// 之前
httpserver.NewStrictJSONEndpoint[Req](ep, maxBodyBytes)
httpserver.NewStrictJSONServer[Req](fn, maxBodyBytes)
httpserver.NewStrictTypedJSONServer[Req, Resp](fn, maxBodyBytes)

// 之后
httpserver.NewJSONEndpointWithBodyLimit[Req](ep, maxBodyBytes)
httpserver.NewJSONServerWithBodyLimit[Req](fn, maxBodyBytes)
httpserver.NewTypedJSONServerWithBodyLimit[Req, Resp](fn, maxBodyBytes)
```

行为未变。旧名字暗示了一种从未存在的解码差异：所有 JSON 入口都会拒绝未知字段与
尾随数据，这三个构造器的唯一区别只是多接一个显式的请求体上限。放宽严格性仍然只有
`NewJSONEndpointWithDecodeOptions` 一条路。

### 500 响应不再携带错误文本

三个 HTTP 错误编码器在状态码为 500 时统一回答 `"Internal Server Error"`。此前未分类
错误包住的内容——驱动报错、上游响应体——调用方是能读到的。

错误对象本身没变，日志里仍然完整。如果某条消息确实是给调用方看的，就明确声明：

```go
// PublicMessage() 在任何状态码下都生效，包括 500。
type rateLimited struct{ error }

func (rateLimited) StatusCode() int       { return http.StatusTooManyRequests }
func (rateLimited) PublicMessage() string { return "slow down" }
```

主动设置的、500 之外的 5xx——比如 `StatusCode()` 返回 503——保留自己的消息。只有 500
这个未分类错误的落点会被脱敏。

gRPC 侧同理：`codes.Internal` 回答 `"internal error"`。

## 升级到未发布版本（sd 实例元数据）

服务发现现在携带标签，而不只是地址。需要改三处源码，都很机械。

1. `sd.Event.Instances` 的类型变为 `[]sd.Instance`。没有标签要上报时，用
   `sd.Addresses` 构造快照：

   ```go
   // 之前
   cache.Update(sd.Event{Instances: []string{"host1:8080", "host2:8080"}})

   // 之后
   cache.Update(sd.Event{Instances: sd.Addresses("host1:8080", "host2:8080")})

   // 带标签
   cache.Update(sd.Event{Instances: []sd.Instance{
       {Address: "host1:8080", Metadata: map[string]any{"zone": "a", "weight": 5}},
   }})
   ```

2. `endpointer.Factory` 收到的是整个实例，因此可以遵循决定"如何连接"的标签：

   ```go
   // 之前
   func(instance string) (endpoint.Endpoint, io.Closer, error) {
       return newClient(instance), nil, nil
   }

   // 之后
   func(instance sd.Instance) (endpoint.Endpoint, io.Closer, error) {
       return newClient(instance.Address), nil, nil
   }
   ```

3. `balancer.NewWeightedRandom` 接收 `balancer.WeightFunc`：

   ```go
   // 之前
   balancer.NewWeightedRandom(set, func(instance string) int { return weights[instance] })

   // 之后
   balancer.NewWeightedRandom(set, func(instance sd.Instance) int { return weights[instance.Address] })

   // 或者直接读注册方上报的权重
   balancer.NewWeightedRandom(set, balancer.MetadataWeight(balancer.DefaultWeightKey, 1))
   ```

自定义的 `sd.Instancer` 实现改为发布 `[]sd.Instance`；除此之外字段结构不变，
类型别名仍是匿名结构体，因此 provider 模块可以继续在结构上镜像它们，无需依赖
核心模块。

如果你已经在用子集过滤，断言移到了根 `sd` 包以便新的实例层共用，同时两个装饰器
改名，与 `selector.Filter` / `selector.Prefer` 对齐：

```go
// 之前
endpointer.Subset(set, endpointer.MetadataEquals("zone", "a"))
endpointer.PreferSubset(set, endpointer.MetadataEquals("zone", "a"))

// 之后
endpointer.Filter(set, sd.MetadataEquals("zone", "a"))
endpointer.Prefer(set, sd.MetadataEquals("zone", "a"))
```

`Match`、`MetadataIn`、`MetadataMatches`、`HasMetadata`、`And`、`Or`、`Not`
同样移动。行为不变：`Filter` 无匹配即失败，`Prefer` 无匹配时回退到全集。

选择契约现在统一。`sd.Balancer` 暴露
`Pick(ctx, request) (sd.Picked, error)` 与 `Close`；删除 `Endpoint`、
`EndpointFor` 和 `RequestBalancer`。`sd.Picked` 携带被选中的实例、端点以及
每次调用结束后必须执行的 `Done(sd.Outcome)`。`selector.Strategy` 同样统一为
`Pick(ctx, request, instances)`，不再有 `RequestStrategy`/`PickFor` 双接口。
`retry.Error.Attempts` 会记录每次失败尝试的地址与时延。`sd.Instancer` 现在也
要求 `Close() error`。

`sd/feedback.Table` 是进程内结果表，记录 EWMA 时延、错误率、字节数、在途请求
以及首次出现时间。使用 `Table.Wrap`、`Table.Score`、`Table.LeastRequest` 或
`Table.FirstSeen` 将本地观测接入策略选择；它不会把实时信号写回注册中心。
被动摘除是独立组件 `feedback.Ejector`。

接入时有三处必须改对：

```go
// 之前
lb := balancer.NewLeastRequest(set, balancer.WithTable(table))
strategy := table.Wrap(selector.Scored(table.Score()))
healthy := table.Healthy(policy)          // 曾经是 sd.Match
following := table.Follow(instancer)

// 之后
lb := balancer.New(set, table.LeastRequest())             // 表始终由调用方持有
ejector := feedback.NewEjector(table, feedback.EjectionPolicy{
	MaxErrorRate: 0.5,
	MinSamples:   5,
})
following := feedback.Follow(instancer, table, ejector)   // 丢弃被替换的地址
defer following.Close()
strategy := table.Wrap(selector.Filtered(selector.Scored(table.Score()),
	ejector.Filter()))                                     // 现在是 sd.InstanceFilter
```

`selector.ScoreFunc` 现在接收 `(context.Context, request, sd.Instance)`，并由
`selector.Scored`、`selector.NewRanker` 与 `feedback.Table.Score` 共用。只按实例评分
时可以忽略前两个参数，按请求变化的评分也不需要另一套请求感知接口。

`balancer.NewLeastRequest` 已删除，`balancer.LoadFunc`、`LeastRequestOption`、
`DefaultChoices`、`WithChoices` 这几个别名也一并删除，请直接用 selector 里的
原件。端点层现在完全不 import `sd/feedback`，因此纯轮询的装配不会把反馈层
编译进来。最少请求的组装方式与其他所有依赖反馈的策略一致：
`balancer.New(set, table.LeastRequest(...))`。这同时去掉了旧的 `table == nil`
简写——它造出一张调用方拿不到句柄的表，于是永远无法 `Retain`，在任何跨越滚动
发布的进程里都是一张无界的 map。

`Ejector.Filter` 之所以是 `sd.InstanceFilter`，是因为它的摘除上限是对整个候选集的
判定。务必调用 `feedback.Follow`——或自己调用 `Table.Retain(snapshot)` 与
`Ejector.Retain(snapshot)`——否则表会为见过的每个地址各留一条记录，并随每次
滚动发布持续增长。

摘除不再是永久的。一次摘除在 `EjectionPolicy.BaseDuration` 后到期（按该地址
历史摘除次数翻倍，上限 `MaxDuration`），到期时会重置该地址的测量值。如果你要
自己实现同类策略，记得一并重置测量值：拿不到流量的 EWMA 永远不会恢复，只放回
而不清掉当初导致摘除的数据，下一次选择就会立刻再摘一遍。

新增的可选层，都不影响既有装配：

```go
// 主动探测——装饰 instancer，下游一律不用改。
checked := health.Check(instancer, health.TCPProbe(2*time.Second))
defer checked.Close()

// 排序而不是选一个，适合选路服务。第一个参数是 selector.Source，不是切片。
pool := selector.Subscribe(checked)
defer pool.Close()
top, err := selector.NewRanker(pool, table.Score(), ejector.Filter()).Rank(ctx, request, 3)

// 慢启动，避免冷实例一上来就赢下每一次比较。
weight := selector.SlowStart(selector.MetadataWeight("weight", 1), table.FirstSeen(), 30*time.Second)
```

排空现在有了明确的标签约定：把注册信息里的 `sd.StateKey` 设为
`sd.StateDraining`，并用 `sd.Keep(sd.Serving())` 过滤。

`selector.Select` 现在会返回完成回调，与均衡器早已放进 `sd.Picked.Done` 的
是同一件事：

```go
// 之前
instance, err := pick.Select(ctx, request)

// 之后
instance, done, err := pick.Select(ctx, request)
if err != nil { return err }
started := time.Now()
err = dial(instance.Address)
done(sd.Outcome{Err: err, Latency: time.Since(started)})
```

回调在成功时永不为 nil，且幂等。即使策略看起来无状态也请上报结果——实例层
此前会丢掉它，导致任何基于反馈表的策略把每次选择永久计为在途。

### 两条装配路径统一到一条所有权规则

`selector.Selector` 新增 `Close() error`，同时 `endpointer.Filter` /
`endpointer.Prefer` 不再关闭 source。两条路径的规则一致：关闭你自己构造的东西，
交给它的东西仍归你。

```go
// 之后
pick := selector.New(pool, strategy)
defer pick.Close()   // 释放 strategy；pool 与 Instancer 仍归你

view := endpointer.Filter(set, sd.MetadataEquals("zone", "a"))
defer set.Close()    // 关 set 而不是 view：view.Close() 现在是空操作
```

需要改的两处：

- 自定义 `Selector` 实现需要补一个 `Close() error`；不持有资源时返回 `nil` 即可。
- 依赖 `filter.Close()` 顺带关闭底层 endpoint set 的代码，改为直接关闭 set。
  其余不变，并且多个视图现在可以安全共享同一个 set。

相关但无需改动：装饰其他策略的策略现在会真正触发内部 `Close`。
`feedback.Table.Wrap` 与 `selector.Filtered` 通过 `selector.CloseStrategy` 透传；
如果你写过自己的装饰器，补上同样的一行，避免可关闭的内部策略被搁死：

```go
func (d *myDecorator) Close() error { return selector.CloseStrategy(d.inner) }
```

不需要改源码但需要注意的行为变更：

- 只改元数据也算变更。即使地址集合完全相同，重新打标签也会通知订阅方；
  此时 endpointer 会复用活跃端点，不会重连。
- 同一快照中出现两次的地址现在只产生一个端点。重复项被丢弃，而不是覆盖首个
  条目并泄漏它的 closer。
- 在 Consul 中，服务的 `Meta` 会呈现为 `Instance.Metadata`；Tags 不会，它仍然是
  `TagsInstancerOptions` 的过滤输入。

## 升级到 v2.7.0

三处源码改动：

1. `endpoint.Metrics` 的计数字段不再导出。改为通过 `Snapshot()` 读取，字段名
   保持不变：

   ```go
   // 之前
   count := metrics.RequestCount

   // 之后
   count := metrics.Snapshot().RequestCount
   ```

2. `endpoint.TypeAssertError.Want` 改为 `reflect.Type`。请用泛型构造函数而不是
   复合字面量创建该错误：

   ```go
   // 之前
   var zero Req
   return &endpoint.TypeAssertError{Got: request, Want: zero}

   // 之后
   return endpoint.NewTypeAssertError[Req](request)
   ```

3. `endpoint.RateLimiterFunc` 改名为 `RateLimiterFuncs`。只有类型名变了，字段
   保持不变：

   ```go
   // 之前
   limiter := endpoint.RateLimiterFunc{AllowFn: bucket.Allow}

   // 之后
   limiter := endpoint.RateLimiterFuncs{AllowFn: bucket.Allow}
   ```

不需要改源码但需要注意的行为变更：

- 端点超时现在编码为 HTTP 504 而不是 500，与 `apperror.KindDeadlineExceeded`
  及 gRPC 适配器一致。客户端断连（`context.Canceled`）现在编码为 499 而不是
  500。此前把这两者当作 500 的客户端与告警规则需要更新。
- `ErrCircuitOpen`、`ErrBulkheadFull`、`ErrBackpressure` 现在编码为 HTTP 503
  而不是 429，因为它们自带 `apperror.KindUnavailable` 分类。`ErrRateLimited`
  仍为 429。在 gRPC 上这四个错误此前都表现为 `Internal`，现在会带上真实的码。
  请更新客户端重试策略以及任何把熔断计为 429 的看板。
- `kit.WithMetrics` 会用路由 pattern 标注每次观测，并运行在链的最外层。
  `metrics.Snapshot()` 仍然给出总量，但现在也会计入被限流或熔断拒绝的请求；
  按路由的数据用 `metrics.SnapshotFor(pattern)` 读取。
- `Fallback` 会在调用方已取消时跳过兜底，并在兜底也失败时合并两个错误，因此
  此前依赖兜底掩盖全部失败的调用方现在会看到主故障原因。

## 升级到 v2.6.0

`v2.6.0` 是经批准的架构进化版本，包含有意的破坏性变更。需要修改源码：

1. 装配层：移除 `kit.Service`、`kit.New`、`kit.MustNew` 与 `Service.Run`。
   拆分为：

   ```go
   // 之前
   svc, err := kit.New(":8080", opts...)
   kit.HandleJSONTyped(svc, "/hello", handler)
   svc.Run(ctx)

   // 之后
   svc, err := kit.NewHTTP(":8080", opts...)
   kit.HandleJSONTyped(svc, "/hello", handler)
   host, err := kit.NewHost(kit.WithLifecycle(svc))
   host.Run(ctx)
   ```

   `Handle`、`HandleFunc`、`HandleSSE` 成为 `*kit.HTTP` 的方法。
2. 错误模型：移除 `server.HTTPError`、`server.NewHTTPError`、
   `server.WrapHTTPError`。用 `apperror` 分类失败（各传输一致映射，包括
   gRPC）；协议专属定制使用 `transporthttp.StatusCoder`、`ErrorCoder`、
   `PublicMessager`、`Headerer`。
3. 移除已废弃的 `log` 门面；直接使用 `log/slog`。
4. SSE：移除 `kit.SSEWriter`。使用 `server.SSEStream`（方法集不变）配合
   `kit.HandleSSETyped`（套用 endpoint 中间件），或用 `HTTP.HandleSSE`
   方法注册原生流。
5. 生成项目（`microgen` `v0.3.0`）：用户持有的 `cmd/custom_routes.go`
   钩子现为纯标准库契约 —— `func registerCustomRoutes(r *http.ServeMux)`
   直接在 mux 上注册路由；把清单条目移入 `customRouteDescriptions() []string`。
   extend 拒绝 v3 之前的清单（`microgen.project.v2`）并给出此迁移提示；
   之后重新完整生成以刷新清单。

值得采用的新能力：`security` 主体契约（`RequireAuthenticated`、
`RequireRole`）、`slogadapter.NewTelemetry`、`kit.NamedLifecycle` /
`kit.ReadinessProvider`、`apperror` 快捷构造函数。

## 升级到 v2.4.1

`v2.4.1` 完全向后兼容：全部为新增能力与行为修复，无需修改源码。

从 `v2.2.0` 跨版本升级时值得复核的行为变化：

- `endpoint.TracingMiddleware` 生成的 trace ID 为 32 位小写十六进制字符
  （W3C Trace Context 格式），不再是 16 位（`v2.3.0` 变更）。把 ID 当作
  不透明字符串的调用方不受影响。
- `endpoint.ErrBackpressure` 与 `endpoint.ErrBulkheadFull` 在 HTTP 中编码为
  429，不再是 500（`v2.3.0` 变更）。

## 兼容性策略

- 补丁版本修复行为；次版本新增能力。两者在 `/v2` 内均向后兼容。
- 不兼容变更需要新的主 module 版本，但经批准并记录的例外除外：
  `v2.1.0` 的直接重构、`v2.2.0` 的指标返回类型修正、`v2.6.0` 的架构进化、
  `v2.7.0` 的错误契约统一。
- v2 不保留废弃转发 API。早期版本的文档仍可通过不可变的发布标签获取。
