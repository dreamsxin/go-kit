# 变更日志

[English](CHANGELOG.md) | 简体中文

## [2.9.0] - 发布候选

分层。目标是各个部件可以单独使用，所以契约层不再携带策略，依赖方向由门禁保证而不是
靠文档描述。

### 变更

- 挂在 `Host` 上、却没有任何组件服务就绪探针的 `ReadinessProvider`，现在是 `NewHost` 的一个错误。
  它此前被收集然后丢弃：实现这个契约的意义就在于让编排系统能看到答案，而一个组件在报告"就绪"的
  探针背后默默预热，比它从未声称要预热更糟。`kit.ReadinessSink` 就是 Host 要找的那个契约——
  `Probes() *health.Registry`——HTTP 组件与 gRPC 组件都满足它。

- CSRF 令牌现在在有限时间内只为一个会话授权。`CSRFConfig.SessionID` 为必填，
  `TokenTTL` 默认 12 小时；HMAC 覆盖 nonce、签发时间与会话，因此为某个调用方铸造的
  令牌对另一个会被拒绝，泄漏的令牌也会失效。此前签名只覆盖一个随机 nonce，这意味着
  服务端签发过的每个令牌对每个用户永久有效——只要有任何办法往受害者 jar 里放一个
  cookie，就足以做登录 CSRF 或令牌固定。

  无法解析会话的非安全请求被拒绝。没有会话的安全请求照常服务但不铸造令牌，令牌随登录
  后的第一个请求到达。

- 铸造 CSRF 令牌的响应声明 `Cache-Control: no-store` 与 `Vary: Cookie`。标识某个会话的
  `Set-Cookie` 不是共享缓存可以重放给下一个用户的响应。

- `SecurityHeadersConfig.AssumeHTTPS` 与 `CSRFConfig.AssumeHTTPS` 声明 TLS 在上游终止。
  HSTS 只对 HTTPS 请求发出，CSRF 的同源检查也要与请求自身的来源比较，而当本进程在负载
  均衡器之后提供明文服务时，这个 scheme 是它观察不到的。来自 `NewTrustedProxy` 的转发
  scheme 依然优先——它是测量，而不是声明。

- 最低 Go 版本为 1.26.0。每个模块的 `go` 指令、workspace、CI 通道、README 徽章以及
  生成项目模板一并更新。
- 已评审的 API 快照只哈希导出声明，不再哈希 doc comment 正文。此前正文在哈希内，因此
  改一个注释里的错别字就是一次发布门禁事件，任何文档改进都要重刷快照。真正值得保留的
  是测试在结构上给不了的那个保证：没有任何东西断言"导出的就是这些、且只有这些"，而
  已发布的库无法收回一次误导出。注释是否准确属于评审问题。`TestDeclarationsOnly*`
  从两侧钉住新行为——改正文不动哈希，新增、删除、改名、改签名都会动。

- `endpoint` 只导入标准库。它定义了 `Endpoint`、`Middleware` 与 `Chain` 是什么，却
  同时导入 `apperror`，导致没人能只要这三个而不一并接受框架的错误分类体系。现在它使用
  结构化分类契约 `interface{ ErrorKindName() string }`——这正是 `apperror` 本来就
  推荐给"不能依赖它"的调用方的那个契约，也是生成 SDK 使用的那个。

  对应用的影响：

  - `endpoint.ErrCircuitOpen`、`ErrBulkheadFull`、`ErrBackpressure`、
    `ErrRateLimited`、`ValidationError` 以及舱壁的等待错误实现
    `apperror.KindNamer`，不再实现带类型的 `apperror.Kinder`。HTTP 状态码与 gRPC
    code 不变：两个编码器都是先读 `Kinder` 再回落到 `KindNamer`。按带类型契约做匹配的
    代码需改用字符串契约。
  - `endpoint.DefaultRetryable` 只读 `ErrorKindName`。`apperror` 两个都实现，所以对它
    没有变化；只实现 `Kinder` 的自定义错误在线上仍被正确分类，但不再被重试。
  - `DefaultPanicHandler` 返回框架自有的已分类错误而不是 `*apperror.Error`。它的
    kind（`internal`）、code（`endpoint.panic`）与消息均不变。

### 新增

- `transport/http` 接管请求 ID 关联：`RequestIDHeader`、`MaxRequestIDLength`、
  `RequestIDValidator`、`DefaultRequestIDValidator`、`NewRequestID`、`RequestID`、
  `ValidRequestID`，以及钩子 `ExtractRequestID` / `RequestIDExtractor` / `EchoRequestID`。
  头名称、信任策略与生成器原先都在 `kit`，因此建立在传输包之上的服务——文档推荐的 `kit` 之后
  那条路——要关联一个请求就得把三者全部重写一遍。`kit.RequestIDValidator` 现在就是传输包的那个
  类型，`kit.MaxRequestIDLength` 就是它的常量，现有配置照旧编译。

  校验器拒绝控制字符不是为了美观：这个 ID 会被回写到响应头，携带 CR 或 LF 的值就是一次头注入。
  测试把这点写明了。

- `kit/grpc` 提供标准 gRPC 健康服务。`grpc.health.v1.Health/Check` 从组件的探针注册表作答，
  每次调用都实际求值，而不是读某个人记得去设置的状态，因此 `grpc_health_probe` 与 Kubernetes
  原生 gRPC 探针可以用与 HTTP 服务 `/readyz` 同一个答案来编排一个纯 gRPC 服务。带服务名的查询
  返回 `NotFound`——注册表描述的是进程而不是其中某个服务；`Watch` 未实现，因为它必须轮询检查
  才能合成状态变迁。

- `health` 包持有探针引擎：liveness 与 readiness 检查的 `Registry`、带每检查超时的并发求值、
  单飞门控与 panic 收容、探针一直返回的那个 `Report` 结构，以及它的 HTTP handler 与 `Mount`。
  这些原来是 `kit` 里的 213 行，全部不可导出且绑死在 `*kit.HTTP` 上，因此 gRPC 服务根本没有
  就绪面，而由传输包直接组装的服务只能自己重写一份。现在 `kit` 是挂载这个注册表，而不是拥有它。

  `kit.HealthCheck` 就是 `health.Check`，`kit.DefaultHealthCheckTimeout` 就是
  `health.DefaultTimeout`，因此现有配置照旧编译。`kit.WithProbePaths` 把探针放到你选的路由上，
  `kit.WithoutProbes` 一条都不放，`kit.HTTP.Probes()` 返回那个注册表——足够在构造之后追加一个
  检查，或者把探针放到独立的管理监听器上。

- `kit.WithRegistrar` 与 `kit.RegistrarLifecycle`：在 Host 运行期间发布一个服务实例。
  `sd.Registrar` 与 `kit.Lifecycle` 一直都在，却没有相遇，于是每个服务都自己手写那层
  适配——同样的三行，一个服务写一遍。

  doc comment 里写了签名表达不了的两件事。把注册挂在真正承载流量的 server *之后*：
  组件按声明顺序启动、逆序停止，这样地址才会在监听已经 accept 之后才出现，并在监听消失
  之前先撤下。以及不要把 go-kit `sd/etcd` 那个 `Deregister(); Register()` 的写法带过来
  ——它绕的是那个实现：不带 TTL 时用 etcd 的 `Create` 注册，key 已存在就失败，所以非正常
  退出后重启注册不上。这里 `Register` 覆盖的是按实例区分的 key，由租约持有，并且它把错误
  返回而不是只记一条日志。

- `tools` 中的 `TestComponentsDoNotDependOnAssembly`：除 `kit` 与 `cmd/microgen`
  自身外，任何包都不得依赖它们，因此组件无法悄悄反向依赖装配层。只写在文档里的分层
  留不住。

### 性能

现在每个请求都会经过的路径都有了基准——`Chain`、JSON 服务端往返、`balancer.Pick`、
`feedback.Table`、`Metrics.Observe` 与 `TracingMiddleware`——因此一次优化可以被证明有效，
一次回退可以被看见。下列数字来自同一台机器上的 `go test -bench . -benchmem`，值得信的是
比例而不是绝对值。

- `feedback.Table` 读取不再加锁。一次选择要问每个候选的 load 或 score，而每一次询问都要在
  记录路径同用的那把互斥锁上取一次读锁。现在条目表以写时复制发布，每项测量是一个原子字段；
  记录、重置与保留仍然取那把锁，因此 in-flight 计数与退役生命周期的顺序保持原样。读者可能
  看到一次记录两侧的字段混合，这是负载启发式可以承受的。9 个候选实测：8 并发下每次选择的读
  取 566 ns → 46 ns，64 并发下 543 ns → 37 ns。读+记录的完整往返在 8 并发下
  1567 ns → 534 ns，64 并发下 1549 ns → 533 ns；`balancer.Pick` 配 LeastRequest 在 12 并发下
  779 ns → 489 ns。
- 客户端一次请求不再拷贝实例快照，也不再分配一个用不上的回调。
  `endpointer.Cache.InstanceEndpoints` 返回已发布的快照本身而不是它的拷贝——这个 slice 在每次
  发现更新时被整体替换、从不原地修改，因此读者持有它期间视图始终一致；不要修改它。
  `balancer.Pick` 在策略不保存反馈状态时返回共享的空回调，保存时返回一个带守卫的回调。
  RoundRobin 实测：单实例 130 ns/96 B/4 allocs → 55 ns/24 B/1 alloc，9 实例
  224 ns/552 B/4 → 102 ns/224 B/1，12 并发 203 ns → 87 ns。每候选都要查反馈表的 LeastRequest
  从 471 ns/7 allocs 到 372 ns/6。
- 关联标识用一个 context 值携带，而不是三个。`TracingMiddleware` 每请求写一个节点而不是最多
  三个，`TraceContextFromContext`、`TraceIDFromContext`、`RequestIDFromContext` 各做一次
  查找。后写的 `With*` 依然覆盖先前的值——因为它替换的是整组。实测：铸造新 trace
  495 ns/296 B/10 allocs → 341 ns/192 B/5 allocs；延续入站 trace 487 ns/9 allocs →
  317 ns/4 allocs；五中间件链从 1074 ns/14 allocs 降到 791 ns/9 allocs。
- 请求 ID 走与 span ID 相同的低分配 hex 路径，不再用 `fmt.Sprintf`。熵源失败时的降级路径现在
  产出符合 W3C 规范长度与字母表的标识；此前它把一个 64 位值补零拉长到 32 个字符。
- `endpoint.Metrics` 保留它那把互斥锁。改成原子计数试过并实测更慢——单线程每次记录
  23.6 ns → 46.4 ns，12 线程 60 ns → 109 ns——因为一把锁覆盖十个字段更新，而原子操作每个都要
  付一次竞争的 cache line。数字写在该类型的 doc comment 里，让这个想法不会被再次盲目尝试。

### 修复

- `NewCORS` 拒绝不透明的 `null` origin——`NewCSRF` 本来就拒绝它。sandbox 文档、
  `data:`/`file:` 页面以及被重定向洗过的请求都呈现这个 origin，允许它并带凭证等于给它们
  开了一条带凭证的跨域通道。
- 每条 CORS 应答都声明 `Vary: Origin`，包括拒绝与无 origin 的直通。否则共享缓存可能存下
  一条不按 origin 归键的 403，再把它投给一个合法来源。
- `TestEndpointHasOnlyStandardLibraryImports` 里有一处显式豁免，恰好放过了它本该拦住的
  `apperror` 导入。已删除。
- `apperror.KindNamer` 声称框架里每个分类点都先读 `Kinder` 再回落到它。endpoint 现在
  只读 `KindNamer`；注释已如实说明。

## [2.8.1] - 2026-09-05

对全部运行时 package 做了一轮契约审计。下列每一条都是代码与它自身文档化契约不
一致的地方。版本号取 patch 是因为这些改动是纠正而非新特性集；其中若干条确实改变
了可观测行为，已排在最前。

### 修复 - 行为

- `security`：匿名或没有身份的 subject 不再满足 `RequireAuthenticated` 与
  `RequireRole`。`Middleware` 也不再丢弃只带 roles 或 claims 的 subject——此前这
  会让一个已认证的调用者拿到 401。
- `interaction`：nil 的 `AuthorizerFunc` 改为拒绝而非放行。它只会以 typed nil 的
  形式落到 `Authorizer` 字段上，而 `AuthorizationHook` 自己的 nil 检查看不到这种
  情况，所以这原本是一条 fail-open 路径。
- `interaction`：当某个 hook 拒绝调用时，`Runtime.CallTool` 会对已经放行的 hook
  执行 `AfterToolCall`，使 `AuditHook` 记录这次拒绝。此前审计 sink 完全看不到拒绝。
- `kit`：panic 的健康检查被报告为不健康的检查，而不是让进程退出。检查跑在各自的
  goroutine 中，而探针是未认证请求。
- HTTP 错误编码器：实现了 `transporthttp.PublicMessager` 的错误现在自行决定它的
  消息，包括空消息——空表示"这里没有任何内容是给客户端的"。`apperror.WrapCause`
  因此兑现了它文档中"cause 保持内部"的承诺；对于不在该契约内的错误，500 以下仍
  回落到 `err.Error()`。
- `TextErrorEncoder` 通过与 JSON、纯文本编码器相同的规则解析消息。此前它在 500
  也会应用 `PublicMessager`，并且把 499 命名为 "HTTP error" 而不是
  "Client Closed Request"。
- `integrations/grpc`：`StatusError` 给出的公开消息只包含上游 code，不包含上游
  description，与 `client.HTTPStatusError` 保持一致。
- 超限请求体在 JSON 与 raw codec 两条路径上都归类为 413，与
  `ParseMultipartForm` 以及 `RawBodyCodecWithMaxBytes` 的文档一致。此前经
  `JSONDecodeError` 是 400，直接抛出时是 500。
- `endpoint.TimeoutMiddleware` 与 `sd/retry`：非正的 timeout 表示不施加 deadline，
  而不是把一个已过期的 context 交给调用。
- `endpoint.BackpressureMiddleware`、`endpoint.InFlightMiddleware` 与
  `sd/retry.Retry`：非正的上限被 clamp 到 1，与 `BulkheadMiddleware` 原有行为一
  致。此前上限为 0 会拒绝每一个请求。
- `sd`：包装型 strategy 或 balancer 在丢弃一次成功的内层 `Pick` 时会释放它的
  `Done`。`selector.Filtered`、`feedback`、`selector` 与 `balancer` 各自都在拒绝
  分支上泄漏了一个 in-flight 预留。
- 成功路径的响应编码器会忽略超出 100-999 的 `StatusCoder` 值，而不是把它交给
  `WriteHeader`——后者会 panic。
- `interaction`：缺少 `Sessions`、`Events` 或 `Tools` 的 `Runtime` 报告
  `ErrRuntimeNotConfigured`，而不是在第一次调用时 panic；`StartSession` 在无法发
  出启动事件时会释放它已创建的 session。
- `observability/otel`、`observability/slog`、`integrations/zap`：panic 的
  endpoint 被报告为 panic。此前 otel span 以 Unset 结束且不记录错误，zap 记录
  "endpoint call succeeded"，slog 什么都不记录。
- `DefaultErrorEncoder` 只在返回的错误本身实现 `json.Marshaler` 时才启用该逃生口。
  此前 `errors.As` 会遍历整条链，一个可序列化的 cause 就能替换整个响应体——这是绕过
  消息规则的唯一路径，也正是 `apperror.WrapCause` 要防的东西。
- 超限请求体的错误不再在非 JSON 路由上说 "json"。同一个有界 reader 也保护 protobuf
  等原始请求体，而 500 以下消息会直接上线。它仍然可以用
  `errors.Is(err, ErrJSONBodyTooLarge)` 匹配。

### 变更 - 生成代码

需重新生成才能生效；它们违反的运行时契约见上面的条目。

- 生成的中间件链在最内层安装 `endpoint.FailerMiddleware`，这样 `Failer` 响应不会在
  传输层回答错误的同时被生成的指标与日志算作成功。
- 生成的指标通过 `endpoint.RecordingMiddleware` 记录进同一个按 operation 标签化的
  collector。此前是每个 operation 一个未标签化的 collector 存在未导出的 map 里，导致
  `SnapshotFor` 与 `Operations` 永远为空，生成项目里没有任何代码能读到这些计数。现在
  生成一个 `Metrics()` 访问器暴露该 collector。
- 生成 SDK 的 `APIError` 满足传输错误契约：`StatusCode`、`ErrorKindName`、
  `PublicMessage` 与 `Retryable`。此前转发它的服务只会得到一个笼统的 500，且上游响应
  体被嵌进消息里。它的 `StatusCode` 字段改名为 `Status`，因为 `StatusCode` 是契约要求
  的方法名。
- 生成 SDK 的 `WithTimeout` 改为按调用施加 context deadline，因此在
  `WithHTTPClient` 替换了客户端时依然生效——此前那种情况下它是静默失效的。
  `WithTimeout`、`WithHTTPClient` 与 `WithMaxResponseBodyBytes` 现在对错误输入采用
  同一条策略：忽略并保留默认值。

### 新增

- `endpoint.FailerMiddleware` 与 `endpoint.ResponseError`，以及
  `Builder.WithFailer`。`Failer` 响应到达传输层时错误为 nil，因此指标、熔断器和
  重试都把它算作成功。安装在最内层时，该中间件先把它转换出来，且不改变客户端可
  观测到的任何东西。
- `security.Subject.Authenticated`：区分"subject 存在于 context 中"与"principal
  已被建立"的检查。
- `sd.Release`：包装型 strategy 用来交回自己无法使用的 `Done` 的辅助函数。
- `interaction.ErrRuntimeNotConfigured`。

### 修复 - 文档

doc comment 在本仓库属于已评审的 API 快照，因此单独记录而不并入上面的条目。

- `JSONErrorEncoder` 没有 doc comment；描述它的那段被挂到了
  `JSONErrorEncoderWithKindMapper` 上。`RawBodyCodec` 的注释从句子中间开始。
- `sd/endpointer` 声称 round-robin 与 random balancer 接受较窄的 `Endpointer`，
  `sd/client.NewEndpoint` 声称它组合了一个 `Endpointer`。该模块中的一切都返回
  `InstanceEndpointer`。
- `SubjectFromContext` 声称对匿名调用者返回 false。
- `client.KindForStatus` 声称自己是 `server.HTTPStatusForErrorKind` 的逆映射。那个
  映射并非单射：`KindAlreadyExists` 与 `KindConflict` 都回答 409，
  `KindDeadlineExceeded` 与 408 请求超时共用 504。因此有两个 kind 在 HTTP 上无法
  round-trip，而在 gRPC 上全部可以。
- `apperror.Kind` 没有说明空 kind 会被规范化为 `KindInternal`——所有构造函数与
  `ErrorKind` 都会这么做。
- `docs/concepts.md` 把 `endpoint.ValidationError` 的 400 归因于 `PublicMessager`，
  实际来自 `apperror.Kinder`。`transport/README.md` 与 `docs/errors.md` 的正文仍在
  描述已被取代的消息规则。

## [2.8.0] - 2026-09-04

这是当前 `go-kit/v2` 产品线的首次公开发布。仓库以单一 Go module 发布：

```text
github.com/dreamsxin/go-kit/v2
```

运行时 package、传输适配器、服务发现 provider、可观测性适配器和
`cmd/microgen` 统一由同一个 module 和根 tag 进行版本管理。`examples`、
`tools` 与 `tools/contractcheck` 仍是仓库内部 workspace module。

### 包含内容

- `Service -> Endpoint -> Transport` 服务架构。
- 类型化 HTTP JSON 处理器、自定义编解码、SSE、gRPC 适配器以及 Go/TypeScript 客户端。
- 传输无关的应用错误，HTTP 与 gRPC 使用一致的错误映射。
- endpoint 中间件：校验、超时、追踪、指标、恢复、限流、熔断、降级、背压、
  舱壁和重试。
- 服务发现快照、端点缓存、负载均衡、重试、健康检查、被动摘除、反馈以及长连接统计。
- 交互运行时：会话、工具、资源、提示、授权、审计钩子和 MCP Streamable HTTP。
- 配置、OpenAPI、JSON Schema、数据库脚手架和确定性的 `microgen` 扩展流程。
- 明确的生命周期所有权、有界停机、请求关联和 package 级依赖门禁。

### 发布契约

- 唯一发布 tag 是根 `v2.8.0` tag。
- 历史 v2 module tag（根标签和嵌套标签）已删除。
- 未来版本使用一个版本和一个根 tag。
- 行为与 API 变化记录在本文件中；本次首次公开基线不维护单独的迁移历史。
