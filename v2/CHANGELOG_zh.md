# 变更日志

[English](CHANGELOG.md) | 简体中文

## [2.22.0] - Release Candidate

服务发现这一子树从未被审计过。对它做的两次审计——负载均衡路径与韧性路径——开启了这个里程碑。

### 修复

- **一次带重试的调用可能把已经收到的响应丢掉。** 那个循环同时 select 调用的 context 和这次尝试的
  结果 channel，而两个 case 同时就绪时 select 是等概率选择的：一次尝试恰好在预算到期的同一瞬间完成，
  就有一半的概率被丢弃并报成 `context.DeadlineExceeded`。对非幂等请求来说这是最糟的一种报告——写已经
  发生了，调用方被告知没有发生，而它无法为自己从未得知的事情做补偿。现在一次尝试会在 outcome 回调之前
  把结果交给循环，于是 `Done` 里的部署代码无法把这个窗口拉宽；并且循环在服从 context 之前，会先把已经
  送达的结果取出来。
- **预算已经花光的调用仍然会再派出一次尝试。** 那个循环把启动尝试 goroutine 作为第一条语句，之后才看
  context，于是已经放弃的调用方——或者在预算到期之后才返回的退避计时器——仍然会导致一次 `Pick` 和一次
  没人会读结果的上游请求。现在在派发之前先检查 context。

## [2.21.0] - 2026-09-08

生成器产出什么，也是框架的一部分。这个里程碑来自两次审计：第一次真正看代码生成路径，以及一次只看
"特性被组合起来用"而不是单独用时会发生什么。

### 变更

- **API 兼容性门禁不再在"没有 tag 的检出"上绿着通过。** `TestAPICompatibilityWithLastRelease` 在找不到
  `v2.*` tag 时 `Skip`，而 skip 就是绿——于是浅克隆、`git archive` 导出、或者不拉 tag 的 CI 检出，都在
  静默地关掉那道守着整个导出面的门禁。现在它会失败，并说明你处在两种情形的哪一种；其中诚实的那一种
  （还什么都没发布）是 `-allow-missing-release-tag`，而且必须手打。
- **生成契约的快照不能再被"带外"刷新。** `UPDATE_CONTRACT_SNAPSHOTS=1` 和那个 flag 并列生效，于是留在
  shell profile 里、或漏进 CI 的一个值，就让这三份快照永久自我祝福，而任何命令行上都看不出来。现在只剩
  flag。
- `TestEveryContractSnapshotHasALiveCaller` 拒绝"没人读的快照"。比较逻辑在一个由集成测试调用的 helper
  里，删掉一行调用不会在任何地方失败：`.sha256` 文件留在树里没人读，而承载这些调用的测试又不在
  `RELEASE.md` 点名的门禁里，于是"检查门禁是否还存在"的那个契约测试也看不见它。新门禁从快照那一端走，
  而不是从调用者那一端走，并且它自己也写进了 `RELEASE.md`，所以删掉它会被抓到。
- **导出面门禁现在点名"哪些包动了"。** 它按包各存一份 digest，而失败时打印两段三十五行的十六进制，
  于是审查者的第一件事是用肉眼 diff 十六进制，之后才谈得上回答这道门禁真正要问的问题。现在失败会列出
  发生变化的包，并标出新增和消失的包，同时指向 `go doc -all`。存储形式仍然是 digest：这让失败可读，
  并没有让快照可审。

### 修复

- **v2.20.0 把生成的 MCP 装配弄坏了。** 收紧 Origin 检查之后，原本被服务的浏览器客户端被拒，而没有
  任何模板提到 `AllowedOrigins` 或 `TrustRequestHost`——在 `cmd/microgen` 里 grep 这两个词零命中。
  生成的 `main` 现在带上这两个字段，以及"为什么请求自身的 `Host` 不被信任"，于是读生成代码的人能看见
  这个决定，而不是去 debug 一个 403。
- 生成的停机路径从不调用 `mcpHandler.Shutdown`，于是 MCP 会话和它们的 goroutine 不被排空。现在它跑在
  HTTP 停机之前：一个会话握着一条开着的 SSE 流，先关会话才能让 `Shutdown` 结束，而不是把预算烧完再
  fallthrough 到 `Close`。
- 生成的成功编码器是 `json.NewEncoder(w).Encode(response)`。它不设 `Content-Type`——于是 `net/http`
  把 JSON 嗅探成 `text/plain`，和同一个生成器写在它旁边的 OpenAPI 文档自相矛盾——并且在响应类型上的
  `StatusCoder` 或 `Headerer` 被读之前就写了 body，把每个回答钉死成隐式 200，也丢掉了 204 无 body
  规则。现在改为调用 `server.EncodeJSONResponse`，也就是携带那些 promise 的那个编码器。
- 带 `--grpc` 生成的项目根本编译不过：`go.mod` 既没 require `google.golang.org/grpc` 也没有
  `google.golang.org/protobuf`，而生成的 `main` 两个都 import。现在都 require 了，版本取自本 module
  自己在用的那个。
- `kit.HandleSSETyped` 从来没把组件级的 JSON server options 传给 SSE server，而 `kit` 的包文档却把它
  列在"组件的 JSON server options 全都生效"那一组里。于是组件级安装了 `ProblemJSONErrorEncoder` 的部署，
  在每条 JSON 路由上得到 `application/problem+json`，在 SSE 解码失败时得到一个普通信封——一个服务两套
  错误约定。现在传了，组件在前，于是路由自己的仍然胜出。
- **一条 SSE 路由用两种方式回答自己的错误。** 解码失败走的是这条路由的 options 解析出来的编码器，而中间件
  拒绝被硬编码成 `JSONErrorEncoder`——于是同一条路由把 400 渲染成文本、把 401 渲染成 JSON，而自己装了
  编码器的部署只在其中一条路径上拿到它。拒绝现在用这条流自己的编码器，也就是
  `server.SSEServer.ErrorEncoder` 报告的那一个。因此没装任何编码器的组件，回答拒绝的方式和它其余路由
  回答错误的方式一致——如果你原本依赖那个 JSON 信封，这是一处可见的变化：通过 `WithJSONServerOptions`
  安装 `ServerErrorEncoder(server.JSONErrorEncoder)` 即可保留它，而且是在每条路由上，而不是在一条路由的
  一条路径上。
- **一条在第三个事件上死掉的流被记成成功。** 那个"把一条流当成一次请求"的桥，无论流做了什么都返回
  `(struct{}{}, nil)`，于是每个指标、日志和 trace 看到的都是一次完成的请求。
  `server.SSEServer.ServeStream` 就是会把"结束这条流的那个错误"返回出来的 `ServeHTTP`，而桥把它交回链上。
  一旦 200 和若干事件已经 flush 出去，响应就不能再改——错误被记录，而不是被渲染——而"流还没开始就被拒绝"
  仍然是唯一会写出错误响应的情形。
- 来自 IDL 或数据库 schema 的文本，未经转义就进了生成的 Go 字符串字面量、结构体 tag 与注释。生成器有一个
  `escape` 辅助函数，只替换双引号——不管反斜杠，也不管换行——而且没有任何模板用它。含引号的文档注释会产出
  `Description: "a "user" record"`，那是**语法合法的 Go**，于是生成器自己的格式检查放它过去、到用户 build
  时才失败；含换行或以反斜杠结尾的会在前面的文件已写出之后中断生成；列名里的一个反引号会终止结构体 tag
  的原始字符串。`escape` 被替换为 `quote`（`strconv.Quote`，一次处理全部这些）、`comment`（折叠所有行终止符
  ——因为 `//` 注释到换行就结束，而 `.proto` 输出从不经过格式化器）与 `tag`（去掉原始字符串无法转义的字符），
  并且 `interaction`、`model`、`service`、`sdk`、`proto` 模板里全部十九处插值点都改用了它们。
- **生成的 OpenAPI 文档和生成的 handler 自相矛盾。** 每个 message schema 对 `additionalProperties`
  闭口不谈，而解码器设了 `DisallowUnknownFields`，而沉默的含义是"允许多余属性"——于是照文档生成的
  客户端，会被它旁边生成出来的那个服务拒绝。现在 message schema 都写 `false`。`ErrorResponse` 有意
  不写：错误编码器是部署方的选择，`ProblemJSONErrorEncoder` 写的是一份更宽的文档，把那个 schema 关起来
  就是承诺生成器并不产出的东西。
- **指针字段的可空性在三份产物里都丢了。** OpenAPI schema、JSON Schema 包、TypeScript SDK 都只写了
  值的类型，而 nil 指针会 marshal 成 `null`——生成的结构体不带 `omitempty`——于是文档描述的是服务并不
  产出的载荷，而 TypeScript 使用方的判空是照错误的形状写的。现在普通类型是 `["string", "null"]`，
  `$ref` 包进 `anyOf`（`$ref` 旁边的关键字并非所有实现都认），TypeScript 字段是 `T | null`。

### 文档

- `MICROGEN.md` 现在写明了把生成文档和生成 handler 连起来的三条规则，包括其中那条"是推导而非修复"的：
  `required` 由类型推导，"声明为指针"就是声明可选，而它是关于载荷的陈述，不是"服务端会拒绝缺省"的承诺。
  非指针字段上无法把缺省和零值区分开，于是缺失的 `count` 到达时就是 `0`——当"缺省"必须区别于"零"时，
  把这个字段声明成指针。
- `kit` 的注册指南对 `HandleSSETyped` 说过头了——而那段是本仓库两个版本前自己写的。它后来长出的两条注意
  ——硬编码的拒绝编码器、以及看起来永远成功的流——在上面被修掉了，而不是被记下来，所以指南现在写的是这个
  函数做什么，并只点出那条真实的限制：一旦流已经回答了 200，错误只能被记录，不能被渲染。
- 那个作为范本的 SSE 示例忽略了 `Stopping` 通告、只 select `ctx.Done()`，和 `kit/drain.go` 里的排空示例
  自相矛盾。照它写出来的流会一直跑到宽限期取消它的 context，那对客户端来说是连接被切断，而不是流正常结束
  ——而写 SSE 的人读的正是这个示例。

## [2.20.0] - 2026-09-08

请求里的任何东西都不能为自己作证。

### 变更——可能需要动作

- **`mcp.StreamableHandler` 不再放行与请求自身 `Host` 相同的 `Origin`。** `Host` 是调用方给的，拿它
  和 `Origin` 互相比较，正好绕过了 Origin 校验对一个本地监听的 MCP 服务器所要拦的攻击：DNS rebinding
  下浏览器会同时发 `Origin: http://evil.example` 与 `Host: evil.example`，而那个比较会说"是"。
- 这个行为现在是一个接缝，不是策略。当部署方确知 `Host` 可信时——有一个会重写它的反向代理，或者所在网络
  没有浏览器能到达——设 `StreamableHandler.TrustRequestHost = true`，行为与从前完全一致。否则就别开。
- **如果升级后某个浏览器客户端不再被服务**，把它的来源写进 `AllowedOrigins`。那是永远有效的答案，因为
  它不依赖任何由调用方控制的 header。如果你确实想要原来的行为，`TrustRequestHost` 会一字不差地恢复它。
- 不受影响：不带 `Origin` 的请求照样被服务——它不是从浏览器来的，没有浏览器强加的来源要检查——列在
  `AllowedOrigins` 里的来源也照样被服务。
- 发现它的那次审计把它归类为"已声明，但后果没写明"：结构体注释确实写了"同源请求总是允许"，但它没有说
  `Host` 值几个钱。没有任何测试走过这条捷径，这正是那个后果一直没被注意到的原因。声明了
  `mcp.request-host-is-not-trusted-by-default`。

## [2.19.0] - 2026-09-08

promise 没有覆盖到的地方。这个框架里的每一条行为都在源码里以 `// Stable: <id> — <promise>` 标记，
并写明覆盖它的测试，还有一道门禁检查那个测试确实存在。这个版本开启的里程碑来自问下一个问题：那个测试
真的断言了 promise 说的事吗？凡是没有的地方，缺口都不在文字里——而是一条没人走过的路。

### 修复

- MCP 的 `GET` 与 `DELETE` 从来没有到达 `MethodAuthorizer`，而 `mcp.method-authorization` 声明的是
  "每一个请求都会到达"。持有一个 session ID 的调用方，可以挂上服务端推送通知与 sampling 请求的那条流，
  或者终止任意会话，而部署方的策略完全不被问及。现在两者都会到达，名字是
  `mcp.MethodOpenStream`（`stream/open`）与 `mcp.MethodDeleteSession`（`session/delete`）——用的是
  没有任何 MCP 方法占用的命名空间，于是策略不必去匹配 HTTP 动词——拒绝是 403 且什么都没做，因为没有
  request id 可以回答。授权跑在会话查找之前，所以被拒绝的调用方学不到"哪些 session 存在"。原来的覆盖
  测试跑了十个无状态 POST 方法，一个 `GET`、一个 `DELETE` 都没有；现在有了。声明了
  `mcp.transport-operation-authorization`。
- SSE 分帧只按 LF 切行。规范里 CRLF、CR、LF 都结束一行，因此携带裸 CR 的载荷被放进了一个 `data:` 行
  内部，而一个守规范的客户端会在那里结束字段、把其余部分当成无名字段读——载荷被静默截断。现在三种终止符
  都会开启新的 data 行。声明了 `http.sse-line-terminators`。
- SSE 的 data、注释文本与事件名都没有转义，因此其中任何一个都能结束自己的帧、往流里注入一个自选的事件
  ——任何会流式输出"受调用方影响的文本"的 handler 都可达。现在 data 与注释整体保持为 data 与注释（注释
  里的空行变成又一行注释），而携带行终止符的事件名是一个错误而不是一个帧：字段值装不下终止符，所以那个
  名字是调用方的 bug，剥掉它和原样透传都不是诚实的回答。声明了 `http.sse-no-frame-injection`。
- 错误响应可能带上两个 `Content-Type`。编码器先设一个，再用 `Add` 合并 `Headerer` 错误报出的 header，
  于是一个 `Headers()` 里带 `Content-Type` 的错误会产出没有客户端能解释的响应。body 是编码器选的，所以
  由编码器来命名类型——并且放在最后命名，在错误要求的其他一切都合并之后。三个调用点收敛成一个 helper。
  声明了 `http.error-single-content-type`。
- 一个本服务从未发出过的 MCP cursor 被当成 offset 0，于是跨目录变更持久化了 cursor 的客户端被悄悄递上
  第一页，并以为那是它的那一页。cursor 就是服务端作为 `nextCursor` 返回的那个偏移，所以非数字、负数、
  或已到/越过末尾的都是非法的：四个 list 方法现在都回 `-32602`——这是规范的要求，也是唯一能被客户端察觉
  的回答。现在每个 list 方法都由同一个 `listError` 支撑，于是同一个坏 cursor 不会在一个方法上是 invalid
  params、在另一个上是 internal error。声明了 `mcp.invalid-cursor-is-invalid-params`。
- 序列化不了的 MCP 响应会以空 SSE 事件或被截断的 body 到达调用方，调用方在等一个永远不来的答复，且什么都
  没记录。两条路径现在都先序列化再写，于是要么整个响应到达，要么一条点名故障的 internal error 到达。
  声明了 `mcp.response-is-whole-or-an-error`。
- `mcp.error-codes` 枚举了服务端会发的 code，却漏了 `-32021`——`stateless.go` 会返回它，而它自己的标记
  也承诺了它。两个标记对词汇表说法不一致；枚举里现在写上了它。
- 写在 `_test.go` 里的行为标记从来不被行为门禁评审，于是它在源码里读起来像一条 promise，实际却在冻结之外。
  唯一存在的那一条——`otel.metrics-agree-with-the-exposition`——被移到了它该在的 `NewMetrics` 上，并由
  `TestBehaviourMarkersLiveInNonTestSource` 让下一条这样的标记响亮地失败，而不是被跳过。

### 变更

- `http.multipart-limits` 原文是"超限的请求体或文件是 413 request_too_large"。文件那一支一直发的是
  `request_too_large.file`，而那是更有用的回答——它说出了撞到的是哪个上限——所以现在是 promise 补上两个
  code，而不是让行为退化到更含糊的那个。
- `MultipartLimits` 现在写明了哪个字段限的是"工作量"。一个文件的大小只有在它那一段被读完之后才知道，
  所以 `MaxFileBytes` 限的是回答，而 `MaxBodyBytes` 限的是一个请求能让进程往磁盘写多少。这个不对称是
  真实存在且没被写明的；"边流式读边按段强制上限"这个做法被考虑过并被否掉，理由记在 roadmap 里。拒绝确实
  会把临时文件和已解析的 form 一起带走，这一点现在被声明了：
  `http.multipart-file-refusal-leaves-nothing-behind`。

## [2.18.0] - 2026-09-08

被别人读。这个版本开启的里程碑来自一次换位审阅：第一次上手的人、写业务逻辑的人、运维它的人、
扩展它的人，以及给它写测试的人。结果是运维视角被服务得最好，写测试的人最差；而文档的缺口不在
散文里，在 godoc 里。

### 新增

- `endpoint.Clock` 让时间变成一个测试可以决定的输入。`time.Now()` 过去被生产代码直接调用，于是部署方
  测不了一个退避间隔或一次 token 过期，除非真的睡过去——这个仓库自己也一样。nil 表示真实时间，因此不
  关心这件事的服务什么都不用写，行为也一点没变。
- `endpoint.NewManualClock` 返回一个由测试用手推的时钟，带 `Advance`、`Set` 与 `Pending`。它和契约放
  在一起交付，而不是塞进只给测试用的包：没人能拿到的接缝不是接缝——应用可以接受这个框架的组件所接受的
  同一个时钟。在被测代码从其他 goroutine 读它的同时从测试 goroutine 推进它是安全的。
- 接缝有三处：`endpoint.WithRetryClock`（重试尝试之间的等待）、`endpoint.Metrics.Clock`（快照报告的
  `LastRequestTime`）、`httpsecurity.CSRFConfig.Clock`（token 何时铸造、TTL 何时检查）。CSRF 那个是
  结构化声明而不是 import，因为那个包有意不依赖这里的任何东西——任何带 `Now` 方法的值都能用，
  `ManualClock` 也在内。
- 时钟故意不决定的，是真实工作花了多久。`Observation.Duration` 与被记录的请求耗时仍然是量出来的：一个
  能把它们缩短的时钟，会让 recorder 报告一件关于系统的不实之事。声明了
  `endpoint.clock-nil-is-wall-clock`、`endpoint.manual-clock-advance-fires-due-timers`、
  `endpoint.retry-clock`、`endpoint.metrics-clock` 与 `security.csrf-clock`。
  `docs/testing.md`（中英）都加了对应章节。

### 弃用

- `kit.HandleSSE` 弃用，改用 `Handle`。它的函数体一直只有一行——`h.Handle(pattern, handler)`——因此
  不携带名字所暗示的任何 SSE 行为，而顺着名字从 `HandleSSETyped` 找过来的读者会静默丢掉端点中间件链
  与 recorder。`Handle` 是同一次调用，但名字写明了自己跳过什么。它是保留而不是删除，因为 API 兼容性
  门禁说得对：一个已发布的符号就是一个承诺。
- `kit.JSON` 与 `kit.JSONTyped` 弃用，改用 `kit.NewJSONHandler` 与 `kit.NewJSONTypedHandler`。它们
  返回一个未注册的 handler，却和 `HandleJSON` / `HandleJSONTyped` 只差一个动词——这不足以区分"会挂
  路由"和"不会挂路由"的函数。`New` 前缀与它们包装的 `httpserver.NewJSONServer` 一致。

### 变更

- `kit` 的包文档现在按"实际做什么"把全部十一个注册入口分了组——完整链、逃生舱、两者都不是——于是这个
  区别由 API 自己说明，而不是依赖读者先找到 customization 那张表。

### 文档

- `apperror` 的包注释，对于业务代码 import 的第一个包来说只有四行。现在它回答读者带着来的问题：一个
  错误携带的三样东西各自要怎么斟酌、空 kind 为什么变成 KindInternal、500 处消息会怎样、什么时候该用
  `WrapCause` 而不是 `Wrap`，以及结构化的 `KindNamer` 契约意味着"不 import 这个包也能被正确分类"。
- `sd/endpointer` 与 `sd/instance` 原本完全没有包注释，`sd/balancer`、`sd/retry`、`sd/client` 各只有
  一行。这五个包现在都说明了自己是干什么的、以及和邻居的区别——为什么负载均衡和选择是两个包、为什么
  `sd/retry` 每次尝试都重新选而 `endpoint.RetryMiddleware` 只重复同一个 endpoint、以及
  `InvalidateOnError` 的零值到底意味着什么。
- `DOCS_INDEX.md` 与 `docs/index.md` 现在互为超集。此前各自都漏掉了对方列出的文档，于是能不能找到一篇
  文档，取决于你打开了哪个索引。

## [2.17.0] - 2026-09-08

不错的默认值。这个版本开启的里程碑来自一次全局审阅，而不是一个功能想法：在十六个里程碑把行为
一一钉住之后，剩下的弱点不是缺能力，而是少数几处"默认值是从标准库继承来的"。

### 新增

- `kit.HandleJSONEndpointWithBodyLimit` 与 `kit.HandleJSONTypedWithBodyLimit` 让单条路由可以
  接受不同的请求体上限。这不是加一个开关，而是堵一个坑：`kit.WithJSONMaxBodyBytes` 是组件级的，
  于是只有一条上传路由的服务过去只能改用 `kit.Handle` 注册，而那条路会静默跳过端点中间件、
  记录器，以及组件装好的 JSON server options——为了设一个体积上限，代价是丢掉可观测性。新的注册
  方式和其他 JSON 路由走同一条路，唯一的区别就是上限。上限为零或负数会 panic：不限制请求体不是
  一个可以从单条路由夹带进来的取值。声明了 `kit.per-route-body-limit`。
- `server.ProblemJSONErrorEncoder` 写出 RFC 9457 的 `application/problem+json`。它是被提供的，
  不是被装上的：`ErrorResponse` 已被声明为稳定，是这个框架上每个服务今天都在发的东西，而服务说
  哪种错误格式是它的客户端要一起承担的决定——这是接缝，不是框架策略。状态码映射、500 处的脱敏、
  `Headerer` 与 `Retry-After` 都没变，机器可读的 code 作为扩展成员保留，因此按 `code` 分支的客户端
  照样能用。换来的是校验失败一直携带、而信封无处安放的那份字段清单：`endpoint.ValidationError` 的
  `[]FieldError` 变成 `errors`，不再被压成一条 `message`。`type` URI 标识的是一份得有人发布、
  并且要一直可解析的文档，所以不会替你发明——传 `nil` 写 `about:blank`，需要真实 URI 时给一个
  `ProblemTypeResolver`。`server.ProblemFromError` 与 `server.WriteProblemJSON` 已导出，供部署
  自己写 encoder（包括带自定义 kind 映射的那种）。声明了 `http.problem-media-type`、
  `http.problem-document-shape`、`http.problem-redaction` 与 `http.problem-encoder-parity`。
  `docs/errors.md`（中英）都加了对应章节。
- `endpoint.KeyedRateLimiter` 让"按调用方的额度"变得可表达。`RateLimiter` 限的是整个进程——
  `Allow()` 不带 key——所以按租户、按 API key 的限流不只是没有，而是通过被支持的契约根本无法表达，
  一个吵闹的调用方就能把所有人都拒掉。新契约是 `AllowKey(ctx, key)` / `WaitKey(ctx, key)`，用
  `KeyedRateLimitMiddleware`、`DelayKeyedRateLimitMiddleware`、`Builder.WithKeyedRateLimit`
  或 `WithDelayKeyedRateLimit` 装上，key 由 `RateLimitKeyFunc` 给出，因为"什么算一个调用方"该由
  部署决定。它是第二个契约，而不是给第一个契约再加两个参数：不带 key 的限流器没有按 key 的状态可查，
  不该通过忽略一个参数来假装自己有。空 key 在空 key 上限流，绝不豁免，因此少一个 header 不是一条
  出路；key 函数为 nil 会在装配处 panic，因为那会静默地又变回进程级限流。
  `KeyedRetryAfterReporter` 给出按 key 的等待时长；当所有 key 补充速率相同时，不带 key 的
  `RetryAfterReporter` 依然有效。令牌桶仍然属于应用。声明了 `endpoint.keyed-rate-limit` 与
  `endpoint.keyed-rate-limit-shares-one-bucket`。

### 变更

- 由 `transport/http/client` 构造的客户端不再使用 `http.DefaultClient`。那件事对服务有两处不对，
  而且都不是策略：它是进程里任何库都能改的包级变量，别人设的超时或换掉的 transport 会影响到这些
  调用；而 `http.DefaultTransport` 每个 host 只留两个空闲连接——对"打很多 host 各一次"的工具是
  对的，对"每个请求都打同一个上游"的服务是错的，表现出来就是应用代码里解释不了的延迟。
- 现在用的是这个包自己的客户端：`MaxIdleConnsPerHost` 100、空闲 60 秒回收、dial 与 TLS 握手都有
  上限。`client.DefaultClient()` 把它暴露出来，于是包外的调用也能共用同一个连接池；
  `client.NewTransport()` 返回一个新的 transport，供需要代理或 `tls.Config`、但想保留连接池配置的
  部署作为起点。相关常量都已导出并命名。
- 它仍然不设 `Timeout`，而且现在把理由写明：在那里设超时，等于用框架发明的数字给进程里每一次调用
  设上限，且在调用现场看不见。deadline 属于调用方——`NewJSONClientWithTimeout`、
  `endpoint.Builder.WithTimeout`，或你自己派生的 context。`SetClient(nil)` 现在回落到本包的客户端，
  而不是进程级默认值。

声明了 `httpclient.pool-is-ours` 与 `httpclient.no-invented-deadline`。`PRODUCTION.md`（中英）写明了
变了什么、以及哪些仍然要你自己设。

## [2.16.0] - 2026-09-07

一次纠正，以及那句话曾经替谁开脱。

### 修复

- `kit/grpc` 实现了 `grpc.health.v1.Health/Watch`：先立即给出当前 serving 状态，之后每次变化发
  一条，数据来源是 `Check` 求值的同一个探针注册表。Milestone 14 曾声明"`Watch` 未实现，因为按
  gRPC 健康编排的工具调用的是 `Check`"——这对 `grpc_health_probe` 和 Kubernetes 原生 gRPC 探针
  成立，但对最重要的那个消费者不成立：grpc-go 自己的客户端健康检查调用的是 `Watch`，而且在收到
  `UNIMPLEMENTED` 时会把连接标为 ready 并从此不再问。于是开启了健康检查的客户端永远不会知道某个
  实例开始 drain 了——而这正是 drain 通告存在的意义。
- `kit/grpc.HealthWatchInterval` 写明了这条流的分辨率：一秒；并且就绪检查每个间隔求值一次、由所有
  watcher 共享，而不是每个 watcher 每条消息求值一次——一群客户端不该把一次数据库 ping 变成负载。
  最后一个 watcher 离开后轮询就停。它是常量而不是 option，因为健康服务是组件自己注册的；需要不同
  行为的部署应当自己构造 `grpc.Server`——这个限制现在被写下来了。
- `docs/lifecycle*.md` 里的传输对照表改成了事实。

## [2.15.0] - 2026-09-07

不重启的轮换。Milestone 13 交付了进程内 TLS，然后自己把缺口写了下来：证书文件只读一次，所以
续期意味着重启进程。那句话很诚实，但那个行为不好——证书是按计划过期的，而续期它的平台会在
运行中的服务底下替换文件，并期待服务注意到。这个版本让证书可以在监听器服务期间被替换，并且
不让一次坏的续期夺走任何人的监听器。

### 新增

- `kit.CertificateSource` 就是那个接缝：一个方法，每次握手都会问它，于是监听器启动时不会把
  证书的任何东西固定下来。证书从哪来仍然属于部署方——文件、密钥管理服务、ACME 客户端、给 SNI
  用的按名字映射——而 `kit.CertificateSourceFunc` 让闭包也能用。`kit.WithTLSCertificateSource`
  负责装上它，除了 `WithTLSConfig` 本来就有的最低版本默认值之外不强加任何东西。
- `kit.CertificateFiles` 是挂载 secret 所需要的那个文件型 source。`NewCertificateFiles` 先把
  证书对加载好，所以路径写错仍然是"带着路径的启动失败"，而不是某个客户端的握手失败；`Reload`
  在部署方说该看了的时候重读两个文件。什么时候说，刻意不属于框架：不装文件监听、不装定时器、
  不装信号处理器，因为每一个都是某些部署得绕开的策略。`Certificate` 从不碰磁盘，所以握手路径
  不会被磁盘拖慢或弄挂。
- 失败的 reload 返回错误，并继续服务已经加载的那一对。写了一半的 secret 只值一条日志，不值一次
  故障，而测试断言监听器仍然用上一张证书应答。
- 生成的服务通过同一个"每次握手"的接缝提供证书，并在收到 `SIGHUP` 时重读两个文件：成功记录路径，
  失败保留上一张证书。续期是 `kill -HUP`，不是一次发布。选 `SIGHUP` 是因为它本来就是运维会用的
  信号、续期任务也能发；进程不轮询，也不会替你去监听文件系统。
- 配置指南现在写明了：你配置的东西里哪些会被再读一次、哪些只读一次——证书按你的触发时机、
  `tls.Config` 的其余部分在构造时、地址与超时与路由在 `Start` 时、配置文件与环境变量每进程一次。
  第一组之外的任何改动都意味着一个新的监听器，也就意味着滚动重启——而那正是 drain 时序存在的理由。

## [2.14.0] - 2026-09-07

两个传输，一套契约。前十三个版本把 HTTP 表面在就绪、draining、流式、指标与 TLS 上都说清楚了，
而 gRPC 表面只跟上了其中一部分。缺口不在 RPC 层本身——它有分类错误、metadata 钩子和 trace
传播——而在它周围的运维契约：`Host.Drain` 跳过 gRPC 组件，`kit.Stopping` 对 gRPC 流返回 nil，
scrape 对 RPC 一句话都说不出来，生成代码构造的是一个没有凭据、没有健康服务的裸
`grpc.NewServer()`。这个版本把能补的缺口补上，把补不了的差异写清楚。

### 新增

- `kit/grpc.Component` 实现了 `kit.Draining`，于是 `Host.Drain` 会按它对其他组件一样的
  反向挂载顺序把"要停了"通告给 gRPC 服务器。在此之前它做类型断言、断言失败、然后静默跳过。
- gRPC handler 用和 HTTP handler 一样的调用读取停止信号：`kit.Stopping(stream.Context())`
  在 draining 开始时关闭，于是流可以在宽限期之内自己收尾。unary 与 stream 拦截器都会携带它，
  并且装在 trace 提取之前、调用方自己的 option 之前，所以经由任何一条路径到达的 handler 都有。
- `kit.WithStopping` 现在是导出的：这是传输层要实现的那个接缝。`kit` 的 HTTP 组件用它，gRPC
  组件用它，想让 `kit.Stopping` 对自己 handler 生效的自定义传输，也用它传入一个自己会关闭的
  channel。没有人装信号的地方 `Stopping` 依旧返回 nil，而从 nil channel 接收会永久阻塞，所以
  同一个 `select` 在所有地方都是对的。
- drain 一个 gRPC 组件是通告，不拒绝调用，这是刻意的：在 drain 延迟里收到 `UNAVAILABLE` 的
  客户端会重试，而且可能重试到同一个实例，因为路由层还没跟上。真正让流量挪走的是 readiness
  失败，而 gRPC 健康服务已经在报告它。  `Component.Shutdown` 也会通告，所以被直接停掉的组件同样会告诉它的 handler；而它现在自己给
  等待设界，不再依赖 grpc 的 `GracefulStop`：那个调用在等 handler 返回时持有服务器自己的
  互斥锁，而 `Stop` 需要同一把锁，所以一个永不返回的 handler 会让这一对死锁——建立在它上面的
  "有界停机"根本没有界。组件通过自己的拦截器统计在途调用，关闭监听器以阻止新连接进来，在预算
  用尽之前等在途调用结束，然后关闭传输，并通过包装 `kit.ErrShutdownIncomplete` 报告打断了
  多少个——和 HTTP 组件报告的是同一个错误。

- `grpcserver.Observation`、`grpcserver.Recorder`、`grpcserver.RecorderFunc`、
  `grpcserver.RecordingUnaryInterceptor` 与 `grpcserver.RecordingStreamInterceptor` 让 gRPC
  传输拥有了 HTTP 传输早就有的 recording 契约。标签是完整方法名，它不需要 HTTP 路由那样的
  "防守"：方法集合由服务定义固定，调用一个不存在的方法在拦截器运行之前就被拒绝，所以流量
  无法增加序列。流在结束时被记录并带 `Stream: true`，因为流的 duration 是一段生命周期而不是
  一次延迟，把两者平均在一起得到的数字谁都不描述。
- `observability/metrics/grpc.Recorder` 把这些 observation 桥接进 exposition 端点渲染的
  `endpoint.Metrics` 收集器，于是纯 gRPC 服务的 scrape 终于说得出话来。它刻意是一个独立
  package，并有自己的依赖门禁：`observability/metrics` 仍然只靠标准库就能用，所以纯 HTTP 服务
  不会因为"想被 scrape"而拿到 gRPC 库。描述调用方的状态码——`NotFound`、`InvalidArgument`、
  `PermissionDenied`、`Canceled` 之类——不算错误，这和"把 4xx 挡在 HTTP 错误率之外"是同一个理由。
- 生成服务的 gRPC 端口现在和第一个端口一样是被配置过的。2.13.0 的证书对同时保护两个监听器——
  用同一个 `tls.Config` 构造一个 `grpc.Creds`，所以不匹配仍然会带着路径让启动失败。标准
  `grpc.health.v1` 服务被注册，并报告 HTTP 探针报告的同一个就绪状态，包括 draining 一开始就
  `NOT_SERVING`，于是纯 gRPC 的部署终于有东西可以让探针去探。trace 提取会安装；设置了
  `server.metrics_path` 时，recording 拦截器也会安装。而且每个 server 拿到自己那份停机预算，
  不再抢同一个 deadline：以前 HTTP 排空慢一点，gRPC 就什么都不剩了。
- 两个传输之间的差异是被声明的，不是被发现的。`docs/lifecycle_zh.md` 新增一张表，列出每个传输
  分别回答了什么——就绪、draining 通告、停止信号、停机预算、TLS、指标、tracing——包括那些不好看
  的行：只有 HTTP 会有连接逃出停机，而 gRPC 健康的 `Watch` 没有实现，因为按它编排的工具调用的是
  `Check`。有门禁把这件事钉住：契约从任一传输上被去掉会编译失败；`kit` 新增了没有为两者分类过的
  导出接口，测试会失败，而每一个"不需要"都记录了理由。

## [2.13.0] - 2026-09-07

一个可以直接放到公网上的监听。v2 目前只服务明文 HTTP：库与生成代码里没有任何 TLS，所以每个
部署都在别处终止 TLS，而没有任何文档说出这件事。这个立场本身站得住，但它是"未声明"的——正是
这一点会让一个服务带着假设而不是决定被部署上线。这个版本让"自己终止 TLS 的监听"成为可能，写明
它对协议协商做了什么、没做什么，并钉住"被 hijack 的连接"对一个承诺会结束的 shutdown 意味着
什么。

### 新增

- `kit.WithTLS`、`kit.WithTLSConfig`、`kit.DefaultTLSMinVersion` 与 `HTTP.ServesTLS` 让组件
  可以自己终止 TLS。无法加载的证书在构造时失败并带上出错的路径，而不是变成别人客户端日志里的
  一个握手错误；HTTP/2 会随 TLS 经 ALPN 一起到来——开启 TLS 的服务同时也换了协议版本，这件事
  值得在流式客户端发现之前就知道。
  策略留在部署侧：传入的 `tls.Config` 按原样服务，包括密码套件、`ClientAuth`、SNI 与
  `GetCertificate`。唯一被强加的值是"MinVersion 为零时变成 TLS 1.2"，因为 Go 在这里的零值对
  服务器意味着 TLS 1.0，而没有哪个部署是想要那个的。该 config 会被克隆，所以之后修改调用方的
  值不会改变已经在服务的内容。明文仍是默认，而且现在是一个被文档化的立场，不再是一个假设。
- 生成项目新增 `server.tls_cert_file` 与 `server.tls_key_file` 配置键（`APP_TLS_CERT_FILE`、
  `APP_TLS_KEY_FILE`，默认都为空即关闭），生成的入口在它们被设置时服务 TLS。两个要么都设、
  要么都不设：只设一个会校验失败，而不是在客户端马上要讲 TLS 的端口上服务明文；证书对在监听器
  开始服务之前加载，所以路径写错会带着路径让启动失败。启动横幅现在报告它实际在服务的 scheme，
  而不是假定 `http`。`PRODUCTION.md` 新增 TLS 终止一节，说明什么时候该在进程内终止、什么时候
  该交给代理，以及两者分别对就绪、轮换（文件只读一次——除非你提供 `GetCertificate`，轮换就意味着
  重启）、健康探针的 scheme、以及升级连接意味着什么。

### 声明

- 被 hijack 的连接不会被 drain，这件事现在写在"做出了它打破的那个承诺"的代码旁边。
  `http.Server.Shutdown` 不跟踪被 handler 接管的连接，关闭监听器不会关闭它，2.11.0 加入的硬
  关闭也够不到它——于是 `Shutdown` 很快返回 `nil`，而升级后的连接照样在传字节。Milestone 11
  的承诺按其原文仍然成立（没有 shutdown 路径会在**它拥有的**连接仍打开时返回）；缺的是说清
  "被 hijack 的连接不属于它拥有的那批"。缝隙留在做了升级的那个 handler 上：它和别人一样收到
  `kit.Stopping`，而这个信号在 draining 开始时就关闭，所以它能在宽限期内关掉自己的 socket。
  测试把两半都钉住了，包括不好看的那一半。
- 协议协商是被写明的，不是被发现的。在 TLS 上 Go 会经 ALPN 提供 HTTP/2：带 flush 的响应——SSE、
  流式 MCP 传输——仍然是流式的，由 h2 的流层分帧而不是 chunked transfer encoding，现在有测试在
  它不再成立时失败。撑不过去的是协议升级：h2 没有 `101 Switching Protocols`，所以基于 hijack
  的协议只在明文监听器上可用。明文 HTTP/2 是刻意不提供的——协商它需要"预先知道"的客户端或一次
  升级交换，而真正想要它的场合，前面的代理已经拥有那个决定。

## [2.12.0] - 2026-09-07

能被拿来做判断的数字。v2 已经能通过 OpenTelemetry 推送遥测，但多数部署真正在用的拉取模型
——一个 scrape 端点——一直留给每个应用自己接，结果是每个服务用略微不同的名字报略微不同的
数字。这个版本给 scrape 表面一个接缝，把客户端库挡在核心依赖路径之外，并写明标签里可以放
什么——无界标签正是一个指标端点拖垮采集方的方式。

### 新增

- 生成项目新增 `server.metrics_path` 配置键（`APP_METRICS_PATH`，默认为空即关闭），生成的入口
  会把它接上：逐路由 recording 通过 `httpserver.DecorateRoutes` 安装在注册处，exposition 则挂在
  mux 本身上，于是 scrape 报告的是服务而不是它自己。默认关闭，因为该端点会公开路由名与流量形状
  ——把它放到 admin 监听或网络策略之后。`cmd/custom_routes.go` 里的自定义路由直接注册在 mux 上、
  不被记录：那个文件是你的，要不要包装你自己写的 handler，由你在那里决定。
- `httpserver.RouteRegistrar` 与 `httpserver.DecorateRoutes` 指名了"逐路由中间件必须安装在

  哪里"：注册处——只有在那里 handler 和它的路由 pattern 同时在作用域内。registrar 只要
  `http.ServeMux` 提供的那两个方法，所以 `*http.ServeMux` 天然满足它，调用方也可以传入一个
  装饰器；于是 mux 分派到的就是被包装后的 handler，这才让 `http.Request.Pattern` 是匹配到的
  路由而不是空值。包在 mux 外面的中间件根本看不到 pattern——这就是"逐路由指标"与"一条叫 `/`
  的序列"之间的差别。
- `metrics.HTTPRecorder` 把 HTTP 传输层的 recorder 桥接到 `endpoint.Metrics` 收集器，于是只接了
  HTTP 层的服务——生成项目并没有把每个 handler 都包进 endpoint 链——也能喂给 exposition 读取的
  同一份数字。它是翻译而不是第二次测量：一个请求只产生一次观测。未匹配任何路由的请求根本不记录，
  所以漏洞扫描不会显示成服务真的服务过的流量；结果判定用传输层能如实报告的那一种——5xx 记为错误，
  4xx 不记，因为"告诉调用方不行"的服务器是正常工作的服务器。
- 拉取与推送之间的一致性由测试保证，而不是靠意图：同一批观测经 `endpoint.RecordingMiddleware`
  同时进入 exposition 和 OpenTelemetry 适配器，然后比较计数、总耗时与 operation 标签。两者确实
  不同的地方，包文档写明了——这里的计数器在重启时从零开始（收集器在内存里），耗时是 sum/count
  而不是桶，所以 p99 来自 OpenTelemetry 直方图，而不是来自一次 scrape。
- 基数问题有测试，而不是一句注意事项：同一路由模板下的五十个不同 URL 只产生每个结果一条序列，
  没有任何请求路径进入标签，而未匹配任何路由的请求根本不被记录——于是扫 `/.env` 也无法增长
  序列数。

## [2.11.0] - 2026-09-07

有意为之的停止。一个要离开的进程应该先说出来，把已经接下的活做完，并且自己结束宽限期，
而不是在它拥有的连接仍然打开时就返回。这个版本把关闭从"一个被取消的 context"变成一个有
声明顺序的过程——同时把 draining 究竟意味着什么，留给知道答案的组件。

### 新增

- 生成项目新增 `server.drain_delay` 配置键（`APP_DRAIN_DELAY`，默认 `0s`），入口现在使用与
  框架相同的顺序：先让 readiness 失败，等待 drain 延迟，然后停止。优雅关闭用尽预算时会关闭
  剩余连接，而不是打印一条超时日志、带着还开着的连接退出。该键有文档、有校验（负值会让启动
  失败），并被"把每个已文档化的 `APP_*` 键应用到已构建二进制"的门禁覆盖。
- `kit.Host.Drain`、`kit.Draining`、`kit.WithDrainDelay`、`kit.Host.Draining` 与
  `kit.ErrDraining` 把停止变成一个过程：在任何东西被拆解之前，readiness 开始失败，并且
  每个挂载的 `Draining` 组件都按反向挂载顺序被通知。随后 `Run` 等待配置的 drain 延迟——
  默认为零，也就是 Host 在此之前的行为——让上游有时间在监听关闭之前重新读取 readiness
  或服务发现。draining 期间 liveness 保持通过，因为一个正在收尾在途工作的进程不该被重启。
  `Shutdown` 也会宣布，所以手工组装 Host 的调用方不会去拆一个还在声称自己就绪的组件。
  组件的 `Drain` 错误会被上报，流程继续：一个无法停止接活的东西仍然必须被关闭。
- `kit.Stopping`、`kit.ErrShutdownIncomplete` 与 `kit.HTTP.Addr` 让宽限期真的会结束，而
  不是指望它结束。`Stopping(ctx)` 在 draining 开始时关闭，于是流或长轮询可以自己收尾，
  客户端看到的是"流结束"而不是"连接断了"；在 kit HTTP 组件之外它是 nil，`select` 也能正确
  处理。对什么都不看的 handler，`HTTP.Shutdown` 不再在连接仍然打开时返回一个超时错误：它
  取消请求 context，给 handler 一小段时间收尾，关闭剩下的连接，并通过
  `ErrShutdownIncomplete` 报告打断了多少个请求。`Addr` 报告监听器实际绑定的地址——这是绑
  定 `:0` 之后唯一能得知端口的方式。

### 变更

- 用 `kit.WithRegistrar` 挂载的注册现在在"进程宣布停止"时注销，而不是在拆解时注销，于是实
  例是在 drain 延迟之前离开服务发现，而不是之后。先注销再立刻关闭监听，会让所有已经缓存了
  该地址的对端撞上一个关闭的端口；延迟正是为了覆盖这个窗口，所以注销必须排在前面。对跳过
  宣布的调用方，`Shutdown` 仍然会注销，且不会注销两次。
- `Host.Shutdown` 现在给每个组件分配"剩余预算的等额一份"，而不是把同一个截止时间一路传下
  去。共享截止时间让第一个被停止的组件可以花掉全部预算，排在它后面的组件拿到的是一个已经
  过期的 context——代码上读起来优雅，生产上就是硬关。份额每次重新计算，所以提前返回的组件
  会把时间留给其余组件；调用方没设截止时间时，也不会被强加一个。

## [2.10.0] - 2026-09-07

可以被检查的承诺。兼容性契约不再只是文档：它覆盖的每个表面都指名了保证它的门禁，v2
承诺的协议行为都声明在守护它们的代码旁边。这项检查发现的第一件事，就是 MCP 那条承诺
已经落后两个修订版本——因此本次发布在冻结的那一版之外，也说当前规范。

### 新增

- MCP 2026-07-28 无状态版本，与 2025-06-18 并行提供。`MCP-Protocol-Version` 头逐请求
  选择请求模型；缺少该头时选择 2025-06-18——头成为强制要求晚于该版本，因此既有客户端
  在十二个月弃用窗口内行为不变。新版本下不铸造、不读取、也不要求任何会话：请求在
  `params._meta` 中携带自己的协议版本、客户端身份与客户端能力，工具通过
  `mcp.IdentityFromContext` 与 `mcp.ClientCapabilityFromContext` 读取。
  `server/discover` 为"想先了解能力再决定"的客户端取代握手；`initialize` 与
  `notifications/initialized` 被作为"已退役"应答并指向它。该版本的传输就是 POST，
  为会话而存在的 GET 与 DELETE 返回 405 与 `Allow: POST`。
- 2026-07-28 下基于头的路由：`Mcp-Method` 重复 JSON-RPC 方法，`Mcp-Name` 重复它寻址的
  目标，网关因此无需解析请求体即可路由、计量与鉴权。头与请求体不一致的请求以 400
  `invalid_routing_header` 被拒绝——否则按头做出的按工具决策，会落到与实际执行不同的
  调用上。
- 可缓存的目录：2026-07-28 下 `tools/list`、`prompts/list`、`resources/list`、
  `resources/templates/list` 与 `resources/read` 带 `ttlMs` 与 `cacheScope`，由
  `StreamableHandler.ListCacheTTL` 和 `ListCacheScope` 配置。scope 默认 `private`，
  因为目录可能是按授权过滤的。
- 多轮请求（MRTR），让工具在没有常开流的情况下也能向调用方提问。工具返回
  `interaction.InputRequired`，携带问题与希望被回传的状态；传输层以
  `resultType: "input_required"` 加 `inputRequests` 与不透明的 `requestState` 应答，
  调用方带 `inputResponses` 重发调用。工具再跑一遍，`interaction.InputAnswersFromContext`
  返回答案与状态——它因此是一道门禁而不是被挂起的调用，两轮之间不持有任何东西。调用方
  没有声明能回答的问题会以 `-32021` 与 `data.requiredCapabilities` 被拒绝，而不是照问。
  在 2025-06-18 下同一个返回值是错误，并指名承载它的版本。
- `interaction.EventToolInputRequired`：为"停下来提问"的调用发出。未完成的调用既不是
  结果也不是错误，其日志以 `Info` 记录"需要输入"——按设计工作的对话不应该把值班叫起来。
- `StreamableHandler.RequestStateKey` 与 `RequestStateTTL` 对中途提问交给调用方的
  `requestState` 做认证。配置了密钥时，被改动或已过期的状态在工具看到它之前就被拒绝；
  是否校验由服务端配置决定，而不是由令牌自身携带的内容决定，因此调用方无法通过去掉签名
  来绕过。没有密钥时状态按返回的原样接受，文档对此直说。服务同一地址的所有实例需要使用
  相同的密钥。
- `mcp.MethodAuthorizer`、`mcp.MethodRequest`、`mcp.MethodAuthorizerFunc` 与
  `StreamableHandler.Authorizer` 决定哪些调用方可以到达哪些 MCP 方法。两个版本上的每一个
  请求——包括通知——都会带着方法、目标名、HTTP 头，以及该请求被服务时的 context 到达授权
  器，且发生在任何注册表、provider 或工具被触及之前；拒绝是 `-32001`，没有 id 可回应的
  请求则是 403。策略自己从 context 读取主体，因此 `mcp` 对身份如何表示不持意见，也绝不
  从请求 body 里取一个主体。默认什么都不授权：`Authorizer` 为 nil，框架拥有的是这个问题
  的形状和拒绝的形状，而不是答案。在 2025-06-18 上，SSE 流与会话删除不逐请求授权——它们
  作用的会话在 `initialize` 创建时就已被授权。
- `mcp.Extension`、`mcp.ExtensionMethod`、`StreamableHandler.RegisterExtension`、
  `mcp.ClientExtensionFromContext` 与 `mcp.MetaFromContext` 实现了 2026-07-28 的扩展
  框架，而不实现任何一个扩展。部署声明自己的扩展——反向 DNS id、它自己的版本、它自己的
  配置对象、它自己的命名空间方法——传输层则在 capabilities 与 `server/discover` 中声明
  它、在两个版本上路由它的方法、并像对待其他方法一样对它授权。注册之前扩展全部关闭；
  未注册扩展的方法保持 `-32601`，于是客户端看到的是一台从来没有该扩展的服务器，并回落
  到核心协议。规范的 `io.modelcontextprotocol/` 命名空间对应用是被拒绝的；离开自己命名
  空间、或抢占核心方法的方法在注册时就被拒绝；不认识的 `params._meta` 键既不被拒绝也不
  被丢弃：它会到达实现，因为它属于别人写的扩展。

### 变更

- 生成的 `main` 在 `Config.Validate` 之前应用所有命令行标志。`-auto-migrate` 原本写在
  校验之后，是唯一一个其值从未经过校验的标志。
- `internal/docs/RELEASE.md` 中的兼容性契约为六个表面各指名门禁，被指名的门禁不再存在
  时 `TestCompatibilityContractNamesItsGates` 失败。四十九条协议行为以
  `// Stable: <id> — <承诺>` 的形式声明在其实现旁边，经过评审的集合保存在
  `tools/testdata/protocol_behaviour.txt`；两条刻意保持不稳定的行为以同样形式说明。
- API 兼容性门禁能区分"新增"与"破坏"，而不再把两者都报成 changed：删除或重排结构体字段、
  为接口新增方法会失败，新增结构体字段与常量块重新对齐则通过。重排会连同原因一起报告——
  未加 key 的复合字面量会继续编译并赋给别的字段。
- [开源协议](docs/licenses_zh.md) 说明 MIT 覆盖的范围，以及使用方会一并继承的每个直接依赖
  的协议，其中两个 MPL-2.0 被指名，并写出把它们带进来的包。
  `TestDependencyLicensesAreDocumented` 从每个模块自己的协议文件判定协议类型，在文档与
  模块图不一致时失败；`TestProjectLicenseIsOneText` 保持仓库与模块两份 `LICENSE.txt` 一致。
- 生成的项目不承担任何协议义务。`microgen` 写下的内容属于使用者——不要求声明、不要求署名、
  不产生回流义务，也不写 `LICENSE` 文件，选择权留给他们——而项目所 import 的框架仍是 MIT。
  生成的 README 会这样写明，`TestGeneratedOutputCarriesNoLicenseNotice` 会在模板开始输出
  声明、或经过评审的生成布局中出现协议文件时失败。
- TypeScript 类型检查门禁能区分"编译器给出了否定结论"与"编译器自己死了"。类型错误照旧在
  第一次尝试就失败；被信号杀死或在自身运行时内失败的进程——例如 tsc 7.0.2 以 `0xC0000409`
  退出——会重试一次，若再次死亡则报告为编译器崩溃，而不是生成 SDK 里的类型错误。

## [2.9.0] - 2026-09-05

分层。目标是各个部件可以单独使用，所以契约层不再携带策略，依赖方向由门禁保证而不是
靠文档描述。

### 变更

- 依赖测量的负载均衡由 `sd/feedback.Measure` 装配，那些静默什么也不做的组合被消掉了。
  `Measured` 在一个已经绑定到发现订阅的 `Table` 之上提供 `LeastRequest`、`Scored`、
  `SlowStartWeighted`、`Balancer` 与 `Ranking`，而 `Measured.Eject` 加入的正是那条订阅，
  不再另开一条可能报出不同快照的订阅。`Table.Score`、`Table.Load` 与 `Table.FirstSeen`
  被移除，因为裸函数恰恰是它们唯一不工作的形态：分数函数交给 `selector.Scored` 而不经过
  `Table.Wrap`，就没有任何东西写入 table，于是每个实例得分相同、选择退化成随机；而在一张
  没有跟随发现的 table 上做 slow start，每个实例永远看起来是全新的，每个权重都坍缩成 1。
  `Table.Scored` 以"已经 Wrap 好的策略"取代前者；`Table.LeastRequest`、`Table.Wrap`、
  `Table.Stats` 与 `feedback.Follow` 保持不变，供自己装配这些部件的调用方使用。
  `sd/balancer` 未变，仍是不需要测量的那些策略所在的层——`NewScored` 依然接收 ORCA 或 LRS
  那类进程外上报，不需要 table——这也是它没有 `NewLeastRequest` 的原因。

- 反馈统计自己就跟随注册而不是健康判定。`health.Checker` 现在实现了
  `sd.DerivedInstancer`，`feedback.Follow` 与 `Measure` 据此把过滤视图解析回它所派生的
  Instancer。把健康检查视图交给 `Follow` 曾经让主动与被动健康检查相互抵消：对 retainer
  来说撤下与注销无法区分，于是探测把实例撤下的那一刻，正是摘除它的那些测量被丢掉的时刻，
  实例带着一份干净记录回来。

- 服务发现背后的订阅、错误宽限与失效状态机只有一份实现。`sd/selector.Subscription`、
  `sd/endpointer.Cache` 与 `feedback.Follow` 共用它，`sortInstances` 从五份变成一份，
  etcd 与 consul 提供者通过 `sd/instance.Cache` 广播而不再各自持有一份私有副本。没有导出
  API 变化，也没有行为变化；受评审的 API 快照只多了那个新的 internal 包。

- `sd.Registrar` 现在声明自己的冲突语义。`sd.Conflict` 给出三种——`ConflictOverwrite`、
  `ConflictCreateOnly`、`ConflictCompareAndSwap`——由 `etcd.ConflictRegistrarOptions`
  选择；被拒绝的注册返回包装了新增 `sd.ErrConflict` 的错误，etcd 的守护逻辑随之停止
  重注册，而不是反复重试一个属于别人的身份。默认仍是覆盖，因为它是唯一能从"非正常退出
  留下的 key"里自愈的设置。仅创建与比较并交换由一次 etcd 事务保证：仅创建在 key 不存在时
  写入，比较并交换在 key 不存在、或仍是本 client 写下的内容时写入——于是租约丢失依然能恢复，
  同时不会去抢一个已经易主的 key。Consul 只支持覆盖，并且现在把这点写进了文档：它的 agent
  接口按 service ID upsert，写入前无法比较。`etcd.Client.Register` 把语义作为参数接收，
  因此自行实现该接口的代码需要补上这个新参数。

- 生成代码把类型不匹配归类成错误，而不是 panic。端点适配器、gRPC 编解码、gRPC 服务端方法
  与 SDK 的 gRPC 客户端改为通过 `endpoint.TypedEndpoint.Wrap`、`endpoint.Unwrap` 或带
  comma-ok 检查并上报 `endpoint.NewTypeAssertError` 的断言来转换。中间件把响应换成另一个
  类型时——缓存层、返回 nil 的兜底——原先会在请求处理器内部 panic，看起来像框架崩溃，而它
  其实是接线错误。重新生成即可获得该行为；手工改过的生成文件保持原样。

- trace context 现在无需接线就能穿过每种传输。`kit.NewHTTP` 在每条路由上提取进来的
  `traceparent`，`kit/grpc.New` 装上提取用的一元与流式拦截器——以 chain 方式安装，
  所以 `grpc.UnaryInterceptor` 仍然留给调用方——`integrations/grpc/client.NewClient`
  默认把 context 里的 trace context 注入出方向 metadata。需要显式打开的传播，就是会在
  某个人忘记的第一跳断掉的传播。

- `oteladapter` 的 instrument 遵循 OpenTelemetry 约定。`go_kit.endpoint.duration`
  的单位从毫秒改为秒；失败的调用是同一条 `go_kit.endpoint.requests` 序列携带
  `error.type`——即错误通过 `interface{ ErrorKindName() string }` 自报的 kind——而不是
  第二个计数器。`go_kit.endpoint.errors` 与 `outcome` 属性已移除：错误率现在是同一个
  instrument 上的比值。读旧名字或旧单位的仪表盘需要更新。

- `slogadapter.Telemetry.Middlewares` 的类型改为 `[]NamedMiddleware`，每一项自带
  它所报告的标签，不再按位置贴标签。原先追加第四个中间件再调用 `Apply` 会越过标签
  列表并 panic。`TelemetryConfig.Operation` 现在只在装配日志维度时必填——它命名的是
  日志记录。

- 严格 JSON 服务端对自己不说的媒体类型在读取 body 之前回 415，由
  `JSONDecodeOptions.RequireJSONContentType` 控制——`StrictJSONDecodeOptions` 默认开启，因此
  所有高层 JSON 辅助函数都有。此前声明 `text/plain` 的请求只要字节恰好能解析就被接受。完全
  不带 `Content-Type` 的请求仍被接受：无 body 的请求本来就不带，强制要求会拒绝本来正确的请求。
  接受 `application/json`、任意 `+json` 后缀，以及 UTF-8 的 charset 参数；其他 charset 不接受，
  因为 JSON 按 RFC 8259 就是 UTF-8。

- 空 body 会说自己是空的。此前消息是字面量 `EOF`——那是解码器的处境，不是调用方的错误——
  code 也是 `bad_request.invalid_json`。现在消息为 `request body is empty`，code 为
  `bad_request.empty_body`，并且 `ErrJSONBodyEmpty` 可用 `errors.Is` 匹配。

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

- `observability/otel` 现在装配的是整条流水线，而不只是中间件：`Setup(ctx, Config)`
  构建 tracer/meter provider、走 gRPC 或 HTTP 的 OTLP exporter，以及由 `ServiceName`、
  `ServiceVersion`、`Environment` 构成的 resource；把它们连同 W3C trace context 与
  baggage propagator 装到全局；并返回带 `Tracer()`、`Meter()` 与幂等 `Shutdown` 的
  `Providers`。`Config.Signals` 选择 traces、metrics 或两者，`SpanExporter` /
  `MetricReader` 可替换 OTLP 流水线。正确的 OpenTelemetry 接线原先是应用里几十行代码，
  而最常被漏掉的正是全局 propagator——没有它，服务发出的 span 下游谁也接不上。

- `oteladapter.NewHTTPMetrics` 按 HTTP 语义约定记录 `http.server.request.duration`，
  携带 `http.request.method`、`url.scheme`、`http.route` 与 `http.response.status_code`，
  于是响应状态可从指标告警。`transport/http/server` 新增了它所依据的契约——
  `Observation`、`Recorder`、`RecorderFunc`、`RecordingMiddleware`——`kit.WithHTTPRecorder`
  把它装在每条路由上，那是匹配到的 pattern 唯一存在的地方。未匹配任何路由的请求不带
  `http.route`，也不会退化成原始 URL 路径。

- `integrations/grpc` 双向传播 W3C trace context：`TraceparentKey`、
  `ExtractTraceparent`、`InjectTraceparent`、`TraceparentUnaryServerInterceptor`、
  `TraceparentStreamServerInterceptor`、`TraceparentUnaryClientInterceptor`。gRPC 服务
  原先完全没有关联管路，一条 trace 在第一个 gRPC 跳就断了。

- `interaction.Runtime.WithLogger` 用调用方的 `*slog.Logger` 上报每一次工具调用，
  字段与请求路径一致——`duration`、`success`、`error`、`trace_id`、`request_id`——于是一次
  MCP 工具调用能和承载它的 HTTP 请求对上。失败或被拒的调用记在 Error 级：别处不会报告它
  ——工具失败是以结果的形式回到模型，而不是一个传输错误。logger 为 nil 时什么都不记，
  这也是默认值。

- `slogadapter.Signals` 用来选择 `NewTelemetry` 装配哪些遥测维度——`SignalTracing`、
  `SignalMetrics`、`SignalLogging`，零值表示全部——因此指标来自 OpenTelemetry meter 的
  服务可以只取日志维度，而不会把每次调用记两遍。


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

- `microgen -from-db` 校验自己是否拿到了 `-dsn`。没有它就会一路走到
  `sql.Open(driver, "")`，抛出驱动的空 DSN 解析错误——那句话没有指向调用方能处理的任何东西。
  `-dbname` 仍是可选的：像 SQLite 这样基于文件的数据库没有库名可给。
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
