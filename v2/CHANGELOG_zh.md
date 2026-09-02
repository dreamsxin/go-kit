# 变更日志
[English](CHANGELOG.md) | 简体中文

所有重要的 v2 变更都记录在这里。旧历史仍可通过不可变的 v0 和 v1 标签获取。

## [未发布]

### 新增

- `sd/balancer` 新增三种选择策略，轮询不再是唯一选项：`NewRandom`（均匀抽样，
  避免多客户端共享计数器导致的步调一致）、`NewWeightedRandom`（按每个实例的
  权重成比例选择；权重为 0 即可摘除实例，无需等待服务发现）、
  `NewConsistentHash`（虚拟节点哈希环，配合 `WithReplicas`、
  `DefaultReplicas`，只有归属实例离开时对应的键才会迁移）。最少请求改为组装而
  非构造——见 `feedback.Table.LeastRequest`——因为它需要一份测量存储，而端点层
  不应该依赖测量存储。
- `sdclient.WithBalancer` 用于替换选择策略。此前 `sd/client.NewEndpoint` 把轮询
  硬编码，想换策略只能绕开构造器，手工串接 `endpointer`、负载均衡器与 `sd/retry`。
- `sd.Outcome`、`sd.Done` 与 `sd.Picked`：每次 `Balancer.Pick(ctx, request)` 都会
  返回被选实例、端点和完成回调，retry 与直接调用方可以将时延、错误、字节数
  回灌到选择策略。
- `endpointer.InstanceEndpoint` 与 `endpointer.InstanceEndpointer` 会报告每个端点
  由哪个实例生成。加权、最少请求与哈希策略需要这个身份信息；轮询和随机不需要，
  仍然接收 `endpointer.Endpointer`。
- 实例元数据：`sd.Instance` 在 `Address` 之外新增 `Metadata map[string]any`，
  于是注册方可以上报静态标签（可用区、版本、协议、能力、权重、租户），
  服务发现侧可以据此选择。`sd.Addresses` 用于从裸地址构造快照，适合测试、
  本地开发以及不提供标签的注册中心。
- `sd.MetadataString`、`sd.MetadataInt`、`sd.MetadataBool` 对标签值做类型归一，
  因此按 `5` 书写的断言也能匹配注册中心返回的 `"5"`。
- `sd/endpointer` 新增子集过滤，对齐 Envoy 的 subset load balancer：`Filter`
  （不回退）与 `Prefer`（过滤结果为空时回退到全集），断言来自 `sd` 包。
  就近路由是组合出来的，而不是给每个策略都做一个可用区变体。
- `balancer.WeightFunc`、`balancer.MetadataWeight` 与 `balancer.DefaultWeightKey`
  从注册标签中读取权重。
- `consul.MetaRegistrarOptions` 在注册时上报静态标签，Consul instancer 会把每个
  条目的服务 `Meta` 还原成 `Instance.Metadata`；只改元数据也会作为变更广播。
  实时负载不进 catalog：由 `feedback.Table.LeastRequest` 在进程内度量。
- 新增 `sd/selector` 包：面向实例快照的选择策略，与端点无关。包含
  `Strategy.Pick(ctx, request, instances)`、`RoundRobin`、`Random`、
  `WeightedRandom`、`Scored`、`LeastRequest`、`ConsistentHash` 六种策略，`Static`、
  `Subscribe`、`Filter`、`Prefer` 四种数据源，以及用于绑定的 `New`/`Select`。只需要地址的
  调用方——自己拨号的代理、回答"我该连哪台"的 API——装配这一层即可，完全不运行
  端点工厂，也就不会为没人调用的实例建连接。
- `balancer.New(source, strategy)` 把任意 `selector.Strategy` 变成
  `sd.Balancer`，自定义策略无需重复实现端点查找即可被 `sd/client` 与 `sd/retry`
  使用；请求始终通过同一个策略方法传入。
- `sd/feedback.Table` 记录 EWMA 时延、错误率、字节数、在途请求，以及每个地址
  第一次出现的时间。`Table.Score`、`Table.Load`、`Table.LeastRequest`、
  `Table.FirstSeen` 与 `Table.Wrap` 将本地观测接入评分选择、最少请求选择与慢启动。
  `feedback.Follow(instancer, retainers...)` 与 `Table.Retain(instances)` 会丢弃
  已离开服务发现的地址的测量数据，使长期运行的表规模等于服务规模，而不是部署
  历史的规模。`Table.Reset(instance)` 清除某个地址的测量值，但保留其首次出现
  时间与在途计数。`feedback.WithClock` 用于在测试中注入时间。
- 新增 `feedback.Ejector` 与 `feedback.EjectionPolicy`：对齐 Envoy 的被动异常点
  检测。`Ejector.Filter()` 在观测到 `MinSamples` 次调用后，移除超过
  `MaxErrorRate`、`MaxLatency` 或 `MaxInFlight` 的实例；`MaxEjectionPercent`
  （默认 50）在候选集里"不健康"占比过高时拒绝摘除——整池同时失败通常意味着
  共享依赖出问题。一次摘除在 `BaseDuration` 后到期，并按该地址历史摘除次数
  翻倍、上限 `MaxDuration`；到期时会重置导致摘除的测量值，否则没有流量的
  衰减均值永远不会恢复，第一次摘除就等于永久摘除。
- 新增 `feedback.Retainer` 与包级 `feedback.Follow`：只向 instancer 订阅一次，
  同时驱动表与所有 ejector，因此"地址已离开"只在一个地方被遗忘。服务发现报错
  不再被当成"实例全没了"。
- 新增 `sd.InstanceFilter` 与 `sd.Keep(match)`，以及
  `selector.Filtered(strategy, filters...)`：为无法按单个实例判定的策略提供
  集合级过滤契约。被动摘除必须先知道"这一摘会摘掉多少"才能决定摘不摘，
  因此 `Ejector.Filter` 返回 `InstanceFilter`，其摘除上限按当次候选集计算。
- 新增 `sd/health` 包：以 `sd.Instancer` 装饰器的形式做主动探测，下游每一层
  都不需要改。`health.Check(source, probe, options...)`，配合 `health.Probe`、
  `health.TCPProbe`、`health.HTTPProbe`，以及探测间隔、超时、阈值、并发度、
  日志与初始状态等选项。被动检测只能看见有流量的实例；主动探测还能看见冷实例
  与不可达实例。实例在首次探测完成前按健康对待；当没有任何实例通过探测时，
  checker 会原样发布未经检查的集合而不是空集，避免探针自身故障把整个服务
  变成黑洞。
- 新增 `selector.Ranker`、`selector.NewRanker` 与 `selector.ScoreFunc`：对快照
  排序并返回最优的 N 个，而不是只选一个。这正是"选路服务"需要的形状——调用方
  自己去连。同分按地址排序，因此两个进程对同一份快照排序结果一致。
  `Rank(ctx, request, n)` 接收 request 的理由与 `Strategy.Pick` 相同——候选清单
  可以取决于"谁在问"；它有意不做成 `Strategy`：ranker 不拥有任何一次调用，
  因此没有 `Done` 可以返回。
- 新增 `selector.SlowStart` 与 `selector.FirstSeenFunc`：让新实例的权重在一个
  窗口内逐步爬升。否则冷实例在最少请求与评分比较里稳赢，会在缓存和连接池还没
  热起来时就承接全量流量。
- selector 层新增 `selector.LeastRequest`、`selector.LoadFunc`、
  `selector.WithChoices` 与 `selector.DefaultChoices`，不需要端点也能做在途选择；
  `feedback.Table.LeastRequest` 把它与度量它的那张表绑定在一起。
- 新增 `sd.StateKey`、`sd.StateReady`、`sd.StateDraining`、`sd.Draining()` 与
  `sd.Serving()`：表示"仍在注册中心里、但不该再接新流量"的标签约定。排空是
  注册信息的属性，因此它属于元数据，而不属于反馈表。
- `balancer.NewScored` 与 `selector.Scored` 按调用方给出的分数选择，是本进程并未
  亲自度量的负载信号的接入点：实例推送的报告、ORCA/LRS 式带外上报，或任何本地
  表。返回 `false` 表示排除该实例，无需再写一个断言即可表达硬过滤。
- 新增 `sd.Match` 及标签断言 `sd.MetadataEquals`、`sd.MetadataIn`、
  `sd.MetadataMatches`、`sd.HasMetadata`、`sd.And`、`sd.Or`、`sd.Not`，
  端点层与实例层共用同一套断言。
- 新增服务发现章节（中英双语）`docs/service-discovery.md`：端到端的出向链路、
  每一层拥有与不拥有什么、`Pick`/`Done`/`Outcome` 生命周期、策略选择、反馈与
  摘除、慢启动、主动探测、provider 选择、长连接、关闭顺序，以及按症状排查的
  对照表。`sd/README.md` 保留 API 参考并链接到该章节，不再重复其中的推理；
  `feedback`、`health` 与 etcd 现在各自有了导航入口。

### 变更

- **破坏性变更：** `sd.Event.Instances` 的类型由 `[]string` 改为 `[]sd.Instance`。
  没有标签需要上报时，用 `sd.Addresses("host:port", ...)` 构造快照。
- **破坏性变更：** `endpointer.Factory` 的参数由字符串地址改为 `sd.Instance`，
  使 factory 能够遵循决定"如何连接"的标签——scheme、TLS、协议。地址读
  `instance.Address`。
- **破坏性变更：** `balancer.NewWeightedRandom` 的第二个参数由 `func(string) int`
  改为 `balancer.WeightFunc`（`func(sd.Instance) int`）。
- **破坏性变更：** 子集断言从 `sd/endpointer` 移到根 `sd` 包，因为端点层与新的
  实例层都要读它：`endpointer.Match` → `sd.Match`，
  `endpointer.MetadataEquals` → `sd.MetadataEquals`，`MetadataIn`、
  `MetadataMatches`、`HasMetadata`、`And`、`Or`、`Not` 同理。
  `endpointer.Subset` 与 `endpointer.PreferSubset` 改名为 `endpointer.Filter`
  与 `endpointer.Prefer`，与 `selector.Filter`、`selector.Prefer` 对齐：同一个
  操作不该在两层有两个名字。
- **破坏性变更：** `sd.Balancer` 改为 `Pick(ctx, request) (sd.Picked, error)` 并
  增加 `Close`；删除 `Endpoint`、`EndpointFor` 与 `RequestBalancer`。
- **破坏性变更：** `sd.Instancer` 增加 `Close() error`。
- **破坏性变更：** 删除 `balancer.NewLeastRequest`，同时删除
  `balancer.LoadFunc`、`LeastRequestOption`、`DefaultChoices`、`WithChoices`
  这几个别名，以及更早的 `WithTable`/`WithFeedback` 选项。请改为组装：
  `balancer.New(set, table.LeastRequest(...))`。两个原因：端点层此前无条件
  import `sd/feedback`，导致连纯轮询的装配也把可选的反馈层编译进来；而
  `table == nil` 这个简写造出的表调用方拿不到句柄，因此永远无法 `Retain`，
  在滚动发布下会无界增长。最少请求本身位于
  `selector.LeastRequest(load, options...)`，实例层也能用它。
- **破坏性变更：** 被动健康检查由表上的方法改为独立的 `feedback.Ejector`。
  移除 `Table.Healthy`、`feedback.HealthPolicy`、`feedback.Policy`、
  `Table.Done` 与 `Table.Follow`；改用
  `feedback.NewEjector(table, feedback.EjectionPolicy{...}).Filter()` 配合
  `selector.Filtered`、`Track`，以及包级
  `feedback.Follow(instancer, table, ejector)`。摘除现在有放回路径：没有它的话
  第一次摘除就是永久的——实例拿不到流量，导致摘除的测量值也就永远不会衰减。
- `sd/balancer` 现在是 `sd/selector` 之上的薄层：加权、评分与哈希策略都住在
  selector，balancer 只补上端点查找。均衡器策略相关 API 保留——`WeightFunc`、
  `KeyFunc`、`ConsistentHashOption`、`DefaultWeightKey`、`DefaultReplicas`、
  `MetadataWeight`、`WithReplicas` 现在是 selector 对应物的别名或转发。轮询与
  随机仍直接读端点，因为它们不需要实例身份。
- 当实例只是改了标签时，endpointer 缓存会复用活跃端点，因此重新打标签不会重连；
  同一快照中的重复地址会被跳过，避免泄漏首个端点的 closer。
- `endpointer.NewEndpointer` 的返回类型声明为 `InstanceEndpointer`。

### 修复

- **破坏性变更：** `selector.Selector.Select` 与包级 `selector.Select` 改为返回
  `(sd.Instance, sd.Done, error)`。此前实例层丢弃了策略返回的回调，导致基于
  反馈表的策略在 `selector.New` 路径下把每次选择都永久计为在途：该实例在最少
  请求与评分选择里从此显得满载，`Table.Reset` 按设计不清在途计数，`Table.Retain`
  也永远无法回收这条记录。现在回调会被转发，成功时永不为 nil，且幂等，因此
  `defer done(outcome)` 是安全的。
- `sd/health` 不再让 `WithInitiallyHealthy(false)` 形同虚设。发布未检查全集的
  fail-open 现在要求每个实例都已产生过探测结果；此前在启动瞬间尚无任何探测结果，
  于是这个选项恰好发布了它本该隐藏的实例。
- `feedback.Table` 补齐了条目的生命周期闭环。实例带着在途调用离开服务发现时，
  其记录此前要等下一份快照才会被清理，若此后再无快照就永久留存；现在最后一次
  完成回调会删除它。已退休的地址若重新出现在服务发现里，测量数据会被保留。
- `sd/health` 改用固定 worker pool 探测，不再每轮为每个实例创建 goroutine，并在
  `Close` 取消后立即停止投喂任务。`health.Probe` 的文档现在明确要求它必须在
  context 取消时返回，因为 `Close` 会等待正在进行的这一轮。
- 文档不再把已删除的 `integrations/circuitbreaker` 与 `integrations/ratelimit`
  模块当作仍然存在。`endpoint/README` 曾写"仍是可独立选择的组件"，
  `internal/docs/RELEASE.md` 的发布 runbook 仍为二者列出打标签命令、却漏了
  `integrations/etcd`，依赖报告与路线图也仍在统计它们。仅存在于这两份废弃 README
  里的内容——熔断器默认值、`CircuitBreaker.State()`、`RateLimiterFuncs`
  以及多副本限流说明——已并入 `endpoint/README`。

### 移除

- 删除 `integrations/circuitbreaker` 与 `integrations/ratelimit` 目录。两个模块在
  v2.4.0 中间件并入 `endpoint` 时就已删除，残留的只是一份废弃说明 README；而
  `go get` 解析旧依赖走的是 module proxy 与既有标签，不是仓库里的文件。


## [2.7.0] - 2026-09-01

### 新增

- `kit` 路由级 endpoint 中间件：`HandleJSONWithMiddleware` 与
  `HandleJSONTypedWithMiddleware` 把路由级中间件组合在处理器最近处，位于
  `WithEndpointMiddleware` 安装的组件级链之内。
- MIGRATION 记录从旧版 go-kit（v0/v1 风格）迁移到 v2 的构造映射与推荐顺序。
- `server.AccessLogMiddleware`：传输边界的标准库访问日志（方法、路径、状态码、字节数、耗时、trace ID），经 `kit.WithHTTPMiddleware` 安装。
- 中间件章节记录各横切关注点（日志、追踪、指标、错误）在 service、endpoint、transport 三层的规范位置。
- 新增排障章节（双语）：按症状排查——request_id/trace_id 请求关联、状态码成因（含请求被哪种保护拒绝）、启动失败、就绪失败、数据库连接池设置、日志配置与调试开关。配置章节新增完整的生成配置段参考。
- 新增自定义章节（双语）：选择日志去向（文件存储、多目标写入、轮转指引）、编写自定义端点与 HTTP 中间件及安装位置表、错误自定义决策表。
- `endpoint.Recorder` 与 `endpoint.Observation`：指标扩展点。
  `RecordingMiddleware(operation, recorders...)` 为每次调用计时并按操作标签
  上报，因此对接 Prometheus 或 OpenTelemetry 只需实现一个接口，不必再改中间件。
  `endpoint.Metrics` 实现了 `Recorder`，并新增 `SnapshotFor` 与 `Operations`
  用于读取按操作维度的数据，总量读取方式不变。
- `kit.WithRecorder` 注册外部 recorder；它与 `kit.WithMetrics` 都会用路由
  pattern 标注每次观测，因此 `metrics.SnapshotFor("POST /users")` 只报告该路由。
- `endpoint.RetryMiddleware`（Builder：`WithRetry`）：以指数退避加全抖动重试
  瞬时失败。`DefaultRetryable` 重试 `apperror.KindUnavailable` 以及通过
  `interface{ Retryable() bool }` 自行分类的错误，不重试 context 错误、本地拒绝
  与未分类错误；`WithRetryable` 与 `WithRetryBackoff` 可分别替换这两项策略。
  错误自带的重试提示优先于退避计划，上限为 `endpoint.MaxRetryAfterHint`。
- `client.HTTPStatusError` 现在自带分类，因此客户端 endpoint 可与服务端 endpoint
  共用同一套中间件。它通过把响应状态码反查为 kind（导出为
  `client.KindForStatus`）实现 `apperror.Kinder`，并通过解析 `Retry-After` 响应头
  （秒数与 HTTP-date 两种形式）实现 `endpoint.RetryAfterReporter`。客户端 endpoint
  上的 `WithRetry` 不再需要自定义分类器；转发上游 503 时也会保持状态码，而不再退化
  为 500。
- `grpc.StatusError`、`grpc.ClassifyError` 与 `grpc.KindNameForCode` 让 gRPC
  客户端获得同等能力：客户端会包装失败的 `Invoke`，使错误上报 kind 名称、
  `Retryable` 与 `google.rpc.RetryInfo` 中的延迟，同时 `status.FromError` 与
  `status.Code` 照常可用。
- `grpcserver.NewErrorEncoder` 配合 `WithKindMapper` 与 `WithErrorDomain`，用一个
  基于 option 的工厂取代不断增长的编码器构造函数。稳定业务码现在按 AIP-193 放在
  `google.rpc.ErrorInfo` detail 中（与既有的 `RetryInfo` 并列），因此 gRPC 这一跳
  不再像以前那样丢掉业务码——HTTP 消息体一直是携带它的。
  `ErrorEncoderWithKindMapper` 保留为简写。
- `grpc.StatusError.ErrorCode` 与 `ErrorDomain` 把该 `ErrorInfo` 读回，
  `client.HTTPStatusError.ErrorCode` 解析 JSON 消息体中的 `code`。两者都满足
  `transport/http.ErrorCoder`，因此原样转发上游错误会复现上游业务码，而不是按状态码
  生成的名字。上游 message 有意不作为可公开消息转发。
- `endpoint.RetryAfterReporter`、`endpoint.RetryAfterError` 与
  `endpoint.NewRetryAfterError`：传输无关的重试提示。HTTP 编码器写出
  `Retry-After`，gRPC 编码器附加 `google.rpc.RetryInfo`。熔断器打开时上报开窗
  剩余时间；实现了该接口的 `RateLimiter` 的延迟会附加到 `ErrRateLimited`。
- `apperror.KindCanceled`（HTTP 499 / gRPC Canceled）与
  `apperror.KindUnimplemented`（HTTP 501 / gRPC Unimplemented），以及对应构造
  函数。`server.StatusClientClosedRequest` 为非标准的 499 提供命名常量。
- 补齐目录中其余中间件的 Builder 快捷方式：`WithRateLimit`、
  `WithDelayRateLimit`、`WithCircuitBreaker`、`WithRecording`、`WithRetry`。

### 变更

- **破坏性：** endpoint 层的拒绝错误现在通过 `apperror` 自带分类，HTTP 编码器
  不再对它们做特殊处理。`ErrRateLimited` 仍是 429（`KindResourceExhausted`），
  但 `ErrCircuitOpen`、`ErrBulkheadFull`、`ErrBackpressure` 从 429 改为 503
  （`KindUnavailable`）：卸载负载是服务端状况，不是调用方配额问题，Envoy 与
  Resilience4j 也是这样表达的。gRPC 适配器此前把这四个错误全部映射为
  `Internal`，现在与 HTTP 一致。此前把熔断当作 429 处理的客户端需要接受 503。
- **破坏性：** `endpoint.RateLimiterFunc` 改名为 `RateLimiterFuncs`。它是两个
  函数组成的结构体而非函数类型，复数名称更贴合其形态，也让 `XxxFunc` 约定继续
  表示"函数类型"。
- **破坏性：** 从 handler 原样返回的 `client.HTTPStatusError` 现在按上游状态码
  编码（依赖返回 404 就回 404），不再一律 500，因为该错误已自带分类。当上游失败
  对你的调用方含义不同时，请显式转译：
  `apperror.WrapCause(apperror.KindUnavailable, "upstream.users", err)`。
- HTTP 错误编码器在缺少 `apperror.Kinder` 时改用 `apperror.KindNamer` 解析错误
  kind，因此为避免依赖 `apperror` 而采用结构式分类的模块（例如 gRPC 集成）会映射到
  正确状态码，而不再一律 500。gRPC 编码器本就是这样工作的。
- HTTP 错误编码器把未分类的 `context.Canceled` 映射为 499 而不再是 500，使
  客户端断连不再计入 5xx。gRPC 适配器本就把它映射为 `Canceled`。
- `kit.WithMetrics` 现在把收集器作为带路由标签的 recorder 安装在链的最外层，
  而不是按 option 顺序插入中间件，因此被限流或熔断拒绝的请求同样会被计入。
- `DecodeRequestFunc`、`EncodeResponseFunc`、`EncodeRequestFunc`、
  `DecodeResponseFunc`、`ServeGRPC` 与 `Interceptor` 的动态参数改写为 `any`
  而非 `interface{}`。两者类型完全相同，调用点无需改动。
- HTTP 错误编码器把未分类的 `context.DeadlineExceeded` 映射为 504 而不再是
  500，使 `TimeoutMiddleware` 造成的端点超时与 `apperror.KindDeadlineExceeded`
  以及 gRPC 适配器（本就把 context 超时映射为 `DeadlineExceeded`）保持一致。
  显式分类仍然优先：包裹了超时原因的 `apperror` 按自己的 kind 映射。此前把端点
  超时当作 500 处理的客户端需要接受 504。
- `Fallback` 在调用方 context 已取消或已过期时不再执行兜底端点；兜底也失败时
  合并主错误与兜底错误。此前主故障原因会被丢弃。
- 需要依赖的中间件构造函数——`MetricsMiddleware`、`RateLimitMiddleware`、
  `DelayRateLimitMiddleware`、`Fallback`、`InFlightMiddleware`——现在在装配期
  对 nil 触发 panic，而不是延迟到第一个请求，与 `NewBuilder`、`Chain` 一致。
- `NewCircuitBreaker` 把非正数设置归一化为导出的默认值，因此
  `WithBreakerFailureThreshold(0)` 按文档选择 5，而不是首次失败即熔断。
- 完整的 kind→状态码映射表现在只保留在错误处理章节（`docs/errors_zh.md`），
  并补上 gRPC 列与不经分类的映射（拒绝错误、context 超时）；核心概念改为链接
  过去，不再重复一份不完整的副本。
- `make verify` 新增 `test-fmt` 关卡，任何 Go 文件不符合 gofmt 即失败；仓库已
  全量格式化。

### 修复

- `WithBreakerSuccessThreshold` 此前完全无效：half-open 状态下第一次探测成功
  就闭合熔断器，忽略配置的阈值。现在会累计连续探测成功次数，并在探测失败或
  熔断器重新打开时清零。
- `make test-runtime` 引用了已删除的 `./log` 包以及已删除的
  `integrations/circuitbreaker`、`integrations/ratelimit` 模块，导致
  `make test` 与 `make verify` 无法完成。

### 移除

- **破坏性：** `endpoint.Metrics` 的导出计数字段（`RequestCount`、
  `ErrorCount`、`SuccessCount`、`TotalDuration`、`LastRequestTime`）。它们允许
  在不加锁的情况下读取由内部 mutex 保护的状态。请通过 `Snapshot()` 读取，
  `MetricsSnapshot` 保留了相同的字段名。迁移方法见 MIGRATION_zh.md。
- **破坏性：** `endpoint.TypeAssertError.Want` 改为 `reflect.Type` 而不再是零
  值，因此接口与指针目标也能给出可用的类型名。请使用新的
  `endpoint.NewTypeAssertError[T](got)` 构造。
- `endpoint/metrics_prometheus.go`：一个 `//go:build ignore` 且函数体整体被注
  释掉的文件。指标导出请使用 `observability/otel`。

### 新增

- `apperror` 补齐其余 kind 的便捷构造：`Internal`、`AlreadyExists`、
  `FailedPrecondition`、`ResourceExhausted`、`DeadlineExceeded`。现在每个 kind
  都有同名构造函数。
- `endpoint.DefaultBreakerFailureThreshold`、`DefaultBreakerSuccessThreshold`、
  `DefaultBreakerOpenTimeout` 导出熔断器默认值。
- `tools/documentation_api_test.go` 校验文档中提到的每个框架符号是否仍被其所在
  包声明，捕获人工黑名单会漏掉的已删除 API 引用。

## [2.6.0] - 2026-08-27

### 新增

- `security` 根包：面向认证边界的传输中立主体契约 —— `Subject`、
  `Authenticator`、`WithSubject` / `SubjectFromContext`，以及 endpoint 中间件
  `Middleware`、`RequireAuthenticated`（401）、`RequireRole`（403），全部通过
  apperror 分类，由各传输一致映射。
- `kit.NamedLifecycle` 与 `kit.ReadinessProvider`：生命周期组件可为启动/
  故障/停机诊断提供名称，并把异步预热桥接到 `/readyz` 与 `/health` 的
  就绪检查。
- `apperror` 快捷构造：`InvalidArgument`、`Unauthenticated`、
  `PermissionDenied`、`NotFound`、`Conflict`、`Unavailable`，以及不携带公开
  消息、保留原因的 `WrapCause`。
- `endpoint.ValidationError` 实现 `apperror.Kinder` 与 `apperror.KindNamer`，
  传输层经标准 apperror 路径映射校验失败。
- `sd/client.NewEndpoint` 接受 nil logger，回退到 `slog.Default()`。
- `slogadapter.NewTelemetry` 把内置可观测性维度装配为一条固定顺序的中间件链（tracing → metrics → logging），共用同一个 `endpoint.Metrics` 采集器；`Telemetry.Apply` 可将链安装到 Builder 并带稳定标签。
- Server-Sent Events 下沉到传输层：`server.NewSSEServer` 与 `server.NewSSEServerTyped` 把流式处理器适配为 `http.Handler`，复用标准 Server 钩子（ServerBefore、头前解码、ServerErrorHandler、ServerFinalizer）；`kit.HandleSSETyped` 经 endpoint 中间件链注册类型化流，认证、追踪、指标与校验从此对流生效。一个流计为一次请求；解码失败映射为常规错误响应。
- ARCHITECTURE 文档固化模块依赖层级（L0–L5）与 context 键规范。

### 修复

- `kit` 生命周期监视器现在持续消费组件的异步错误直至停机；此前每个组件只汇报第一个错误。

### 移除（Breaking）

- `server.HTTPError`、`server.NewHTTPError`、`server.WrapHTTPError`。
  从 endpoint 或 service 层携带 HTTP 状态违反了分层边界。请使用 `apperror` 分类失败（传输中立，同样适用于 gRPC）；协议专属定制继续通过 `transporthttp.StatusCoder`、`ErrorCoder`、`PublicMessager`、`Headerer` 实现。
- 已废弃的 `log` 兼容门面。请直接使用 `log/slog`。
- `kit.SSEWriter` 与 `kit.HandleSSE` 的流函数签名。流式能力现位于传输层：使用 `server.SSEStream`（方法集不变）配合 `kit.HandleSSETyped`（套用 endpoint 中间件），或用 `HTTP.HandleSSE` 方法注册原生流处理器。
- `kit.Service`、`kit.New`、`kit.MustNew` 与 `Service.Run`（Breaking）。装配拆分为传输中立的 `kit.Host`（编排 `kit.Lifecycle` 组件）与拥有路由、健康检查和 HTTP 服务器的 `kit.HTTP` 组件。组件使用 `kit.NewHTTP` / `kit.MustNewHTTP`；进程使用 `kit.NewHost(kit.WithLifecycle(...))` + `Host.Run`。`HandleJSON*` 与 `HandleSSETyped` 接受 `*kit.HTTP`；`Handle`、`HandleFunc`、`HandleSSE` 成为 `*kit.HTTP` 的方法。纯 worker 或纯 gRPC 服务现在可以不经 HTTP 通过 Host 运行。

### 变更

- 文档改为引用核心 `endpoint` 熔断器与限流器，不再引用已移除的 `integrations/circuitbreaker` 与 `integrations/ratelimit` 适配器。
- microgen（对生成项目 Breaking）：用户持有的 `cmd/custom_routes.go` 钩子现为纯标准库契约 —— `func registerCustomRoutes(r *http.ServeMux)` 直接在 mux 上注册路由，`customRouteDescriptions() []string`（“METHOD /path” 条目）为 `/debug/routes` 与启动路由清单提供条目，取代原来返回生成器内部路由条目的写法。生成项目写入清单 schema `microgen.project.v3`；extend 拒绝 v3 之前的项目并给出可执行的迁移提示。

## [2.5.2] - 2026-08-22

### 新增

- HTTP 上的自定义 body 格式：`server.RawBodyCodec` 与
  `server.RawBodyCodecWithMaxBytes` 把两个纯函数变成传输编解码器（受限
  请求体、保留 StatusCoder/Headerer），`server.TextErrorEncoder` 让错误
  响应与路由格式一致，而不是默认 JSON。这类路由上的 apperror 是可选项；
  框架不强制任何错误模型。
- `examples/customcodec`：可运行的自定义格式服务（HTTP 上的长度前缀
  二进制 body，错误响应同格式）。

## [2.5.1] - 2026-08-22

### 新增

- 双协议绑定：`transport.Binding[Req, Resp]` 承载一次构建好的中间件端点，
  同时服务 HTTP 与 gRPC，无需重复装配。`Binding.TypedEndpoint()` 直接供
  类型化 JSON 服务器使用；gRPC 侧用 `grpcserver.NewServer` 加两个 protobuf
  映射函数即可。同一条中间件链在两个协议上运行。

## [2.5.0] - 2026-08-22

### 新增

- gRPC 自定义错误种类映射：`grpcserver.ErrorEncoderWithKindMapper` 先经应用
  映射解析 gRPC 状态码，未知 kind 回退内置映射；`grpcserver.CodeForErrorKind`
  公开内置映射用于组合。
- MCP 交互错误映射：`mcp.RPCCodeForInteractionError` 与
  `mcp.ErrorMapperForInteraction` 把交互哨兵错误映射为 JSON-RPC 错误码，
  自定义映射可以基于已记录的契约构建，而不必逆向 handler 实现。
- 响应组装组合器：`server.WrapJSONResponse` 包装响应值（信封、后处理），
  同时保留原响应的 StatusCoder 与 Headerer 行为。
- 中间件链自省：`endpoint.Builder.UseNamed` 为中间件打标签，
  `Builder.Describe` 按应用顺序返回整条链；内置 `With*` 快捷方式自动记录
  标签。启动日志现在可以打印组装后的链路。
- 启动期请求校验：`transporthttp.ValidateQueryStruct[T]` 在装配期检查
  查询/路径请求结构体的标签与支持的字段类型，把不支持的类型从首个请求
  前移到启动失败。

## [2.4.4] - 2026-08-22

### 新增

- 自定义错误种类状态码映射：`server.JSONErrorEncoderWithKindMapper` 先经
  应用映射解析 HTTP 状态码，未知 kind 回退到内置映射；
  `server.HTTPStatusForErrorKind` 公开内置映射用于组合。应用现在可以在
  不替换整个错误编码器的情况下，为自定义 `apperror.Kind` 定义自定义
  状态码。
- `client.DecodeJSONResponse` 与 `client.DecodeJSONResponseWithMaxBodyBytes`
  导出默认 JSON 响应解码器，使用 `NewExplicitClient` 组装自定义客户端时
  可复用相同的状态处理与响应体限制。

## [2.4.3] - 2026-08-22

## [2.4.2] - 2026-08-23

## [2.4.1] - 2026-08-22

## [2.4.0] - 2026-08-21

### 新增

- 传输层响应组装：`server.ServerResponseEncoder` 覆盖 JSON 入口点的成功
  响应编码器；`kit.WithJSONServerOptions` 把 server 选项（信封、错误格式、
  钩子）一次性应用到全部 JSON 路由，单路由选项优先生效。
- `examples/envelope`：传输边界的响应组装示例--业务 handler 不感知信封，
  `{code, message, data}` 及配套错误格式在装配期一次定义。
- 组合与嵌套文档：transport 指南解释累积式与替换式组件，以及 body、
  路径、查询与 multipart 解析器的组合方式；endpoint 指南记录中间件的
  四种流控模式（短路、分支、重复、替换）。

## [2.3.0] - 2026-08-20

### 新增

- 核心包中的 W3C Trace Context 传播：带 `ParseTraceparent` 的 `endpoint.TraceContext`，以及 `transport/http` 中面向服务端和客户端的 `ExtractTraceparent` / `InjectTraceparent` RequestFunc。`endpoint.TracingMiddleware` 现在会在同一 trace ID 下加入传入的 trace 上下文，否则生成符合 W3C 的 32 个十六进制字符的 trace ID。
- `kit.HandleSSE` 和 `kit.SSEWriter`，用于支持逐事件刷新和客户端断连取消的 Server-Sent Events 流；支持命名事件、JSON 事件和多行数据事件、注释心跳以及重试提示。
- `server.ParseMultipartForm`，用于有界的 multipart/form-data 上传（总请求体、单文件和内存上限；413/415/400 分类），以及用于净化文件下载的 `server.WriteAttachment`。
- 请求校验约定：带字段级失败的 `endpoint.Validatable`、`endpoint.ValidationMiddleware` 和 `endpoint.ValidationError`；HTTP 错误编码器将它们映射为 400 和稳定代码 `bad_request.validation`。
- `transport/http` 中的分页约定：`ParsePage`（默认 1/20，页大小上限 100，无效查询值作为校验错误拒绝）、`Page.Limit/Offset` 和泛型 `PageResult[T]` 传输形状。
- `examples/auth`：应用所有的认证与授权中间件，使用 Bearer API 密钥、由 `apperror` 分类的 401/403 响应以及公开的健康路由。
- `examples/todosvc`：端到端 SQLite CRUD 服务，带无 CGO 的仓储、`apperror` 分类、路径参数路由，以及优雅关闭期间的数据库关闭。
- 弹性中间件：`endpoint.Fallback` 在主端点失败时用回退端点应答，`endpoint.BulkheadMiddleware` 按资源键隔离并发；`ErrBackpressure` 和新的 `ErrBulkheadFull` 现在编码为 HTTP 429 而不是 500。
- PRODUCTION.md 新增部署（静态容器、探针接线、终止预算对齐、配置注入）、告警（与文档化指标信号对应的入门告警集）和后台任务（目录结构及 `kit.Lifecycle` 接线）章节。

### 变更

- `endpoint.TracingMiddleware` 生成的 trace ID 是 32 个小写十六进制字符（W3C 格式），而不是 16 个。将该 ID 视为不透明字符串的现有调用方不受影响。

### 修复

- `microgen` 不再注册 `-add-tables` 标志：它此前被解析但从未被使用，误导用户以为 extend 模式可以追加表。MICROGEN.md 现在说明了所覆盖的生成器版本，并列出了源模式和 `-service` 选项及其默认值。

## [2.2.0] - 2026-08-14

本开发周期包含一个经明确批准、范围狭窄的 v2 SemVer 例外：`Metrics.Snapshot()` 现在返回 `MetricsSnapshot` 而不是 `Metrics`。迁移方法记录在 [MIGRATION_zh.md](MIGRATION_zh.md) 中。

### 新增

- 传输无关的 `apperror` 分类，具有一致的 HTTP 与 gRPC 映射，包括默认的 gRPC 错误编码器。
- `kit` 和 `transport/http/server` 中面向具体请求与响应类型的完全类型化 JSON 装配辅助函数。
- 带 `AverageDuration()` 的无锁 `endpoint.MetricsSnapshot` 值。
- 有界的请求 ID 校验器选项，以及对调用方提供的请求 ID 的保守校验。
- `config/custom.go` 中应用所有的生成配置扩展，包括在重新生成时保留的默认值、环境和校验钩子。

### 变更

- **已批准的 SemVer 例外：** `Metrics.Snapshot()` 返回 `MetricsSnapshot`。通过推断的局部变量进行字段访问不受影响；显式将结果声明为 `Metrics` 的代码必须更新其类型。
- `microgen` 使用纯 Go 的 SQLite 内省驱动，因此安装和运行 CLI 不再需要 CGO 或 GCC。
- 生成的演示客户端委托给生成的 Go SDK，而不是维护第二套 HTTP/gRPC 实现。
- 健康检查并发执行，带单检查超时，并且每个命名检查最多允许一个进行中的调用。
- 快速开始和组合示例使用类型化处理器，并通过进程生命周期传播监听器和关闭错误。
- `main` 分支现在只包含维护中的 v2 产品线；旧 v1 源码和文档仍可通过不可变的发布标签获取。
- 仓库根 README 现在是精简的 v2 入口，而不是重复的 v1 使用指南。

### 修复

- 默认的 HTTP 和 gRPC 错误编码器不再向客户端暴露未分类的内部错误消息，而已分类的应用错误保留稳定的公开代码和消息。
- 并发健康探测不再累积串行延迟，也不再启动重叠的超时检查。
- 示例指标读取使用原子快照，快照不再携带会触发 `go vet` copylock 失败的内部互斥锁。
- 公开 API 快照收集忽略写入 stderr 的工具下载诊断信息。

## [2.1.0] - 2026-08-14

本版本是经明确批准的直接重构 v2 SemVer 例外。它与 `v2.0.0` 不保持源码兼容；升级之前请先阅读 [MIGRATION_zh.md](MIGRATION_zh.md)。

### 新增

- 可执行的架构依赖门禁，以及与已发布 `v2.0.0` 基线对比的经过评审的依赖闭包对比。
- 提供者无关的 `kit.Lifecycle` 契约和用于多服务应用的可选 `kit/grpc` 组件。
- 通过 `make verify-published-core` 和 `make verify-published` 对核心模块和所有独立版本化模块进行清单驱动的验证。
- `transport/http/client.NewJSONClient` 的 4 MiB 默认成功响应上限，并为更大的契约提供显式构造函数。
- 通过 `Runtime.ReleaseSession` 实现的由传输所有的交互会话释放。

### 变更

- 仓库所有者批准将直接的不兼容重构作为已记录的直接重构 SemVer 例外发布到 `/v2`；`v2.0.0` 保持不可变，已发布的根重构版本是 `v2.1.0`。
- 根运行时模块现在没有第三方依赖。gRPC、Consul、Gobreaker、限流、Zap 和 OpenTelemetry 位于独立模块中。
- `microgen` 实现包改为内部包，生成代码直接使用 `slog`，最小 HTTP 项目不再包含可选的提供者和数据库模块。
- 推荐示例现在是使用 `kit` 的 `examples/quickstart`；显式的 endpoint/transport 接线位于 `examples/manual_composition`。
- 核心 `kit` 是仅 HTTP 的装配层。带外部依赖的 endpoint 中间件通过 `kit.WithEndpointMiddleware` 显式安装。
- HTTP 和 gRPC 传输默认使用空操作错误处理器；错误上报由应用通过 `observability/slog` 或 `integrations/zap` 适配器所有。
- `microgen` 现在默认关闭 config、model/repository 和数据库运行时接线。`-from-db` 仍然总是生成内省的模型。
- 完整重新生成会保留用户所有的服务实现、`cmd/main.go`、config YAML 和项目 README；endpoint 和 transport 产物在清单中被跟踪为生成器所有。
- 生成的调试路由注册和路由打印改为可选，限流默认禁用，中间件超时来自配置，入站重试配置已被移除。
- 生成的 HTTP 和 gRPC 监听器在开始服务之前绑定；生成的数据库句柄在关闭期间被检查并关闭。
- MCP 工具调用复用绑定到 MCP 传输会话的运行时会话，该会话在 DELETE 或 TTL 到期时释放。
- Go IDL 生成在遇到无效接口方法时失败，而不是静默省略该方法。

### 修复

- Consul 远程配置加载现在遵守其超时和响应大小限制，而不会把第二套基于 Viper 的提供者栈拉入生成的项目。
- 文档记录的 v2 发布标签是 Go 模块解析所需的根 `v2.0.0` 标签；历史上错误的 `v2/v2.0.0` 标签已被移除。

### 移除

- `endpoint`、`transport/grpc` 和 `sd/consul` 下旧的提供者所有的包路径；请使用相应的独立集成模块。
- `kit.WithGRPC`、`Service.GRPCServer`、`kit.WithRateLimit`、`kit.WithCircuitBreaker`、`kit.WithLogging` 以及根传输的 `NewLogErrorHandler` 便捷 API。
- 无用的合并配置模板以及过时的 Swagger 2.0 Make 目标/工具。

## [2.0.0] - 2026-07-20

首个稳定的 v2 版本。导出的运行时 API、`microgen` CLI 与配置、生成物所有权以及文档记录的协议行为现在遵循 [RELEASE_zh.md](internal/docs/RELEASE_zh.md) 中的兼容性策略。

### 新增

- 独立的 `github.com/dreamsxin/go-kit/v2` 模块。
- 返回错误的 `kit.New`、上下文驱动的 `Service.Run`、可配置的优雅关闭超时，以及用于显式 panic-on-invalid 初始化的 `kit.MustNew`。
- 针对服务端、日志、数据库、中间件和远程提供者设置的最终生成配置校验。
- 生成输出的确定性格式化和文本规范化。
- 仓库范围的 UTF-8 校验，拒绝维护文本文件中的 BOM、无效字节序列和 Unicode 替换字符。
- 使用 `go mod tidy` 和 `go test ./...` 的外部生成项目冒烟覆盖。
- 生成的传输、客户端和 SDK 共享的 HTTP 路径/查询编解码器。
- 直接从公共 `microgen` IR 生成 OpenAPI 3.1 和独立的 JSON Schema 2020-12 包。
- 从同一 IR 生成的零运行时依赖 TypeScript Fetch 客户端，带严格编译器设置和外部类型检查覆盖。
- 生成的传输、客户端和 SDK 共享的非 GET 路径参数编码与解码。
- 带版本号的 `.microgen/manifest.json` 项目身份，包含源、能力、路由、服务、模型、中间件和生成器所有的产物元数据。
- 针对 Go IDL、Protobuf 和数据库生成集成路径的 OpenAPI 3.1 解析器校验和 JSON Schema 2020-12 编译。
- 用固定的 TypeScript 编译器对生成 SDK 进行类型检查的发布契约检查。
- 生成的 Go 和 TypeScript SDK 的共享可执行 HTTP 行为覆盖，包括路径、查询、请求体、头部和非 2xx 错误。
- 针对 Go IDL、Protobuf 和数据库生成路径的经过评审的确定性契约快照。
- 可选的 `observability/slog` endpoint 日志，以及带应用所有的提供者初始化的独立 `observability/otel` 追踪/指标适配器。
- 可选的标准库 `security/http` 中间件，用于可信代理和客户端 IP 解析、IP 策略、CORS、签名双重提交 CSRF 以及安全响应头。
- `kit.WithHTTPMiddleware`，用于覆盖健康、endpoint、原生 HTTP 和生成路由的全服务级标准库中间件。
- 针对经过评审的公开 API 漂移、维护的 Markdown 链接、模块 tidy 状态、专项竞态测试、vet 以及已提交 v2 范围整洁性的发布门禁。

### 变更

- `kit` 不再安装进程信号处理器，也不在服务生命周期中调用致命日志。
- `Service.GRPCServer` 在未配置 gRPC 时返回错误。
- 生成的配置优先级为：本地 YAML、可选远程配置、最终环境变量覆盖，然后是校验。
- 服务发现注册同步返回其初始快照，并在不关闭消费方通道的情况下发布后续更新。
- 内存交互提供者复制可变的资源、blob、模板、提示词和渲染参数。
- 生成的 HTTP 服务器使用标准库 `http.ServeMux`；生成的 GET 客户端和服务器共享同一个带标签的查询契约，并且不发送 JSON 请求体。
- 生成的 Go 客户端和 SDK 使用与服务端路由注册和 OpenAPI 输出相同的完整 HTTP 路径。
- 生成的 OpenAPI 项目内嵌 Swagger UI 5 资产，并同时提供 `/openapi.json` 和 `/schema.json`，不依赖 CDN。
- HTTP JSON 客户端超时的构造通过 `NewJSONClientWithTimeout` 显式完成。
- 服务发现重试默认为一次尝试，并且在配置了额外尝试时只重试显式分类的瞬时错误。
- 服务发现 endpoint 构造函数返回自有的 closer，并在启动后台工作之前校验必需的依赖和时序选项。
- v2 文档以任务为导向，不再重复 v1 发布历史、临时路线图或会话快照。
- Extend 扫描以项目清单作为主要能力来源，并在变更之前报告文件系统或所有权漂移。
- 生成的 Go SDK 暴露带稳定状态码和响应体字段的 `APIError`，与 TypeScript SDK 错误契约对齐。
- MCP Streamable HTTP 现在强制执行初始化生命周期、协议版本、浏览器 Origin 策略和客户端 sampling 能力。日志级别是会话范围的，服务器消息使用单一活动 SSE 流。
- `kit` 和生成的 HTTP 服务器使用流安全的默认值，带有限的头部读取且无默认响应写截止时间。
- Consul 注册器操作返回错误；instancer 关闭会取消并加入活动阻塞查询。
- 生成的 Go SDK 以结构化方式解析 URL 并限制响应体大小。
- 生成的仓储只接受从模型派生的排序字段。

### 修复

- 提示词渲染回调不再在持有提供者锁时运行。
- Consul 重试等待响应关闭，重复的 `Stop` 调用是安全的。
- Endpointer 关闭不再与已关闭通道上的生产者发送竞争。
- Endpointer 关闭等待其更新循环，并释放端点缓存仍然持有的所有客户端资源。
- 端点缓存不再就地排序调用方切片，也不再向调用方暴露其内部端点切片。
- 生成的环境变量值在远程加载之后仍保持为最高优先级的配置来源。
- 生成的 Go 文件在写入格式错误的部分文件之前使生成失败。
- 追加服务和追加模型会刷新 OpenAPI、JSON Schema 和 TypeScript 客户端产物，而不是让生成的契约过期。
- 服务、模型和中间件追加操作最后刷新项目清单，并拒绝存在未解决清单漂移的项目。
- 从数据库派生的契约 IR 现在与生成的 Go IDL 在可选创建字段、更新字段、列表查询参数和响应 JSON 形状上一致。
- HTTP 响应写入器拦截保留可选的流式接口，忽略重复的状态写入，并计入 `io.ReaderFrom` 的字节数。
- 缓冲的 HTTP 客户端解码失败会关闭响应体并取消请求上下文。
- gRPC 客户端在应用钩子的同时保留调用方提供的出站元数据。
- MCP 工具触发的 sampling 现在使用来自请求上下文的传输会话，工具执行失败返回 `isError: true` 结果。

### 移除

- v2 文档中的 v1 兼容性声明和 v1.0/v1.6 发布规划。
- 重复的架构、生成器设计、项目快照、路线图、稳定性、可观测性、安全和维护者文档。
- 重复的 HandyBreaker 和内置 Hystrix 实现；Gobreaker 是核心中唯一的熔断器适配器。
- 冗余的 `sd.NewEndpointCloser`；生命周期所有权是每个 `sd.NewEndpoint` 构造的一部分。
- Swagger 2.0 注解输出、`swagger_host` 和 `APP_SWAGGER_HOST`；Swagger UI 现在读取生成的 `/openapi.json` 契约。
- 非标准的 `microgen -skill` 选项、生成的 `skill/` 包、`/skill` 发现端点、仓库 AI `SKILL.md` 和专门的 skill 示例。OpenAPI/JSON Schema 仍是通用契约格式，而可选的交互运行时通过 MCP 暴露工具发现与执行。
