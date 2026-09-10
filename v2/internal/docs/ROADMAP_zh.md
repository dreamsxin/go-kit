# 实施路线图
[English](ROADMAP.md) | 简体中文

本文是 go-kit v2 唯一实施路线图，只记录长期产品里程碑，不记录临时会话过程。

## 产品方向

- 保持 `Service -> Endpoint -> Transport` 作为唯一的运行时架构。
- 让应用可以单独采用各个包，也可以通过 `microgen` 生成完整服务。
- 偏好显式所有权、经过校验的配置、确定性生成、感知取消的生命周期以及安全的并发默认值。
- 只添加可跨无关服务复用的能力。可选集成保持在核心依赖路径之外。

## 已完成基础

- 独立的 `/v2` Go 模块和由上下文所有的生命周期。
- 组件一致的 `kit`、endpoint 中间件、HTTP/gRPC 传输、服务发现、交互运行时和 MCP Streamable HTTP。
- 只读的数据库内省和可选迁移。
- 带外部构建覆盖的确定性 UTF-8 项目生成。
- 一个规范化 IR 驱动路由、Go 客户端、Go SDK、OpenAPI 3.1、JSON Schema 2020-12、TypeScript Fetch 客户端和可选 MCP 工具。
- 保留用户文件的增量 service/model/middleware 扩展。
- 最小化的可选生成器默认值、严格的 IDL 校验、有界的客户端响应以及由传输所有的交互会话清理。

## 里程碑 1（已完成）：生成项目身份

目标：取代特性推断，成为生成项目的主要事实来源。

- 生成带版本号的 `.microgen/manifest.json`。
- 它记录源模式、模块路径、启用的能力、路由前缀、服务、模型、中间件和生成器所有的产物。
- `microgen extend -check` 对照文件系统校验清单，并报告可操作的漂移。
- 完整生成和每个 extend 操作都会刷新清单。

已完成：生成的项目现在能解释其配置和所有权，而无需扫描 Go 源码寻找配置线索。

## 里程碑 2（已完成）：契约质量

- 生成的 OpenAPI 3.1 文档在集成测试中被解析为 v3 模型，生成的 JSON Schema 2020-12 包针对 Go IDL、Protobuf 和数据库源被编译。
- 发布工作流用固定的编译器版本对生成的 TypeScript 客户端做类型检查。
- Go 和 TypeScript SDK 在发布工作流中执行相同的路径、查询、请求体、头部和非 2xx 错误行为契约。
- Go IDL、Protobuf 和数据库源针对生成器所有的公开契约产物有经过评审的 SHA-256 快照。

已完成：已发布的契约产物经过机器校验、行为检查，并受到保护以避免未经评审的确定性漂移。

## 里程碑 3（已完成）：可选运维适配

- `observability/slog` 提供标准库结构化 endpoint 日志，而不替换核心 zap logger API。
- `observability/otel` 提供 endpoint 追踪与指标；没有任何核心包 import 它。
- 提供者初始化、资源、导出器、采样和关闭仍留在应用装配层。

已完成：应用可以显式采用标准可观测性，而不使用这些适配器的服务保持核心依赖路径精简。

## 里程碑 4（已完成）：可选 HTTP 安全

- `security/http` 提供可组合的可信代理/客户端 IP、IP 策略、CORS、签名双重提交 CSRF 和安全头中间件。
- 保持认证和应用授权策略在框架核心之外。
- 代理信任、浏览器 cookie 范围、中间件顺序和 SSE/MCP 交互均有文档记录并有专项测试覆盖。

已完成：常见的 HTTP 加固可以通过标准 `http.Handler` 组合显式启用，而无需更改 endpoint 或传输契约。

## 里程碑 5（已完成）：v2 发布收口

- `make verify` 运行完整功能验证、生成项目和契约检查、固定的 TypeScript 检查、竞态测试、vet、模块 tidy 检查、UTF-8/链接检查以及经过评审的公开 API 快照。
- README 示例、CLI 行为、生成物所有权和导出的运行时包由可执行检查或快照覆盖。
- `make release-check-clean` 在打标签之前验证已提交的 v2 范围。
- 运行时收口现在包括 MCP 生命周期/版本/Origin/能力检查、单流 SSE 投递、会话级日志级别和工具结果错误语义；HTTP/gRPC 元数据和流式资源所有权；可取消的 Consul 阻塞查询；以及流安全的 `kit` 默认值。
- 生成器收口现在包括有界的 SDK 响应读取、URL 解析、仓储排序白名单、有效的日志/超时接线、可选的入站中间件、安全的低速率限制器突发、预先绑定的服务器监听器、数据库资源关闭以及流安全的生成 HTTP 默认值。
- 完整重新生成保护用户所有的服务、装配、配置和 README 文件，同时清单枚举所有生成器所有的 endpoint 和 transport 产物。
- MCP 传输会话拥有并释放一个运行时会话；泛型 JSON 客户端绑定成功响应体；无效的 Go IDL 使生成失败。

已完成：当前正式基线由根 module 和唯一根 tag 管理；发布前的契约、API、文档和
依赖边界均由自动化门禁校验。

## 里程碑 6（已完成）：v2 直接架构重构

### 决策

当前源码以 v2.8.0 候选基线直接维护，不保留开发快照的源码兼容承诺；公开契约由
发布清单、变更日志和 API 快照共同定义。

### 实施状态

- [x] 工作包 0：测试生成到隔离的临时项目中，保持工作树干净。
- [x] 工作包 1 实现：endpoint 缓存所有权移到 `sd/endpointer`，Zap 中间件移到 `integrations/zap`，并且核心 endpoint 包被保护以拒绝非标准导入。
- [x] 工作包 1 竞态门禁：完整的维护竞态套件在本地使用 MinGW-w64 C 编译器通过，并在 Ubuntu/Windows 发布工作流中通过。
- [x] 工作包 2：服务发现契约现在位于 `sd`；负载均衡、重试、endpointer、实例缓存和客户端组合拥有独立的包。泛型 SD 依赖测试拒绝 gRPC 和 Consul 提供者导入。
- [x] 工作包 3：HTTP 错误编码和 HTTP 扩展契约现在位于 `transport/http` 之下；根传输包只保留共享的错误处理器契约。导入测试拒绝 HTTP/gRPC 交叉依赖。
- [x] 工作包 4：`kit` 仅支持 HTTP，可选生命周期组件使用中立契约，gRPC 装配位于 `kit/grpc` package。
- [x] 工作包 5：provider、传输和生成器边界作为 package 保留在单一发布 module 内。依赖闭包由 `TestKitHTTPAssemblyDoesNotResolveOptionalDependencies` 按 package 校验，`TestOnlyOneModuleIsPublishable` 防止重新拆分发布 module。
- [x] 工作包 6：生成器实现包位于 `cmd/microgen/internal` 之下；生成的项目使用新的包拓扑和直接的 `slog`，最小 HTTP 项目不解析任何可选提供者或数据库依赖。
- [x] 工作包 7：`kit` 是唯一的快速开始，底层接线命名为 `manual_composition`，包文档使用最终依赖图。
- [x] 工作包 8：依赖边界、功能、契约、API、vet、模块和干净范围门禁均已完成；当前正式基线由根 module 和唯一根 tag 发布。

### 重构目标

- 保持 `Service -> Endpoint -> Transport` 作为唯一的请求架构。
- 使基础 `endpoint`、HTTP 传输、泛型服务发现、交互运行时、HTTP 安全和 HTTP 装配路径独立于特定提供者的依赖。
- 确保仅 HTTP 的应用不会编译或解析 gRPC、Consul、数据库驱动、生成器、Gobreaker、Zap 或 OpenTelemetry 包。
- 接口和错误放在消费或拥有它们的包中；移除泛型的 `interfaces`、`events` 和 `utils` 包。
- 将可选集成保留在发布 module 内、可独立测试的 package 中。
- 提供一条推荐的首用路径：小型 HTTP 服务用 `kit`，之后需要显式组合时用更底层的 `endpoint` 和 `transport` 包。
- 使完整的验证套件确定性、跨平台，并对 Git 工作树保持干净。

### 目标目录与模块

```text
v2/
  endpoint/                         # endpoint, typed endpoint, generic middleware
  transport/
    http/                           # HTTP contracts, context, query helpers
      client/
      server/
  sd/                               # Event, Instancer, Registrar, Balancer contracts
    endpointer/                     # instance-to-endpoint cache and lifecycle
    balancer/                       # balancing strategies
    retry/                          # protocol-neutral retry execution
    instance/                       # in-memory instance source
  kit/                              # lightweight HTTP assembly and lifecycle
    grpc/                           # optional gRPC lifecycle component package
  interaction/
    mcp/
  security/
    http/
  observability/
    slog/
    otel/                           # optional provider package
  integrations/
    consul/                         # optional provider package
    etcd/                           # optional provider package
    grpc/                           # optional provider package
    zap/                            # optional provider package
  cmd/
    microgen/                       # build-time tool package
      internal/
        dbschema/
        generator/
        ir/
        parser/
```

v2 以一个已发布 Go module 交付。provider、transport、observability 和 generator
边界都是该 module 内的 package 边界；只有 `examples`、`tools` 和
`tools/contractcheck` 保留为仓库内部工作区 module。根 tag 拥有完整 v2 产品，
package 级导入边界与依赖门禁确保最小装配不会承担可选成本。

`gobreaker` 与 `golang.org/x/time/rate` 的适配器模块是明确的非目标。
`endpoint.CircuitBreaker` 与 `endpoint.RateLimitMiddleware` 零依赖且功能完整——包含
基于失败率的触发与慢调用统计——而这类适配器的全部内容就是把一个第三方类型包成一个
`Middleware`，应用自己十几行就能写完。开模块的代价则是一份 `go.mod`、发布检查、
API 快照与双语文档的长期维护。

### 工作包 0：干净且可移植的测试基架

目标：在移动包之前建立可信的基线。

- 将集成项目生成到 `t.TempDir()` 之下，而不是被跟踪的 `tools/testdata/gen_*` 目录。
- 只保留有意的输入夹具和经过评审的 golden 快照被跟踪。
- 在比较文本 API 和契约快照之前规范化 CRLF 和 LF。
- 在禁用 CGO 的情况下运行 SQLite 内省和生成项目测试，使生成器在没有本地 C 工具链的情况下保持可移植。
- 在生成和发布检查周围添加仓库整洁性断言。
- 每条 Go 命令都使用模块声明的工具链运行。

验收：

```bash
go test ./...
go vet ./...
git diff --check
test -z "$(git status --porcelain)"
```

### 工作包 1：核心 endpoint 抽取

目标：使导入 `endpoint` 独立于日志、服务发现和提供者 SDK。

- 只保留 endpoint 函数类型、类型化适配器、中间件组合、超时、背压、错误包装、请求关联和内存指标。
- 将 endpoint 缓存、工厂和失效行为移动到 `sd/endpointer`。
- 将 Zap 日志中间件和 Zap 字段构造移动到 `integrations/zap`。
- 移除 `Logger` 别名并拆分混合用途的源文件。
- 添加一个拒绝核心 endpoint 包非标准导入的导入边界测试。

验收：

```bash
go list -f '{{join .Imports "\n"}}' ./endpoint
go test -race ./endpoint
```

endpoint 导入列表必须只包含标准库包。

### 工作包 2：服务发现重建

目标：使泛型发现和重试独立于 Consul 和 gRPC。

- 在拥有它们的 `sd` 包中定义 `Event`、`Instancer`、`Registrar`、`Balancer` 和 `ErrNoEndpoints`。
- 定义 `Registrar.Register() error` 和 `Deregister() error`；为每个实现添加编译期断言。
- 将 endpoint 调节和资源关闭移动到 `sd/endpointer`。
- 将轮询负载均衡拍平到 `sd/balancer`。
- 在 `sd/retry` 中围绕调用方提供的分类和实现 `Retryable() bool` 的错误重建重试。
- 将指数退避移动到内部包，并将取消纳入其契约。
- 将 gRPC status 分类移动到 gRPC 传输模块。
- 将 Consul 实现移动到 `integrations/consul`，只依赖公开 SD 契约。

验收：

- 泛型 SD 包没有来自 gRPC 或 Consul 的导入。
- 关闭 endpointer 会停止其更新循环并关闭工厂资源。
- 重试取消会中断调用和退避。
- 竞态测试覆盖缓存更新、注册、负载均衡、重试和关闭。

### 工作包 3：传输边界清理

目标：使每个传输包拥有所有协议特定的行为。

- 将 HTTP 状态、头部、公开消息、错误码、编码器和请求元数据保留在 `transport/http` 之内。
- 将 gRPC 元数据、状态转换、拦截器和重试分类保留在可选的 `integrations/grpc` package 内。
- 从根 `transport` 包中移除 HTTP 类型。当没有真正共享的行为剩余时删除根包。
- 在 HTTP 和 gRPC 之间保持等价的 before/after/finalizer 顺序，而不强制相同的函数签名。
- 添加防止 HTTP/gRPC 交叉导入的包导入测试。

验收：

- 仅 HTTP 的测试和示例在 gRPC 模块不可用时能构建。
- gRPC 传输不导入 HTTP 传输包。
- 非成功的客户端响应保持有界，并成为类型化的传输错误。

### 工作包 4：轻量 kit 装配

目标：使 `kit` 成为一个小的 HTTP 装配层，而不是全协议的服务容器。

- 保留 HTTP 监听器生命周期、严格 JSON 注册、健康检查、请求 ID、HTTP 中间件、endpoint 中间件和优雅关闭。
- 从核心 `Service` 中移除直接的 gRPC 服务器字段和选项。
- 为可选服务器引入一个小的生命周期组件契约。
- 在 `kit/grpc` 中实现 gRPC 装配；当应用需要两种协议时可以显式附加。
- 用显式的 `WithEndpointMiddleware` 组合替换拥有依赖的限流和熔断快捷方式。
- 不要隐式创建开发 Zap logger。日志由应用所有，并通过标准或可选适配器安装。

验收：

- 最小 `kit` HTTP 应用不导入 gRPC、Zap、Gobreaker、Consul 或数据库包。
- `Service.Run(ctx)` 仍遵循调用方所有的取消。
- 启动失败保持同步，关闭保持有界。

### 工作包 5：可选 package 边界（历史）

目标：使模块边界与组件边界一致。

- 为提供者 SDK、非标准中间件引擎、gRPC 和生成器创建清晰的 package 边界。
- 添加仓库 `go.work` 用于开发和 CI 编排。
- 依赖是否进入应用闭包由实际导入的 package 决定；根 module 统一管理版本。
- 为根 module 和仓库内部 module 提供 tidy、test、vet 和发布检查。
- 保持发布 module 只使用根版本与根标签。

验收：

```bash
go mod tidy
go list -m all
go test ./...
```

在每个工作区模块中运行等价命令。所有模块清单在 tidy 之后必须保持不变。

### 工作包 6：microgen 收口

目标：只生成新的包拓扑，并将构建期依赖保持在运行时模块之外。

- 将生成器实现移动到 `cmd/microgen/internal` 之下。
- 更新 Go IDL、Protobuf 和数据库流程，以生成新的 SD、transport、observability 和 kit 导入。
- 移除对已删除便捷选项和旧接口路径的生成使用。
- 当包所有权或生成产物所有权变化时更新清单模式。
- 在临时目录中重新生成所有经过评审的契约，并显式评审快照变更。
- 在无法访问可选 package 的情况下构建生成的仅 HTTP 项目。

验收：

- Go IDL、Protobuf 和数据库生成的项目能构建和运行。
- 运行两次生成产生字节相同的自有产物。
- 用户所有的文件在重新生成和 extend 操作之后保留。
- 生成的仅 HTTP `go.mod` 文件不包含未使用的提供者依赖。

### 工作包 7：文档与示例

目标：给出与最终包图一致的一条连贯学习路径。

- 使 `kit` 成为唯一的顶层快速开始。
- 将当前底层快速开始重命名为显式的手动组合示例。
- 在包的最终导入和所有权稳定之后更新包 README。
- 重写 `ARCHITECTURE.md` 以描述结果依赖规则，而不是之前的依赖图。
- 在 `CHANGELOG.md` 中记录每个被移除或重命名的 API，并附前后导入示例。
- 一起更新 `README*`、`PRODUCTION.md`、`MICROGEN.md` 和生成的 README 模板。

验收：

- 每个文档记录的 Go 示例都能编译。
- 文档链接在区分大小写的文件系统上通过。
- 没有文档推荐被移除的包或便捷选项。

### 工作包 8：仓库收口与发布决策

目标：在选择发布路径之前证明重构已完成。

- 从干净的工作树运行完整的测试、竞态、vet、契约、API、生成、编码、链接和模块 tidy 套件。
- 为最小 endpoint、HTTP 传输、SD、kit、交互、安全和可选 package 入口捕获依赖列表。
- 记录最小 HTTP 构建的依赖闭包、二进制大小和模块图，作为后续版本的诊断基线。
- 将最终的导出 API 快照作为一次审慎的契约重置进行评审。
- 按发布清单使用唯一根 v2 tag 发布经过评审的结果。

必需的最终命令：

```bash
make verify
make release-check-clean
go test -race ./endpoint ./kit ./transport/http/... ./sd/... ./interaction/...
git diff --check
git status --porcelain
```

最终状态输出必须为空。

### 依赖门禁

这些规则由测试或仓库工具强制执行，而不仅仅依靠评审：

| 包或模块 | 允许的非标准依赖 |
| --- | --- |
| `endpoint` | 无 |
| `transport/http/...` | 仅核心 endpoint 和协议无关传输契约 |
| 泛型 `sd/...` | 仅核心 endpoint 包 |
| `kit` | 仅核心 endpoint 和 HTTP 传输包 |
| `interaction` | 无 |
| `interaction/mcp` | 仅 interaction |
| `security/http` | 无 |
| `observability/slog` | 仅核心 endpoint 和协议无关传输契约 |
| 可选 package | 仅其声明的提供者 SDK 和核心契约 |

### 完成定义

里程碑 6 仅在以下全部为真时才算完成：

- 没有核心包导入提供者 SDK 或可选协议模块。
- 每个剩余的公开包都有一个清晰的拥有者和包注释。
- `interfaces`、`events` 和 `utils` 兜底包不再存在。
- 仅 HTTP 的使用不会解析或编译 gRPC、Zap、Consul、数据库或生成器依赖。
- 所有生成的项目模式使用新拓扑。
- 完整验证套件在 Linux 和 Windows 上通过，并保持工作树干净。
- 架构、发布、使用和生成器文档保持一致。
- 发布路径和兼容性影响在打标签之前被显式批准并记录。

## 里程碑 7（已完成）：用户工作流覆盖

目标：弥合将用户推到框架请求路径之外、或让生产问题悬而未决的缺口。

- W3C Trace Context 传播：`endpoint.TraceContext`、`ParseTraceparent` 以及 `transport/http` 的 extract/inject RequestFunc；`TracingMiddleware` 加入传入 trace 并生成符合 W3C 的 ID。
- 流式与非 JSON 请求支持：用于 Server-Sent Events 的 `server.NewSSEServer`/`SSEStream`（经 `kit.HandleSSETyped` 注册并套用 endpoint 中间件），带客户端断连取消；用于有界文件上传和下载的 `server.ParseMultipartForm` 和 `server.WriteAttachment`。
- 请求约定：带字段级错误并编码为 400 的 `endpoint.Validatable`/`ValidationMiddleware`；`transport/http` 分页契约（`ParsePage`、`Page`、`PageResult[T]`）。
- 弹性中间件：用于降级应答的 `Fallback` 和按键隔离并发的 `BulkheadMiddleware`；拒绝错误编码为 429。
- 端到端示例：`examples/auth`（应用所有的认证与授权）和 `examples/todosvc`（带优雅数据库关闭的 SQLite CRUD）。
- 生产指引：部署（容器、探针、终止预算）、告警（入门告警集）以及带 `kit.Lifecycle` 接线的后台任务结构。

已完成：每项能力都附带专项测试、导出 API 变更之处经过评审的契约快照，以及所属文档中的文档记录；完整的多模块验证套件通过。

## 里程碑 8（已完成）：有证据支撑的质量

目标：让框架声明的每一条性质都由一个会失败的东西来保证——一个测试、一道门禁，或
一个基准；优先处理那些目前只由文档或注释来声明的地方。

本里程碑来自对 v2.9.0 候选版的一轮完整架构评审。下列每个工作包陈述应当成立的性质，
并各自带一条验收命令。

### 工作包 1：浏览器安全完备性

目标：一个 CSRF 令牌在有限时间内只为一个会话授权，且每个面向浏览器的判定都自己声明
方案与缓存范围。

- CSRF 令牌绑定调用方提供的会话身份与签发时间，超出配置生命周期即拒绝。`CSRFConfig`
  携带会话访问器；无法解析会话的非安全请求被拒绝。
- 铸造令牌的响应声明 `Cache-Control: no-store` 与 `Vary: Cookie`，因此中间设施为一个
  用户存下的令牌只属于这一个用户。
- CORS 与 CSRF 对来源合法性取得一致：不透明的 `null` 来源两者都拒绝；CORS 的每条分支
  ——放行、拒绝、无来源——都声明 `Vary: Origin`。
- HTTPS 的判定来自声明而非推断。`SecurityHeadersConfig` 与 CSRF 来源检查声明是否由受
  信代理终止 TLS，因此 HSTS 与同源比较在负载均衡器之后依然成立。

验收：

```bash
go test ./security/... -run 'CSRF|CORS|Headers|Proxy' -count=1
go test -race ./security/...
```

测试断言：为某会话铸造的令牌对另一会话被拒；超出生命周期的令牌被拒；铸造响应带两个
缓存头；`null` 使 CORS 构造失败；被拒的预检仍然按来源 Vary；代理声明 HTTPS 时发出
HSTS。

### 工作包 2：可测量的性能基线

目标：性能陈述建立在基准之上，因此一次优化可以被证明有效，一次回退可以被看见。

- 基准覆盖每个请求都会经过的路径：零个与五个中间件的 `Chain`、一次 JSON 往返的
  `Server.ServeHTTP`、`balancer.Pick`、8 与 64 并发下的 `feedback.Table`、
  `Metrics.Observe`、`TracingMiddleware`。
- 优化在其基准存在之后才落地，每项在 `CHANGELOG.md` 记录前后数值。
- 关联标识经由一个 context 节点抵达请求。
- 实例快照与反馈测量都以写时复制发布，读取时不加锁不拷贝，因此一次要问遍所有候选的选择
  不必等待记录结果的那条路径。
- 重试在任何尝试次数下都让每次尝试跑在自己的 goroutine 上。正是这个 goroutine 让调用方能
  放弃一个不尊重 context 的实例，而这是超时预算需要的，与后面是否还有第二次尝试无关。
- 为性能而做的改动只在其基准显示收益时保留。被测量否证的改动回滚，并把数字记录在代码所在
  之处，让这个想法不会被再次盲目尝试。`Metrics` 收集器的单把互斥锁是第一条这样的记录。

验收：

```bash
go test -run '^$' -bench . -benchmem ./endpoint ./sd/... ./transport/http/server
go test -race ./endpoint ./sd/...
```

### 工作包 3：探针与关联标识的归属

目标：就绪、存活与请求关联属于任何传输都能挂载的组件，因此 gRPC 服务拥有与 HTTP 服务
相同的运维面。

- 探针引擎——每检查超时、单飞门控、panic 收容与响应结构——独立成包，导出注册表与
  handler。
- `kit` 与 `kit/grpc` 都挂载该注册表；挂在 `Host` 上的 `ReadinessProvider` 无论存在哪些
  传输都能抵达一个探针面。
- 探针路径，以及它们共用应用监听器还是独立管理监听器，都是选项。
- 请求 ID 的 HTTP 半边归 `transport/http`，因此由传输包直接组装的服务获得与 `kit` 相同
  的头名称、校验与生成器。

验收：

```bash
go test ./kit/... ./transport/http/... -count=1
go test ./tools -run TestArchitectureDependencyGates -count=1
```

测试中，仅 gRPC 的装配能回答自己的就绪探针。

### 工作包 4：可观测性装配完备性

目标：一次调用即正确搭好 OpenTelemetry，遥测命名遵循语义约定，trace context 默认穿过
每种传输。

- `observability/otel` 在现有中间件之外，提供 provider、exporter、resource、全局 W3C
  propagator 与 shutdown 的装配。
- instrument 名称与单位遵循 OpenTelemetry 语义约定；HTTP 遥测携带 `http.route` 与
  `http.status_code`，因此响应状态可从指标告警。
- gRPC 服务端与客户端双向传播 `traceparent`；`kit` 与 `kit/grpc` 无需额外接线即提取。
- `interaction` 通过与请求路径相同的 logger 与关联契约上报工具调用。
- `NewTelemetry` 可逐个选择信号，其中间件自带名称而不依赖位置。

验收：

```bash
go test ./observability/... ./integrations/grpc/... ./interaction/... -count=1
go list -deps ./observability/slog | Select-String opentelemetry
```

依赖检查无匹配：只取日志依然零成本。

### 工作包 5：服务发现的可组装性

目标：能编译的发现装配就是能工作的装配，且一套订阅状态机服务所有消费者。

- 订阅、错误宽限与失效状态机只有一份实现，由 selector、endpointer 与 feedback 统计共用，
  `sortInstances` 只有一份。
- `sd/balancer` 覆盖每一种不需要测量的策略，这也是它的全部职责：需要测量的那些由
  `sd/feedback` 装配，于是可选层在构建里和在 API 里同样保持可选。
- 依赖测量的策略与喂养它的统计一同取得，因此 scored、least-request 或 slow-start 均衡器
  无法在缺少其 table、订阅或包装的情况下被构造出来。统计跟随注册而非健康判定，调用方
  不必知道它必须这样做。
- `sd.Registrar` 以选项声明冲突语义——覆盖、仅创建、或比较并交换——每个提供者说明自己
  支持哪些。

验收：

```bash
go test ./sd/... ./integrations/etcd/... ./integrations/consul/... -count=1
go test -race ./sd/...
```

### 工作包 6：HTTP 协议语义

目标：JSON 服务端用协议层面的答案回答协议层面的问题。

- 媒体类型不是 JSON 的请求收到 415，且媒体类型在读取 body 之前检查。
- 解码失败携带写给调用方的消息；空 body 就说空 body。
- `JSONDecodeOptions` 携带解码后钩子，因此希望在解码处做 schema 校验的服务可以在那里做。

验收：

```bash
go test ./transport/http/... -count=1
```

### 工作包 7：生成代码的类型安全

目标：生成代码以与手写框架代码相同的方式失败——返回一个被分类的错误。

- 生成的传输与 SDK 经 `endpoint.Unwrap` 转换 endpoint 响应，因此改变响应类型的中间件
  产生可诊断的错误而不是 panic。
- `microgen -from-db` 在打开连接之前校验自己的必填输入。
- 生成的依赖版本由发布清单派生，因此生成的项目总能解析。

验收：

```bash
go -C ./tools run ./releaseverify -suites test
go test ./cmd/microgen/... -count=1
```

### 完成定义

里程碑 8 仅在以下全部为真时才算完成：每个工作包的验收命令通过；经评审的 API 快照反映
有意的接口变更；`CHANGELOG.md` 记录每项行为变更，其中为性能而做的变更附带实测效果。

已完成：本里程碑审视过的每一条声明，现在都由一个会失败的东西来保证。v2.9.0 交付的性质
包括：绑定会话的 CSRF；被基准覆盖并记录了实测效果的请求路径；与传输无关的探针注册表；
一次调用完成的 OpenTelemetry 装配；一套发现订阅状态机，以及无法脱离喂养它的统计而装配
出来的依赖测量的负载均衡；协议层面的 JSON 答复；把类型不匹配报成可分类错误的生成代码。
性能数字里包含被测量否掉的那一次改动，所以那个想法不会被盲目重试。

## 里程碑 9（已完成）：值得宣布的冻结

目标：兼容性契约在被承诺之前，先由一个会失败的东西来保证。

v2 处于冻结前阶段，`RELEASE.md` 已经列出契约将覆盖的六项内容。把每一项对着 `tools` 里的
门禁套件核过之后：两项只由文档保证，四项只被部分保证。在这个基础上宣布冻结，等于给出一个
工具链兜不住的承诺，而第一次意外破坏将由使用方而不是 CI 发现。

下列每个工作包陈述应当成立的性质，并各自带一条验收命令。

### 工作包 1：破坏性变更被指名，而不只是被察觉

目标：一次导出 API 变更能说清自己是新增还是不兼容。

- `TestPublicAPISurfaceSnapshot` 每个 package 只存一个摘要，因此新增一个导出函数和删除一个
  导出函数失败得一模一样——都只是一串十六进制变了——评审者要手工 diff `go doc` 输出才知道
  发生了哪一种。移除、签名变更、以及被收窄的接口都被报成不兼容，而新增通过。
- 比较的对象是上一个已发布 tag，这样问的问题才是契约要问的问题：这次发布与使用方钉住的那个
  版本兼容吗。
- 刷新受评审接口面这件事仍然是有意为之。`-update-api-snapshot` 记录新的接口面；它不能变成
  不兼容变更被顺手放行的通道。

验收：

```bash
go -C ./tools test . -run 'TestPublicAPISurfaceSnapshot|TestAPICompatibility' -count=1
```

兼容性测试在移除时失败、在新增时通过，二者都由测试自身的用例证明，而不是靠拿真实接口面去试。

### 工作包 2：导出 package 集合被有意钉住

目标：移动或重命名一个导出 package，会让一个专门为此存在的测试失败。

- 现在 package 路径集合只写在 `api_surface.sha256` 的第二列里，也就是说它是被顺带保护的。
  它应当被有意钉住，并且失败信息指名那个被移动的路径。
- `cmd/microgen` 的导出面要么被覆盖，要么被明确写明豁免。它现在被排除在快照之外，对一个内部
  生成器来说这站得住脚，但没有任何地方写下来。

验收：

```bash
go -C ./tools test . -run 'TestExportedPackagePaths' -count=1
```

### 工作包 3：文档里的生成器 flag 存在，存在的 flag 有文档

目标：`microgen` 的命令行与它的文档无法各走各路。

- `main.go` 定义了 23 个 flag；没有任何测试读 `MICROGEN.md`、教程或 README 的 flag 表格。
  一个被重命名的文档化 flag 只会弄坏那些恰好在命令行里用到它的集成测试。文档里出现的每个
  flag 都有定义，定义的每个 flag 都有文档或被明确标为内部使用。
- extend 模式的用法文本在 `newExtendFlagSet` 里手写，而 `TestNewExtendFlagSetUsage` 钉的是
  那段字符串而不是对应关系。用法文本应当从 flag 集合派生，或与之核对。

验收：

```bash
go -C ./tools test . -run 'TestMicrogenFlagsAreDocumented' -count=1
go test ./cmd/microgen/... -count=1
```

### 工作包 4：被承诺的生成文件位置真的被钉住

目标：使用方会去编辑的那套布局，就是契约覆盖的那套布局。

- 生成布局在一处决定——`internal/generator/layout.go`——而检查它的是散落在九个测试文件里约六十次
  `mustExistFile` 调用。覆盖范围取决于哪个 fixture 恰好需要某个文件，于是
  `cmd/generated_runtime.go`、`cmd/generated_services.go`，以及 `config/` 除 `config.yaml`
  和 `custom.go` 之外发出的每个文件，都没有任何测试断言其位置。`layout.go` 声明的布局应当被
  钉成一份受评审的清单。
- 契约快照钉住四个固定路径加两个 glob，而 glob 不是位置约束：某个目录不再产出文件只会让快照
  变小而不是失败，接着 `make update-snapshots` 就把变小后的集合认可了。快照承诺什么，应当以
  路径的形式说明。
- fixture 覆盖了哪些 flag 组合应当写明，因为只在 `-interaction` 或 `-tests` 下产出的路径，
  只有在某个 fixture 传了该 flag 时才真的被钉住。

验收：

```bash
go -C ./tools test . -run 'TestMicrogen.*(Contract|Integration)' -count=1
```

### 工作包 5：每个文档化的配置键与阶段都被跑到

目标：文档画出的优先级链条，就是生成的加载器实际执行的链条。

- `TestMicrogenConfigIntegration` 证明了文件盖过默认值、env 盖过文件、远端盖过文件、env 盖过
  远端。而 `docs/configuration.md` 里写明的 flag 阶段以及其后的 `Config.Validate` 没有被跑到，
  因此 flag 优先级反过来也不会让任何东西失败。每个文档化的阶段都应被覆盖。
- 六个文档化的 `APP_*` 键里只有两个被碰到。每个文档化的键都应被某个测试读到，而加载器不再读
  的键应当让某个测试失败。

验收：

```bash
go -C ./tools test . -run 'TestMicrogenConfigIntegration' -count=1
```

### 工作包 6：稳定的协议行为要自己说出来

目标："被文档声明为稳定"是某个具名行为的性质，而不是发布文档里的一句话。

- 代码库和文档里没有任何东西区分"承诺稳定的协议行为"和"恰好能用的行为"，因此契约的第六项目前
  覆盖的是一个没有名字的集合。稳定行为应当在其实现处被逐一列出，每一条都有一个在它改变时会
  失败的测试。
- 有意不稳定的行为也应标出来，这样"没有承诺"这件事同样被写下来。

验收：

```bash
go -C ./tools test . -run 'TestStableProtocolBehaviour' -count=1
go test ./transport/... ./interaction/... -count=1
```

### 完成定义

里程碑 9 在以下全部为真时完成：每个工作包的验收命令通过；`RELEASE.md` 里六项契约内容各自
指名保证它的门禁。宣布冻结是随后的决定，不属于本里程碑——本里程碑的职责是让那个决定可以被
安全地做出。

已完成：六个契约表面现在各自指名了门禁，`TestCompatibilityContractNamesItsGates` 会在被
指名的门禁不再存在时失败。不兼容的 API 变更被报告为不兼容，而不是一串移动了的摘要；发布的
包路径集合与生成项目布局是经过评审的清单，而不是某个哈希的副作用；生成器的标志与生成程序
的标志都对着列出它们的文档被检查；配置链条一路跑到 flag 阶段以及其后的校验，每个文档化的
`APP_*` 键都被证明能到达它的字段。第六项不再是一个没有名字的集合：四十九条协议行为声明在
它们的实现处，每一条都指名了同包内的测试，两条刻意保持不稳定的行为也以同样的形式说明。

## 里程碑 10（已完成）：值得冻结的协议

目标：v2 承诺的 MCP 行为，就是当前规范定义的行为。

里程碑 9 让协议承诺变得可检查，而第一件被检查出来的事情就是其中一条已经过期。MCP 在
2026-07-28 发布了 2026-07-28 版本：取消 `initialize`/`initialized` 握手与
`Mcp-Session-Id` 头，把协议版本、客户端身份与客户端能力移入每个请求的 `_meta`，新增
`server/discover`，要求 `Mcp-Method` 与 `Mcp-Name` 路由头，并让列表结果带上 `ttlMs`、
`cacheScope` 和确定的顺序而可被缓存。v2 说的是 2025-06-18。旧版本保有十二个月的弃用
窗口，因此两个版本同时提供，而不是新的替换旧的。

### 工作包 1：协议版本决定请求模型

目标：请求由它自己指名的版本来应答，而无状态的那一版不需要请求之外的任何东西。

- `MCP-Protocol-Version` 逐请求选择请求模型。缺少该头时选择 2025-06-18——头成为强制
  要求早于这一版本，所以今天能用的东西不会停止工作。
- 在 2026-07-28 下不铸造、不读取、也不要求任何会话，`initialize` 与
  `notifications/initialized` 被作为"已退役"应答而不是被服务。客户端身份与能力来自
  `params._meta`，并以过去会话状态的方式到达工具。`server/discover` 回答那些想先了解
  能力再决定的客户端。
- 这一版本的传输就是 POST：GET 与 DELETE 是为会话而存在的。

验收：

```bash
go test ./interaction/... -count=1
go -C ./tools test . -run 'TestStableProtocolBehaviour' -count=1
```

### 工作包 2：路由头必须描述请求体

目标：网关用来路由、计量和鉴权的东西，就是请求实际做的事情。

- `Mcp-Method` 重复 JSON-RPC 方法，`Mcp-Name` 重复它寻址的目标。头与请求体不一致的
  请求被拒绝——否则按头判定的按工具限流或策略，会被应用到与实际执行不同的调用上。
- 不寻址任何目标的方法不得携带名字，包括本服务端不认识的方法。

### 工作包 3：列表结果说明自己可以被缓存多久

目标：客户端可以缓存目录，而不是每次重连都重新拉取。

- `tools/list`、`prompts/list`、`resources/list`、`resources/templates/list` 和
  `resources/read` 带上 `ttlMs` 与 `cacheScope`，可在 handler 上配置。scope 默认
  `private`，因为目录可能是按授权过滤的。

### 工作包 4：服务端发起的输入以多轮请求（MRTR）传递

目标：需要中途确认的工具，在没有常开流的情况下也能工作。

- 工具返回 `interaction.InputRequired`，携带它需要被回答的问题以及希望被回传的状态；
  传输层以 `resultType: "input_required"` 加 `inputRequests` 与不透明的
  `requestState` 应答，调用方带 `inputResponses` 重发同一次调用。工具带着答案从头再跑
  一遍，因此它读起来像一道门禁而不是被挂起的协程：两轮之间服务端不持有任何东西。
- 未完成的调用是它自己的一种结果，而不是失败：不发 error 事件，日志说的是"需要输入"
  而不是"调用失败"。
- 调用方没有声明能回答的问题会得到 `-32021` 并指名所需能力——照问不误会把没有相应
  代码的客户端挂住。
- 状态是从调用方回来的，因此由 `RequestStateKey` 对它做认证：配置了密钥时，被改动或被
  重放的状态在工具看到它之前就被拒绝，`RequestStateTTL` 限制窗口。没有密钥时按原样接受，
  文档把这一点直说，而不是让人以为服务端记着什么。
- sampling 在这一版本被弃用，保留在 2025-06-18：`SendSamplingRequest` 继续通过会话流工作。

验收：

```bash
go test ./interaction/... -count=1
```

### 工作包 5：授权与扩展是被指名的表面

目标：被加固的授权模型和扩展框架，与协议其他部分被同等覆盖。

- 这一版本通过六个 SEP 加固授权；v2 实现的每条规则都声明在其实现处，与其他协议承诺
  一致。
- 哪些调用方可以到达哪些方法，是部署的属性，所以 v2 只提供接缝、不提供策略：
  `mcp.MethodAuthorizer` 在两个版本上都被问及每一个请求，带上方法、目标、请求头，以及
  该请求被服务时的 context；没有配置授权器的 handler 服务所有已实现的方法。框架决定的
  是这个问题的形状和拒绝的形状，不是答案。
- 策略自己从 context 读取主体，所以框架对身份如何表示不持意见，也不从请求 body 里取
  主体：请求对自己声称的主体仍然只是一项主张。这是最值得一道门禁的性质。
- 扩展在规范中已经版本化。v2 接受什么、忽略什么，现在写出来了：部署用 `mcp.Extension`
  与 `RegisterExtension` 声明自己的扩展；没有注册任何东西之前不声明任何扩展；未注册扩展
  的方法保持 -32601，于是客户端回落到核心协议；不认识的 `_meta` 键被带给实现，而不是被
  拒绝。框架自己不实现任何扩展，包括官方的那些：`io.modelcontextprotocol/` 命名空间对
  应用是被拒绝的，正是为了让这句话保持为真。

验收：

```bash
go test ./interaction/... -count=1
go -C ./tools test -run TestStableProtocolBehaviour . -count=1
```

### 完成定义

里程碑 10 在以下全部为真时完成：两个版本的客户端都由"不再被服务就会失败"的测试覆盖；
每条新行为都声明在守护它的代码旁边；`RELEASE.md` 仍为每个契约表面指名门禁。

## 里程碑 11（已完成）：有意为之的停止

目标：停止是一个有声明顺序、也有声明终点的过程，而不是"被取消的 context"与"还在运行
的东西"之间的一场竞速。

今天的现状是：`Host` 一被取消，就立即按反向挂载顺序、在同一个共享的截止时间下拆掉所有
组件。这留下三个没有答案的问题，而每一个都会以"一次失败的请求"的形式被用户看到：

- 拆解过程中 readiness 仍然报告就绪，于是负载均衡器和服务注册中心继续把流量送进一个
  正在关闭的进程；
- 长连接响应——SSE、流式 handler——从来没有被告知进程要停止，于是
  `http.Server.Shutdown` 等满整个预算，然后在连接仍然打开的情况下返回一个超时错误；
- 一个慢组件可以吃掉整个预算，排在它后面的组件拿到的是一个已经过期的 context。

### 工作包 1：Draining 是一种状态，不是一个瞬间

目标：进程在离开之前先宣布自己要离开。

- `Host.Run` 在拆解之前进入 drain 阶段：readiness 开始失败，只有在一个可配置的 drain
  延迟之后组件才停止。延迟为零就保持今天的行为，所以没有主动要求它的装配不会有任何变化。
- Draining 是可观测的，不靠猜：readiness 报告它为什么失败；想要停止接受自己那部分工作的
  组件实现这个接缝并被通知到。框架不替别人的组件决定 draining 意味着什么。
- Draining 期间 liveness 保持为真。一个正在收尾在途工作的进程，不是一个该被杀掉的进程。

验收：

```bash
go test ./kit/... ./health/... -count=1
```

### 工作包 2：会结束的宽限期

目标：shutdown 会结束，并且说清它不得不打断了什么。

- 长连接请求通过它自己的 context 得知进程正在停止，于是流可以自己收尾，而不是被切断。
- 优雅关闭的预算用尽时，服务器关闭剩下的连接，而不是在它们仍然打开的情况下返回——并且
  报告被打断了什么。
- 每个组件的关闭预算都被写明。慢组件不得悄悄消耗排在它后面的组件的预算。

验收：

```bash
go test ./kit/... -count=1
go test -race ./kit/... -count=1
```

### 工作包 3：先摘流量

目标：实例先离开服务发现，再停止应答，而不是反过来。

- 服务发现注销、readiness 失败、关闭监听三者之间的顺序被声明并被测试，而不是交给挂载
  顺序去决定。
- 注销做不到的事情也要写出来：对端已经缓存的注册项会比注销活得更久，这正是 drain 延迟
  存在的理由。

验收：

```bash
go test ./kit/... -count=1
```

### 工作包 4：生成的服务也要正确停止

目标：生成的入口用同一套顺序，它的旋钮是被文档化的配置项，而不是模板里的常量。

- drain 延迟与 shutdown 超时是经过校验的配置键，有文档化的优先级，与其他生成配置键一致。
- `PRODUCTION.md` 写明运维契约：滚动发布应该怎么设，以及活得比预算更久的流会怎样。

验收：

```bash
go -C ./tools test -run TestGeneratedConfigKeysAreDocumented . -count=1
go -C ./tools test -run TestMicrogenConfigIntegration . -count=1
```

### 完成定义

里程碑 11 在以下全部为真时完成：关闭顺序声明在守护它的代码旁边；任何一步顺序被改动都会
有测试失败；没有任何关闭路径能在它自己拥有的连接仍然打开时返回。

## 里程碑 12（已完成）：能拿来做判断的数字

目标：运维可以 scrape 任意一个 v2 服务，拿到与其他所有 v2 服务同名、同义的数字——而框架不
替应用选择指标客户端库。

v2 已经能推送遥测：`observability/otel` 一次调用完成装配，instrument 名字遵循语义约定。它做
不到的是回应一次 scrape，而这恰恰是大多数部署真正在用的模型。于是每个应用自己接 exporter、
自己命名序列、自己挑标签，同一个仓库里的两个服务最终对"请求耗时"是什么都各说各话。数字本身
已经存在——`endpoint.Metrics` 与 `endpoint.Recorder` 在采集——所以这个里程碑关于暴露与命名，
不关于测量。

### 工作包 1：有 scrape 表面，核心里没有客户端库

目标：任何装配都能挂载一个指标端点，而核心依赖路径不因此多出一个指标客户端。

- exposition 由 v2 自己在 `observability/metrics` 中渲染，数据来自 `endpoint.Metrics`
  已经持有的数字。整个模块里不出现任何指标客户端，所以也没有什么需要门禁"挡在核心之外"；
  门禁改为把这个包钉在 `endpoint` 与标准库上。需要直方图、exemplar 或自己的 registry 的
  应用，用自己的库去实现 `endpoint.Recorder`——那个接缝比这个更早存在。
- 挂载是应用的决定。`kit` 不导入这个包，依赖门禁保证它继续不导入；于是路由、它所在的监听、
  以及它是否存在，都留在部署手里。纯 gRPC 装配把同一个 `http.Handler` 挂到自己的 admin
  mux 上。

### 工作包 2：一套数字，两条出口

目标：拉取与推送不能互相矛盾。

- scrape 表面报告的东西，与 OpenTelemetry 适配器报告的东西来自同一份
  `endpoint.Recorder` 数据，于是基于其一做的看板和基于另一个做的告警不会对不上。
- 两种模型确实存在差异的地方——重启后计数器归零、直方图桶的划分——要写出来，而不是抹平。

验收：

```bash
go test ./observability/... -count=1
```

### 工作包 3：基数有界且被声明

目标：指标端点不能把采集它的系统拖垮。

- 标签来自匹配到的路由模板，绝不来自原始路径；一个序列可能取到的标签值集合，由服务端能够
  控制的东西限定。
- 这个界限声明在代码旁边，并由"一旦无界值进入标签就失败"的测试守护。

验收：

```bash
go test ./kit/ -run TestSeriesCountIsBounded -count=1
```

### 工作包 4：由配置打开，而不是给人惊喜

目标：生成的服务暴露指标，是因为有人要求它暴露。

- 一个有文档、有校验的配置键打开端点并设置路径；默认关闭，因为把指标端点放在公网监听上是
  一个由部署来做的信息暴露决策。
- `PRODUCTION.md` 写明 scrape 什么、间隔多少、各序列是什么含义。
- 接线过程中发现的约束，它决定了这次改动的形状：生成的路由注册器接受 `*http.ServeMux` 并
  自己注册各自的 pattern，而 `httpserver.RecordingMiddleware` 从 `http.Request.Pattern`
  读取路由——只有被 mux 分派到的那个 handler 才能看到它。因此 recording 必须安装在"知道
  pattern 的地方"，也就是注册器内部，于是 `registerRoutes` 与每个生成的注册器都要接受一个
  registrar 接口，而不是具体的 mux。"先挂上一个没人喂的 exposition"不是可接受的中间态：它
  报告全零，读起来就是一个没有流量的服务，而 `kit` 已经拒绝了这种装配。
- 迁移需要的接缝现在有了：`httpserver.RouteRegistrar` 与 `httpserver.DecorateRoutes`。生成的
  签名已经改为接受它——`RegisterHTTPRoutes`、`httpRegistrars`、`registerRoutes`；而只写一次、
  永不覆盖的 `cmd/main.go` 与 `cmd/custom_routes.go` 仍能编译，因为 `*http.ServeMux` 满足该
  接口。这次不需要 manifest 迁移。

验收：

```bash
go test ./cmd/microgen/... -count=1
go -C ./tools test -run TestMicrogen . -count=1
go -C ./tools test -run TestGeneratedConfigKeysAreDocumented . -count=1
```

### 完成定义

里程碑 12 在以下全部为真时完成：对生成服务的一次 scrape 返回的序列，其名字与标签都声明在发出
它们的代码旁边；依赖门禁仍然把指标客户端挡在核心之外；并且当拉取与推送对同一次请求报出不同
数字时会有测试失败。

## 里程碑 13（已完成）：能直接放到公网上的监听

目标：v2 服务可以自己终止 TLS，并且它在协议协商与被 hijack 的连接上做了什么、拒绝做什么，
全部写下来。

今天 `ServeTLS`、`tls.Config`、`ListenAndServeTLS`、`h2c` 在库和生成代码里一次都没出现。
因此每个部署都在别处终止 TLS——sidecar、ingress、负载均衡器——这是一个合理的默认，同时是一个
未声明的默认。"未声明"才是关键：它正是一个服务带着假设而不是决定进入生产的方式，也是第一句
"为什么 HTTP/2 不工作"由客户端而不是由读者发现的方式。

### 工作包 1：TLS 是配置，而"不做 TLS"是一个表态

目标：进程内终止成为可能，而"不终止"是一个被文档化的立场，不是一个缺口。

- 证书与私钥，或者由部署自己构建的 `*tls.Config`，是服务组件上的选项。除了一个写明的最低
  版本之外，其余都属于部署：密码套件、客户端认证、轮换都是策略，而策略应该待在合规要求所在的
  地方。
- 无法加载的证书让构造同步失败，错误里带着路径——而不是在第一次握手时失败，那种失败要靠
  客户端来替你报告。之所以是构造而不是 `Start`：证书对在任何东西开始监听之前就能读取，而
  最早的诚实失败就是最好的失败。
- 明文仍然是默认，文档写明为什么，以及它对"服务所处网络"做了什么假设。

验收：

```bash
go test ./kit/ -run 'TestTLS|TestNoTLS' -count=1
```

### 工作包 2：协议协商是被写明的，不是被假设的

目标：没人需要抓包才知道自己拿到的是哪个协议。

- 在 TLS 之上，HTTP/2 通过 ALPN 到来，这会改变流式行为。它对 SSE 与流式 MCP 传输意味着
  什么，写在这些特性被文档化的地方。
- 明文 HTTP/2 不启用，理由写出来：它需要先验知识或一次升级交换，而在想要它的那些部署里，前面
  的代理早就拥有了这个决定权。

验收：

```bash
go test ./kit/ -run 'TestSSEStillStreamsOverHTTP2|TestPlaintextListenerSpeaksHTTP11' -count=1
```

已交付为 `kit.streaming-survives-http2` 与 `kit.no-cleartext-http2`。散文在这两个测试之外补上的
那件事：在 h2 上根本不存在 101 Switching Protocols，所以基于 hijack 的升级只在明文路径上有效
——这正是 h2c 的决定与 hijack 的决定其实是同一个决定的原因。

### 工作包 3：被 hijack 的连接不会被 drain

目标：关闭序列对"它结束不了什么"说实话。

- `http.Server.Shutdown` 不等待被 hijack 的连接，关闭监听也不会关掉它。因此 WebSocket 或
  任何被升级的连接都在关闭序列所承诺的宽限期之外，而这必须声明在做出承诺的代码旁边——里程碑
  11 说过"没有任何关闭路径会在它自己拥有的连接仍然打开时返回"，而被 hijack 的连接恰恰是它
  不再拥有的那一种。
- 接缝是执行升级的那个 handler：它像其他 handler 一样收到 stopping 信号，结束被升级的连接
  是它的职责。测试钉住这个信号确实到达了它。

验收：

```bash
go test ./kit/ -run 'TestHijackedConnectionOutlivesShutdown|TestUpgradedHandlerIsToldTheProcessIsStopping' -count=1
```

已交付为 `kit.hijacked-connections-are-not-drained`。测试也钉住了不体面的那一半：在 `Shutdown`
返回 nil 之后，被 hijack 的连接仍然在传字节。读者需要的是这个事实，而不是让框架好看的那个。

### 工作包 4：生成的服务与运维契约

目标：生成的服务可以按配置提供 TLS，并且 `PRODUCTION.md` 写明什么时候该这么做。

- 证书与私钥是有文档、有校验的配置键，默认关闭。
- `PRODUCTION.md` 写明何时进程内终止是对的、何时代理是对的，以及各自对 readiness、drain 与
  被升级连接意味着什么。

验收：

```bash
go -C ./tools test . -run 'TestMicrogenConfigIntegration|TestGeneratedConfigKeysAreDocumented' -count=1
```

已交付为 `server.tls_cert_file` / `server.tls_key_file`（`APP_TLS_CERT_FILE`、
`APP_TLS_KEY_FILE`）。两个值得保留的决定：只配一半的证书对会校验失败，而不是悄悄退回明文；启动
横幅报告它真正在提供的 scheme——对一个 TLS 监听打印 `http://` 的横幅，是对所有人第一个问题的
错误回答。`PRODUCTION.md` 补上了路线图没有列出的运维后果：证书文件只读一次，所以除非部署自己
提供 `GetCertificate`，轮换就意味着重启；而一个仍然打在 TLS 端口上的 `http` 健康探针，读起来
就是一个不健康的实例。

### 完成定义

里程碑 13 在以下全部为真时完成：服务可以按配置提供 TLS；坏证书在启动时失败并带上路径；协商与
hijack 的限制声明在具有这些限制的代码旁边；并且当 stopping 信号不再到达被升级的 handler 时会
有测试失败。

## 里程碑 14（已完成）：两个传输，一套契约

目标：v2 在运维层面对一个 HTTP 监听所承诺的一切，gRPC 监听要用同样的方式回答，否则就把差异
声明出来。

前十三个里程碑都花在让 HTTP 表面变诚实上。gRPC 表面跟上了其中一部分，其余没跟上，而缺口不在
RPC 层——`integrations/grpc` 已经有分类错误、元数据钩子和 traceparent 传播——而在它周围的运维
契约里：

- `kit/grpc.Component` 没有实现 `kit.Draining`，所以 `Host.Drain` 会跳过它。里程碑 11 建立的
  那个"公告"停在了 HTTP 边界上。
- 在 kit HTTP 组件之外 `kit.Stopping` 返回 nil，所以一个长生命周期的 gRPC 流没有任何带内方式
  得知进程即将离开。里程碑 11 对"流没法靠客气地询问来 drain"给出的答案，在这里根本不存在。
- 没有 `httpserver.Recorder` 的 gRPC 对应物，也没有 `metrics.GRPCRecorder`，所以里程碑 12 的
  scrape 对 RPC 流量什么都不报。一个纯 gRPC 服务暴露出的是一个几乎什么都不说的端点。
- 生成代码比库还糟：`grpc.NewServer()` 不带任何选项，没有 TLS 凭证，没有 health server，没有
  拦截器，而且 `GracefulStop` 和 HTTP 共用同一个截止时间。里程碑 13 的证书键只到达 HTTP 监听，
  所以生成的 gRPC 端口永远是明文。

这是框架"先发一个传输"必然积累出来的那种不对称，也是读者会在生产上发现的那一种。这里的对等
不等于"同样的代码"——gRPC 没有 hijack、没有 ALPN 问题、没有路由 pattern——它等于同样的问题都
有答案。

### 工作包 1：公告到达两个传输

目标：drain 一个 Host 会告诉每一个 server，而不只是 HTTP 那个。

- `kit/grpc.Component` 实现 `kit.Draining`：在公告时刻，它以 gRPC 允许的方式停止接受新工作，
  并且它的 readiness 在 `Shutdown` 关掉任何东西之前就开始失败——这正是里程碑 11 声明的顺序。
- stopping 信号变成与传输无关。一个 gRPC handler，尤其是流式的那种，通过与 HTTP handler 相同
  的调用，从自己的 context 得知进程即将离开。在机制无法完全相同的地方，行为是相同的：在一个
  没有任何组件拥有的 context 上，`Stopping` 仍然永远阻塞而不是立刻触发，所以只写一次的
  `select` 在两边都是对的。
- gRPC 无法承诺的东西像 hijack 限制一样声明在代码旁边：`GracefulStop` 会等待在途 RPC，但一个
  永不返回的流会一直占住进程直到预算耗尽，然后它会被从底下停掉。

验收：

```bash
go test ./kit/grpc/ -run 'TestDrain|TestShutdown' -count=1
```

已交付为 `grpc.drains` 与 `grpc.shutdown-ends`，并把 `kit.WithStopping` 导出为任何传输都能用来
携带该信号的接缝。两个值得保留的决定：drain 不会开始拒绝调用——在 drain 延迟期间返回
`UNAVAILABLE` 只会让客户端重试到另一个路由层还没停止选择的实例上——以及拦截器安装在调用方自己
的选项之前，这样 handler 不会因为别人多加了一个选项就拿不到信号。

这个工作包自己关于 `GracefulStop` 的措辞后来被证明是错的，而且是 CI 发现的：那个调用在等待
handler 时一直持有 server 的互斥锁，而 `Stop` 需要同一把锁，于是这一对会死锁而不是超时——建立
在它之上的"有界停止"会把进程挂住。`Shutdown` 通过自己的拦截器计数调用、关闭监听、等待，然后
关闭传输，并带上计数报告 `kit.ErrShutdownIncomplete`。生成的入口点原来有同样的模式，现在改为
触发硬停止而不等待它。

### 工作包 2：一次 scrape 能说出关于 RPC 的事

目标：里程碑 12 的那些数字对 gRPC 也存在，用同样的名字，带同样的基数承诺。

- 一份 gRPC server 的观测契约，对应 `httpserver.Recorder`：方法、结果、耗时——在完整方法名仍
  在作用域内的地方上报。
- `metrics.GRPCRecorder` 把它桥接到 `endpoint.Metrics`，与 `metrics.HTTPRecorder` 并排放置，
  这样依赖门禁保持原样：RPC 传输不认识指标，核心两个都不认识。
- operation 标签是完整方法名，它由服务定义所限定——与"把路由 pattern 作为 HTTP 标签"是同一套
  推理。无法识别的方法不被记录，而不是记录成一条空序列。

验收：

```bash
go test ./integrations/grpc/server/ ./observability/metrics/grpc/ -count=1
```

已交付为 `grpc.recording-method-label` 与 `metrics.grpc-bridge-error-class`。两个工作包没有预料
到的决定：流带着 `Stream: true`，并且只在结束时被记录，因为"生命周期"和"延迟"不该共用一个均值；
以及这个桥是一个有自己依赖门禁的独立包，因为把它放进 `observability/metrics` 会让每一个只想要
scrape 端点的纯 HTTP 服务都依赖上 gRPC 库。

### 工作包 3：生成的 gRPC 监听是一个真正的监听

目标：生成服务的第二个端口，在配置、安全与可观测性上与第一个端口一样。

- TLS 凭证来自里程碑 13 加的那对证书键，所以一对证书同时保护两个监听，而不匹配仍然让启动失败
  并带上路径。
- 注册 health 服务，反映与 HTTP 探针相同的 readiness 状态，这样纯 gRPC 部署也有东西可以让探针
  去打。
- 安装 traceparent 拦截器与指标 recorder，并且 drain 延迟施加在 `GracefulStop` 之前，而不是
  只施加在 readiness 之后。
- 每个 server 拿到自己那一份关闭预算，而不是抢同一个截止时间——`Host.shutdownLifecycles` 已经
  在遵守的规则。

验收：

```bash
go -C ./tools test . -run 'TestMicrogen' -count=1
```

已交付在 `main.tmpl` 中。health 服务用的是 grpc-go 自己的 `health.NewServer` 而不是手写的：
`Shutdown()` 本来就意味着"对所有东西报告 NOT_SERVING"，这恰好就是 drain 公告，而生成文件不是
重新实现一个协议服务的地方。因此 readiness 在两个传输上同时开始失败，而不是只在 `/readyz` 上。

### 工作包 4：差异是被声明的，不是被发现的

目标：读者不必读两份实现就能看出每个传输回答了哪些运维表面。

- 文档里的一张表：readiness、drain 公告、stopping 信号、关闭预算、TLS、指标、追踪——在某个传输
  无法回答的地方给出诚实的条目，并写明原因。
- 一道门禁：当一个生命周期契约只加到其中一个组件上时它失败，这样下一次不对称是一次测试失败，
  而不是一次发现。

验收：

```bash
go test ./kit/ -run TestEveryKitContractIsClassifiedForBothTransports -count=1
```

已交付为 `kit/transport_parity_test.go` 以及 `docs/lifecycle*.md` 里的一张表。这道门禁有两半，
因为任何一半单独都撑不住：编译期断言在契约从任一传输上被删掉时失败，而测试在 `kit` 声明了一个
没人分类过的导出接口时失败——并且每一个"否"都记录了理由，因为那里的 `false` 是一个决定，不是
一次遗漏。

### 完成定义

里程碑 14 在以下全部为真时完成：drain 一个 Host 会向两个 server 发出公告；一个 gRPC 流可以在
stopping 信号上自己结束；对纯 gRPC 服务的一次 scrape 报出按方法划分的序列；生成的服务在它的
gRPC 端口上提供 TLS 与 health；并且当一个传输获得了另一个所没有的生命周期契约时会有测试失败。

## 里程碑 15（已完成）：不重启的轮换

目标：交到一个运行中服务手里的东西——首先是证书——可以在它提供服务的同时被替换，而不能被替换的
东西要被点名。

里程碑 13 交付了进程内 TLS，然后自己把缺口写了下来：`PRODUCTION.md` 说证书文件只读一次，所以
轮换意味着重启。那句话是诚实的，而那个行为是差的。证书按计划过期；续签它的平台——一个
cert-manager secret、一个 Vault agent、运维的一个 cron——会在运行中的进程底下替换文件，并期待
进程注意到。"重启才能生效"把一次例行续签变成一次部署，把一次漏掉的续签变成一次故障。

框架的那一份是接缝以及围绕它的顺序保证。什么时候再看一次是部署的决定：这个包不监视文件系统、
不轮询定时器、不安装信号处理器，因为这些每一个都是某些部署将不得不绕开的策略。

### 工作包 1：进程能替换的证书

目标：轮换不需要重启，而一次坏的轮换不是一次故障。

- 每次握手都会去问证书来源，所以关于证书的任何东西都不会在监听启动时被捕获。它从哪里来是部署
  的事：一个文件、一个密钥管理服务、一个 ACME 客户端、一张按名字索引的 SNI 表。
- 一个文件支撑、按需重新加载的来源，因为挂载进来的 secret 需要的就是这个；而当证书对不可读时
  它仍然让启动失败并带上路径——这是里程碑 13 做出的承诺。
- 失败的重新加载保留已经在提供的那份证书并返回错误。一个写了一半的 secret 应该只值一行日志，
  而不是整个监听。

验收：

```bash
go test ./kit/ -run 'TestCertificate|TestWithTLSCertificateSource' -count=1
```

已交付为 `kit.CertificateSource`、`kit.WithTLSCertificateSource`、`kit.CertificateFiles` 与
`kit.CertificateSourceFunc`，声明了 `kit.tls-certificate-per-handshake` 与
`kit.tls-reload-keeps-serving`。一个值得记录的决定：`CertificateFiles.Certificate` 从不触碰
文件系统，所以握手路径不会被磁盘拖慢或失败——测试断言替换文件在 `Reload` 被调用之前不改变任何
东西。

### 工作包 2：生成的服务在一个它写明的信号上轮换

目标：生成的服务不经过一次部署就能拿到续签后的证书。

- 生成的入口点在被要求时重新加载证书，用的是一个被写明而不是被猜到的信号，并记录路径与结果。
  失败的重新加载被记录下来，而服务继续提供。
- `PRODUCTION.md` 不再说轮换需要重启，而是说它到底需要什么，包括由代理终止的部署改做什么。

验收：

```bash
go test ./cmd/microgen/... -count=1
```

已交付在 `main.tmpl` 中：生成的入口点通过自己的一个按握手取证的来源提供证书，并在 `SIGHUP` 上
重新读取。两个值得记录的决定。信号选 `SIGHUP`，因为它是运维本来就会伸手去用的东西，也是续签
任务能发出的东西——定时器会让进程去猜，而文件系统监视会给一个由用户拥有的文件加上一个依赖和一
条策略。另外生成的代码自带一个很小的 `certificateFiles` 而不是导入 `kit.CertificateFiles`：
生成的 `main` 在别处并不依赖 `kit`，为了二十行把那个包拉进来，会把它的依赖闭包拖进每一个生成
的二进制里。

### 工作包 3：不能被轮换的东西被点名

目标：没人靠试一遍来发现限制。

- 监听地址、协议选项与密码策略在监听启动时就固定了。读者被告知他们配置的哪些东西只读一次、
  哪些会被再读一次，而不是从一张选项表里去推断。

已交付为 `docs/configuration*.md` 里的一节：什么会在你自己选择的触发时机上被再读一次（证书）、
什么在构造时只读一次（`tls.Config` 的其余部分）、什么在 `Start` 时只读一次（地址、超时、路由、
探针路径），以及什么每个进程只读一次（配置文件与环境变量）。这个工作包本质上就是散文——没有
哪个行为需要新的门禁，而它已经被门禁覆盖了——所以诚实的交付物就是这张清单，并且结束在它该结束
的地方：改动其他任何东西都意味着一个新的监听，也就意味着一次滚动重启，而 drain 序列存在的意义
正是让那件事平淡无事。

### 完成定义

里程碑 15 在以下全部为真时完成：服务可以在不重启的情况下被交给一份续签后的证书；一次失败的
续签让监听继续提供上一份；生成的服务在一个被写明的信号上做同样的事；并且文档列出了哪些东西在
启动时就固定了。

## 里程碑 16（已完成）：客户端能跟住的健康检查

目标：纠正一个"对两个工具为真、对库为假"的声明，并让它所免除的那件事真的能用。

里程碑 14 的对等表说 `Watch` 没有实现，理由是"在 gRPC health 上做编排的工具调用 `Check`"。这对
`grpc_health_probe` 和 Kubernetes 原生的 gRPC 探针为真。它对最重要的那个消费者为假：grpc-go 自己
的客户端健康检查——由服务配置里的 `healthCheckConfig` 打开的那个——调用的是 `Watch`
（`health/client.go`，`healthCheckMethod`）。当它收到 `UNIMPLEMENTED` 时，会把连接标记为 `Ready`
并停止再问，于是里程碑 11 建立的 drain 公告从未到达那些正在为它守候的客户端。一个带着这种形状
的洞的声明比没有声明更糟：它读起来像一个决定。

### 工作包 1：Watch 流式给出 Check 所回答的东西

- `Watch` 立即发送当前的 serving 状态，然后每变化一次发一条消息，数据来自 `Check` 评估的同一份
  探针注册表。带名字的服务仍然得到 `NotFound`，因为该注册表描述的是进程。
- readiness 检查每个间隔评估一次，由所有 watcher 共享。按 watcher、按消息各评估一次，会让一群
  客户端把一次数据库 ping 变成负载，而那正是健康检查变成故障的方式。
- 最后一个 watcher 离开时轮询停止，所以没人在看的服务不花任何代价。

验收：

```bash
go test ./kit/grpc/ -run TestHealthWatch -count=1
```

已交付为 `grpc.health-watch`，并用 `kit/grpc.HealthWatchInterval` 命名这条流的分辨率——一秒；它
是常量而不是选项的理由是：组件自己注册 health 服务，所以没有什么有用的东西可以交给一个想要不同
值的调用方。需要自己的 health 服务的部署会构建自己的 `grpc.Server`；那个限制现在成了有意思的
那一个，而它写在这里，不靠被发现。

### 完成定义

里程碑 16 在以下全部为真时完成：一个开启了健康检查的 gRPC 客户端能得知某个实例已经开始 drain；
对等表这么写；并且没人再被告知 `Watch` 未实现。

## 里程碑 17（已完成）：不错的默认值

目标：凡是这个框架不问自答交给服务的东西，交出去的那件东西不该是一项负债。凡是它拒绝替人决定
的地方，它应该在读者会去看的位置把这件事说出来。

这个里程碑来自一次全局审计，而不是一个功能想法。十六个里程碑花在钉住行为上，而审计的结论是：
剩下的弱点不是缺功能——是那少数几个地方，默认值靠从标准库继承而来，或者一个陷阱可以通过受支持
的 API 走到。这些比再来一个能力更值钱。

审计确认为"有意为之且已写明"、因此不算工作的部分：后台任务（`PRODUCTION.md` 说 runner 是一份
需要你自己写的草图，不是这个框架发布的包，并给出了那四条规则）、限流器实现
（`endpoint/rate_limit.go` 说契约属于框架、令牌桶属于应用——*实现*属于应用是对的；契约说不出一个 key
不是，见工作包 4），以及每一个可选 provider 都留在核心依赖路径之外。

### 工作包 1：出站客户端是我们的，不是标准库的

- 由 `transport/http/client` 构建的客户端不再使用 `http.DefaultClient`。那个变量在进程里对每一个
  库都可达，所以别人的超时设置或 transport 替换可能改变这些调用；而 `http.DefaultTransport`
  每个 host 只允许两个空闲连接，这对一次性工具是对的，对持续调用同一个上游的服务是错的。
- 连接池按服务的量级设置，拨号与握手都有界，并且 `NewTransport` 被导出，这样需要代理或
  `tls.Config` 的部署是从这些默认值出发，而不是从那个共享的出发。
- 不发明 `Timeout`。截止时间属于调用本身；在这里设一个，会把进程里的每一次调用都封顶在框架挑的
  一个数字上，而且从调用点看不见。`PRODUCTION.md` 把两半都写出来。

验收：

```bash
go test ./transport/http/client/ -count=1
```

已交付为 `httpclient.pool-is-ours` 与 `httpclient.no-invented-deadline`。

### 工作包 2：按路由的请求体上限不是一个陷阱

- `kit.WithJSONMaxBodyBytes` 是组件级的，而传输层其实已经支持按路由的上限。今天，需要某一个路由
  接受更大请求体的服务只能通过 `Handle` 注册一个裸 handler，而这会静默跳过 endpoint 中间件以及
  `HandleJSONTyped` 安装的那些 recorder。"设一个请求体大小，副作用是丢掉可观测性"正是这个项目
  在别处不愿意留下的那类陷阱。
- 修法是给出一个按路由表达它、同时保留接线的方式，并在 customization 表里紧挨着中间件作用域加上
  一行文档。
- `kit.HandleJSONEndpointWithBodyLimit` 与 `kit.HandleJSONTypedWithBodyLimit` 和它们的组件级
  对应物走同一条注册路径——endpoint 中间件、recorder、JSON server options、HTTP context——唯一的
  区别就是上限。上限为零或负数会 panic，而不是表示"不限制"：只有组件级设置那一处，才允许没有请求体
  上限。

验收：

```bash
go test ./kit/ -run TestPerRouteBodyLimit -count=1
```

已作为 `kit.per-route-body-limit` 交付。

### 工作包 3：错误信封说清自己是什么

- `transport/http/server.ErrorResponse` 是一个自制的三字段信封，且被声明为稳定。RFC 9457 的
  `application/problem+json` 已经存在并且可互操作；框架要么把它作为一个部署可以安装的 encoder
  提供出来，要么说明为什么不。今天具体丢掉的是字段级校验细节：`endpoint.ValidationError` 携带
  `[]FieldError`，而 encoder 把它压成了一条消息。
- 决定：提供，但不采纳为默认。`ProblemJSONErrorEncoder` 用与信封相同的状态码、脱敏、响应头和
  Retry-After 写出 problem 文档，把 `code` 作为扩展成员保留下来，让按它分支的客户端照样能用，并把
  每一个非法字段列在 `errors` 下。`ProblemFromError` 与 `WriteProblemJSON` 已导出，于是带自定义
  kind 映射或额外成员的部署是组合它们，而不是重写一遍。
- 不发明 `type` URI。它指向的是一份得有人发布、并且要一直可解析的文档，这件事属于部署；传 `nil`
  写 `about:blank`，那是 RFC 9457 为"没有自己类型的问题"规定的取值。

验收：

```bash
go test ./transport/http/server/ -run "TestProblem|TestWriteProblem" -count=1
```

已作为 `http.problem-media-type`、`http.problem-document-shape`、`http.problem-redaction`
与 `http.problem-encoder-parity` 交付。

### 工作包 4：额度能说出它限的是谁

- `endpoint.RateLimiter` 是 `Allow() bool`。令牌桶属于应用是对的，保持不变；错的是这个契约只能表达
  进程级的额度，于是按租户、按 API key 的限流不是"没做"，而是"表达不出来"，一个吵闹的调用方就能把
  所有人都拒掉。一个说不出生产真正需要的那件事的接缝，就是被交出去的一份负担——正是本里程碑的主题。
- `KeyedRateLimiter`——`AllowKey(ctx, key)` / `WaitKey(ctx, key)`——是第二个契约，而不是给第一个
  再加两个参数：不带 key 的限流器没有按 key 的状态可查，不该靠忽略一个参数来假装自己有。key 由
  `RateLimitKeyFunc` 给出，因为"什么算一个调用方"该由部署决定。
- 空 key 在空 key 上限流。识别不出来的调用方共用一个桶，而不是各自拿到一个新桶或绕过限流，因此少一个
  header 不是一条出路；key 函数为 nil 会在装配处 panic，理由相同。
- `KeyedRetryAfterReporter` 补上了提示这一侧对应的缺口：按租户的桶知道自己的补充速率，而不带 key 的
  reporter 说不出来。

验收：

```bash
go test ./endpoint/ -run "Keyed" -count=1
```

已作为 `endpoint.keyed-rate-limit` 与 `endpoint.keyed-rate-limit-shares-one-bucket` 交付。

## 里程碑 18（已完成）：被别人读

目标：这个框架在被"不是它作者的人"读的时候也要站得住。读者去哪里找答案，答案就该在那里；两个名字
暗示两种行为时，它们就该有两种行为。

这个里程碑来自一次换位审阅：第一次上手的人、写业务逻辑的人、运维它的人、扩展它的人，以及给它写测试
的人。两个发现决定了工作顺序。运维视角被服务得最好，写测试的人最差——而且不同于后台任务或限流器实现，
从来没有任何地方声明过"测试支持不在范围内"。另外，文档的缺口不在散文里——散文是 42 篇英文与 42 篇
中文一一对应、两个索引零死链——缺口在 godoc 里：读者最先碰到的那些包，包注释要么没有，要么只有一行。

对开启本里程碑那次审阅的一处更正：第一遍报告说"十四个 package 完全没有包级文档"，实际说的是"没有
`doc.go`、也没有 `README.md`"。其中多数包在主文件里是带包注释的——`security`、`health`、
`sd/selector`、`sd/feedback` 的还相当充实。真实的缺口比那个窄，就是下面几个工作包写的内容。

### 工作包 2：业务代码 import 的那些包，注释要读得像文档

- `apperror` 的包注释只有四行。它是业务代码 import 的第一个包，也是决定"每一次失败如何到达客户端"的那
  个包，所以注释应该回答读者带着来的问题：该挑哪个 kind、空 kind 会发生什么、消息去了哪里。
- `security` 与 `health` 已经带着充实的注释，不需要重写。把它们列为缺口是那次审阅的错，不是它们的错。

验收：`go doc ./apperror` 能回答该挑哪个 kind、空 kind 会发生什么、消息去了哪里、什么时候用
`WrapCause` 而不是 `Wrap`。

### 工作包 3：扩展点的注释要读得像文档

- `sd/endpointer` 与 `sd/instance` 完全没有包注释，`sd/balancer`、`sd/retry`、`sd/client` 各只有一行。
  `sd/README.md` 讲了行为，但要写自定义 balancer、或者想搞清楚实例缓存为什么存在的人，看的是 godoc。
- `sd/selector` 与 `sd/feedback` 已经能自己说清楚，保持原样。

验收：`go doc ./sd/endpointer ./sd/instance ./sd/balancer ./sd/retry ./sd/client` 每一个都说明了这个包
是干什么的、以及和邻居的区别。

### 工作包 1：时间是一个接缝

- `time.Now()` 被生产代码直接调用：`transport/http/server/recorder.go`、`access_log.go`、
  `security/http/csrf.go`、`sd/retry/retry.go`、`sd/selector/selector.go`、
  `endpoint/metrics.go`、`endpoint/retry.go`、`interaction/mcp/session.go`。部署方要测一个退避
  间隔、一次 CSRF 过期或一个慢启动权重，只能真的去睡；这个仓库自己也一样——单是
  `sd/selector/slow_start_test.go` 就有七处时间调用。
- 做法是一个 `Clock` 接缝，nil 表示真实时间：不关心的服务什么都不用改，而测试可以决定现在几点。
- 它放在 `endpoint`，而不是新开一个包，因为 `endpoint` 是所有其他层都可以 import 的、只依赖标准库的
  最底层——`Recorder` 与 `RateLimiter` 也在那里，理由相同——而
  `TestEndpointHasOnlyStandardLibraryImports` 意味着"对 `time` 的一个接口"是它唯一可能的形状。
- `ManualClock` 和契约一起导出，而不是藏进只给测试用的包。没人能拿到的接缝不是接缝：应用可以接受这个
  框架的组件所接受的同一个时钟。
- `security/http` 用结构化声明而不是 import `endpoint`，因为它的依赖门禁不允许来自本 module 的任何
  东西，而那是有意的——它是可以丢进任何 `net/http` 栈的中间件。任何带 `Now` 方法的值都能用，所以
  `ManualClock` 在那里照样可用。
- 写明的排除项：时钟不决定真实工作花了多久。`Observation.Duration` 与被记录的请求耗时仍然是量出来的。
  一个能把它们缩短的时钟会让 recorder 报告一件关于系统的不实之事，因此那些测耗时的位置——
  `transport/http/server/recorder.go`、`access_log.go`、`slog`／`zap`／gRPC 的 recorder、
  `interaction/runtime.go`——是有理由地保留 `time.Since`，不是漏掉了。
- 有理由地推后：`sd/retry` 的退避等待。它的构造函数是位置参数式的（`Retry`、`WithCallback`、
  `WithClassifier`），所以那里要放时钟就需要一个这个包还没有的 options 形状；再加第四个位置参数式构造
  函数比留着这个缺口更糟。它应该和一次更大范围的 `sd/retry` options 改造一起做。

验收：

```bash
go test ./endpoint/ -run "Clock|Manual|Retry|Metrics" -count=1
go test ./security/http/ -run TestCSRFRejectsATokenExpired -count=1
```

已作为 `endpoint.clock-nil-is-wall-clock`、`endpoint.manual-clock-advance-fires-due-timers`、
`endpoint.retry-clock`、`endpoint.metrics-clock` 与 `security.csrf-clock` 交付。

### 工作包 2：业务代码 import 的那些包要有 godoc

- `apperror`、`health`、`security` 既没有 `doc.go` 也没有 `README.md`。`apperror` 是业务代码
  import 的第一个包，而它的 godoc 首页是空的。

### 工作包 3：扩展点要有 godoc

- `sd` 的八个子包——`balancer`、`selector`、`endpointer`、`feedback`、`health`、`instance`、
  `client`、`retry`——都没有包级文档。`sd/README.md` 讲了行为，但要写一个自定义 balancer 的人是在
  godoc 里读契约。这是最大的单块空白，而它正好落在扩展点上。

### 工作包 4：注册函数的名字要说清它注册了什么

- `kit` 导出了十一个注册入口，而它们的差别归结为两个轴：端点中间件链与 recorder 是否运行，以及用哪个
  请求体上限。其中两个名字会骗人。`HandleSSE`（`kit/sse.go:15`）是对 `Handle` 的一行委托，不携带
  名字所暗示的任何 SSE 行为，于是顺着名字从 `HandleSSETyped` 找过来的读者会静默丢掉整条链。`JSON`
  与 `JSONTyped` 返回一个未注册的 handler，和 `HandleJSON*` 只差一个动词。
- 修法是让 API 自己说清这个区别，而不是依赖读者先找到 customization 那张表。
- `HandleSSE` 是弃用而不是删除。`Handle` 本来就是那个逃生舱，也本来就写明了自己跳过什么，给它再起一个
  名字只不过是多出一条"不小心走到那里"的路——但 `TestAPICompatibilityWithLastRelease` 说得对：一个已
  发布的符号就是一个承诺，所以它保留可达，并带上一条写明它到底做什么的 `Deprecated:` 注记。那道门禁
  比 `RELEASE.md` 里的冻结前政策更严，而两者相冲时以更严的为准：为了让自己的改动过关而放松门禁，是错
  的本能。
- `JSON` 与 `JSONTyped` 弃用，改用 `NewJSONHandler` 与 `NewJSONTypedHandler`，与它们包装的
  `httpserver.NewJSONServer` 对齐；`kit` 的包注释按"链是否运行"把全部十一个入口分了组。

验收：

```bash
go test ./kit/... -count=1
go vet ./kit/...
```

### 工作包 5：两个索引不是彼此的超集

- `DOCS_INDEX.md` 与 `docs/index.md` 各自漏掉了对方列出的文档：`tutorial-crud.md` 与
  `examples/README.md` 只在手册索引里，`ROADMAP.md`、`docs/licenses.md`、`tools/README.md` 只在
  仓库索引里，`examples/profilesvc/README.md` 两边都没有。找一篇文档不该取决于你打开了哪个索引。

验收：`v2/tools` 里的文档链接与配对门禁通过，且一个索引里列出的每一篇文档，另一个索引里也列出。

## 里程碑 19（已完成）：promise 没有覆盖到的地方

目标：凡是 `// Stable:` 标记写下一条 promise 的地方，它点名的测试就应该断言那条 promise——不是一条更弱
的，也不是 promise 所覆盖路径的一个子集。

`tools/protocol_behaviour_test.go:78` 已经在检查每个标记点名的测试确实存在。它没法检查那个测试是否断言
了 promise，而本里程碑的发现正来自这里。每一条都是一条没人走过的路：文字是对的，代码不是，而测试和代码
达成了一致。

### 工作包 1：每一个请求就是每一个请求

- `mcp.method-authorization` 说每一个请求都会到达 `MethodAuthorizer`。`authorizeMethod` 只有两个调用
  点，都是 POST。`handleGet` 与 `handleDelete` 一个都没有——于是持有 session ID 的调用方可以挂上服务端
  发起的 SSE 流、或终止一个会话，而策略完全不被问及。覆盖测试跑了十个无状态 POST 方法，两个动词都没跑。
- `MethodOpenStream` 与 `MethodDeleteSession` 用没有任何 MCP 方法占用的命名空间把它们呈现出来，于是策略
  既不必匹配 HTTP 动词，也不必写字符串字面量。拒绝是 403：没有 request id 可以回答，和被拒绝的通知同理。
- 授权跑在会话查找之前，于是被拒绝的调用方无法从 403 与 404 的差别里推出哪些 session ID 存在。

验收：

```bash
go test ./interaction/mcp/ -run "Authoriz|Transport" -count=1
```

已作为 `mcp.transport-operation-authorization` 交付。

### 工作包 2：一条流不能给自己分帧

- `http.sse-framing` 描述的是"每一行输入一行 `data:`"。切分只按 LF，而规范里 CRLF、CR、LF 都结束一行
  ——于是裸 CR 留在了一个 `data:` 行内部，守规范的客户端会在那里结束字段、把其余部分当无名字段读。载荷被
  截断。覆盖测试用的是 `"a\nb"`。
- data、注释文本与事件名都没有转义，因此每一个都能结束自己的帧并注入一个事件。任何流式输出"受调用方影响
  的文本"的 handler 都可达。
- 现在 data 与注释整体保持为 data 与注释。携带终止符的事件名是一个错误：字段值装不下它，所以那个名字是
  调用方的 bug，而剥掉它和原样透传都会分出一个没人要的帧。

验收：

```bash
go test ./transport/http/server/ -run SSE -count=1
```

已作为 `http.sse-line-terminators` 与 `http.sse-no-frame-injection` 交付。

### 工作包 3：上限要跑在代价之前

- `transport/http/server/multipart.go:71` 调用 `ParseMultipartForm`，它会把超过 `maxMemory` 的部分落到
  临时文件，之后才拿 `fh.Size` 去比 `MaxFileBytes`。每个请求实际的磁盘上限是 `MaxBodyBytes` 而不是
  `MaxFileBytes`，于是调用方可以逼出更大的写入，然后照样收到 413。
- 同一个标记承诺"超限的请求体或文件"用 code `request_too_large`，但文件那一支发的是
  `request_too_large.file`。按承诺的字符串去匹配的客户端会漏掉它。
- 决定：让 promise 追上行为，而不是反过来。说出撞到的是哪个上限比两者共用一个 code 更有用，所以标记把两个
  code 都写出来。`request_too_large.file` 保留了前缀，因此按前缀匹配的客户端本来就是通的。
- 关于代价的决定：边流式读边按文件强制上限，需要 `r.MultipartReader()` 以及重新实现 form 组装、落盘与
  清理。那是一大堆新代码，去守一个部署方本来就设了的边界——`MaxBodyBytes` 已经限住了临时文件的代价，只是
  从来没有人把它描述成这个作用。否掉它，改为写明哪个字段限的是工作量，并声明拒绝会清理掉什么。

验收：

```bash
go test ./transport/http/server/ -run Multipart -count=1
```

已作为 `http.multipart-file-refusal-leaves-nothing-behind` 交付，并重述了 `http.multipart-limits`。

### 工作包 4：只能有一个 Content-Type

- `transport/http/server/error.go` 先设 `Content-Type`，然后把 `Headerer` 错误报出的每个 header 都
  `Add` 进去。一个 `Headers()` 里带 `Content-Type` 的错误会产出两个值，而那样的响应没有客户端能解释。
  三个调用点。
- 修法就是顺序：先合并错误要求的东西，再命名编码器真正写出的那个 body 的类型。现在三处由同一个 helper
  支撑，于是它们不会再各自漂移。

验收：

```bash
go test ./transport/http/server/ -run "ContentType|HeaderOrder" -count=1
```

已作为 `http.error-single-content-type` 交付。

### 工作包 5：坏 cursor 是一个错误

- `interaction/mcp/handler.go:566` 把非数字、负数、越界的 cursor 都当成 offset 0。MCP 规范要求
  `-32602`。一个跨目录变更持久化了 cursor 的客户端，会拿到第一页并以为那是它的那一页。
- `mcp.error-codes` 枚举了服务端会发的 code，却漏了 `-32021`——`stateless.go` 会返回它，而它自己的标记
  也承诺了它。两个标记对词汇表的说法不一致。
- 四个 list 方法现在都由同一个 `listError` 支撑，于是同一个坏 cursor 不会在一个方法上是 invalid params、
  在另一个上是 internal error。

验收：

```bash
go test ./interaction/mcp/ -run "Cursor|FirstPage" -count=1
```

已作为 `mcp.invalid-cursor-is-invalid-params` 交付，并重述了 `mcp.error-codes`。

### 工作包 6：门禁看不见的标记

- `observability/otel/agreement_test.go:25` 声明了 `otel.metrics-agree-with-the-exposition`，而
  `tools/protocol_behaviour_test.go` 会跳过 `_test.go` 文件——于是那条 promise 不在被评审的快照里、也在
  冻结门禁之外，却在源码里读起来像是两边都覆盖了。
- 两半都做，因为只做一半窟窿还在：promise 移到它该在的 `NewMetrics` 上，并由
  `TestBehaviourMarkersLiveInNonTestSource` 拒绝下一条而不是跳过它。跳过测试文件是对的；**静默地**跳过
  不对。

验收：

```bash
go test ./... -run TestBehaviourMarkersLiveInNonTestSource -count=1   # 在 v2/tools 下
```

### 工作包 7：永远不会到达的响应

- `interaction/mcp/streamable.go:282` 用 `_` 序列化工具结果、又用 `_` 写出去。序列化不了的结果——NaN
  浮点、非字符串键的 map——会发出 `data: `，而客户端在等一个永远不来的答复，且什么都没记录。
  `writeResponse` 经由 `json.NewEncoder` 也是同一个形状，而那里中途失败时字节已经在线上了。
- 两处现在都先序列化再写任何东西，于是要么整个响应到达，要么一条点名故障的 internal error 到达。

验收：

```bash
go test ./interaction/mcp/ -run TestUnserialisableResult -count=1
```

已作为 `mcp.response-is-whole-or-an-error` 交付。

## 里程碑 20（已完成）：请求里的东西不能为自己作证

目标：凡是这个框架做信任判断的地方，它信的那个东西不该是调用方给的。

### 工作包 1：请求的 Host 不是一份凭据

- `interaction/mcp/streamable.go` 放行等于 `"http://"+r.Host` 或 `"https://"+r.Host` 的 `Origin`。
  `Host` 来自调用方，所以在 DNS rebinding 下浏览器会带着 `Host: evil.example` 发
  `Origin: http://evil.example`，而那个比较会说"是"——而这正是 Origin 校验对一个本地监听的 MCP 服务器
  所要拦的攻击。
- 审计把它归类为"已声明，但后果没写明"。结构体注释说了同源请求总是允许；它没有说 `Host` 值几个钱。没有
  任何测试走过这条捷径——这既是它一直没被注意到的原因，也让收紧默认值的测试改动成本为零。
- 考虑过三种做法。无条件收紧最安全，但会破坏"本地起来就能用"。只补文档则把陷阱留在原地。交付的是中间那
  一种，因为它是这个项目在别处一贯的形状：捷径变成 `TrustRequestHost`，默认关闭，于是"信任 `Host`"是
  一件部署方要签字的事。`AllowedOrigins` 仍然是那个不依赖任何调用方可控 header 的答案。
- 这是一次行为变更，因此在 changelog 里配了迁移说明，而不是悄悄收紧：想要旧行为的部署只需设一个字段。

验收：

```bash
go test ./interaction/mcp/ -run "Origin|TrustRequestHost" -count=1
```

已作为 `mcp.request-host-is-not-trusted-by-default` 交付。

## 里程碑 21（完成）：生成器产出什么，也是框架的一部分

目标：生成出来的服务应该和手写的一样正确；一个特性被组合起来用时，行为应该和单独用时一致。

开启它的是两次审计。第一次真正看 `cmd/microgen`——生成器产出一整个服务，却从没被审过——以及一次只看
特性组合、而不是逐包正确性的审计。

作为 v2.21.0 发布。四个工作包落地：生成的服务重新能编译、能正确停机；SSE 保持组件的约定并报告自己的失败；
来自 IDL 的文本在变成代码的地方被转义；生成的文档描述的是生成的 handler。路上发现的两处门禁缺陷——没有
tag 的检出上的空绿、以及带外刷新快照——在同一个版本里修掉。

### 工作包 1：生成的服务重新变对

- v2.20.0 收紧了 MCP 的 Origin 检查，而没有任何模板提到 `AllowedOrigins` 或 `TrustRequestHost`：在
  `cmd/microgen` 里 grep 零命中。生成的服务把原本被服务的浏览器客户端拒了，而且没有任何东西可以指。
  **这一条是本仓库自己造成的回归。**
- 生成的停机路径从不调用 `mcpHandler.Shutdown`，于是会话和它们的 goroutine 活过了停机。现在它跑在 HTTP
  停机之前，因为一个会话握着一条开着的 SSE 流，先关会话才能让 `Shutdown` 结束。
- 生成的成功编码器是手搓的 `json.NewEncoder`：没有 `Content-Type`（于是 `net/http` 把 JSON 答成
  `text/plain`，和同一个生成器写出的 OpenAPI 自相矛盾）、没有 `StatusCoder`、没有 `Headerer`、没有 204
  规则。现在改调 `server.EncodeJSONResponse`。因此变成未使用的 `encoding/json` import 也一起删——否则
  生成的代码编译不过。
- `--grpc` 生成出来的项目编译不过：`go.mod` 既没 require grpc 也没 protobuf，而生成的 `main` 两个都 import。
- 有理由地推后：GORM 驱动的 require。本仓库任何地方都没有 `gorm.io/driver/*` 的版本，凭空编一个会把
  "缺 require"变成"版本不存在"，那更糟。它需要一份钉住的驱动版本表，或者在生成器跑完后的 next-steps
  输出里加一行；后者更好，应该和"生成器跑完之后到底告诉用户什么"一起做。

验收：

```bash
go test ./cmd/microgen/... -count=1
```

### 工作包 2：SSE 保持组件的约定

- `kit.HandleSSETyped` 从来没把 `h.jsonServerOptions` 传给 `NewSSEServerTyped`，而 `kit/doc.go`——
  本仓库在里程碑 18 自己写的那段——把它列在"组件的 JSON server options 全都生效"里。组件级装了
  `ProblemJSONErrorEncoder` 的部署，于是在 JSON 路由上拿到 problem 文档，在 SSE 解码失败时拿到普通信封。
- 同一次审计发现的另两条，先前是**写明**而不是修掉，现在修掉了。中间件拒绝由硬编码的 `JSONErrorEncoder`
  渲染，而同一条路由的解码失败走的是配置出来的那个，于是一条路由有两套错误约定；拒绝现在用
  `SSEServer.ErrorEncoder` 报告的那个编码器。以及那个桥的基础 endpoint 无条件返回 `nil`，于是一条中途死掉
  的流在指标里是一次成功；`SSEServer.ServeStream` 就是原本缺的那个接缝——会把"结束这条流的错误"交回来的
  `ServeHTTP`——而桥把它返回给链。
- 仍然跟不上的是响应本身。一旦流回答了 200 并 flush 了事件，错误只能被记录、不能被渲染，所以桥只在"流从未
  开始"时才编码。要把这两种情形分开，正是基础 endpoint 在 context 里标记 `streamOutcome` 的原因——而不是
  从错误本身去猜。

### 工作包 3：来自 IDL 的文本不是代码

- `template_funcs.go` 的 `escape` 只替换 `"`，不管 `\` 和换行，而且没有任何模板用它：转义机制存在、不完整、
  且是死代码。十九处插值点把 IDL 与数据库文本直接写进 Go 字符串字面量、结构体 tag 与注释。
- 引号那一例最危险，因为它**语法合法**：生成器的 `format.Source` 检查把
  `Description: "a "user" record"` 放了过去，到用户 build 时才失败。换行会在前面的文件已写出之后中断生成。
  列名里的反引号会终止结构体 tag 的原始字符串。
- 替换为三个名字说明自己守什么的辅助函数：`quote`（`strconv.Quote`，一次处理引号、反斜杠、换行与控制字节
  ——这正是用它而不是替换单个字符的理由）、`comment`（折叠所有行终止符，因为 `//` 到换行就结束，而 `.proto`
  输出从不经过格式化器）、`tag`（去掉原始字符串无法转义的字符）。
- 回归测试断言生成片段**能被解析**，并且那个字面量还能还原回输入——"仅语法合法"正是当初放它过去的原因。

验收：

```bash
go test ./cmd/microgen/... -count=1
```

### 工作包 4：文档描述的是 handler

- 每个 message schema 都没有 `additionalProperties`，而生成的解码器设了
  `DisallowUnknownFields`。沉默的含义是"允许多余属性"，于是照文档生成的客户端会被它旁边生成出来的
  服务拒绝。现在每个 message schema 都写 `false`；`ErrorResponse` 有意不写，因为错误编码器是部署方
  的选择，而 `ProblemJSONErrorEncoder` 回答的是一份更宽的文档。
- 指针字段的可空性被丢了三遍：OpenAPI schema、JSON Schema 包、以及 TypeScript SDK 都只写了值的
  类型，而 Go 编码器对 nil 写的是 `null`——生成的结构体不带 `omitempty`。现在普通类型是两元素类型
  列表，`$ref` 包进 `anyOf`（`$ref` 旁边的关键字并非所有实现都认），TypeScript 字段是 `T | null`。
- `required` 不是缺陷，而是一条推导规则，现在写在用户会遇到它的地方：非指针即 required，"声明为指针"
  就是声明可选，而它是关于载荷的陈述，不是"服务端会拒绝缺省"的承诺。非指针字段上无法把缺省和零值
  区分开，于是缺失的 `count` 到达时就是 `0`——这正是"当缺省必须区别于零时，把字段声明成指针"的理由。

验收：

```bash
go test ./cmd/microgen/... -count=1
go -C ./tools test . -run TestMicrogen -count=1
```

### 这两次审计里仍未处理的

已记下文件与行号，尚未动手：

- 一条活着的 SSE 流会吃掉 HTTP 组件停机预算的全部份额，于是任何撞上开着的流的滚动发布都会产出
  `ErrShutdownIncomplete`；`WithTimeout` 是组件级的、没有按路由的出口，于是要托管一条长流就得把它旁边
  JSON 路由的 deadline 也一起去掉。两者都是代码已经写明的机制，但这个取舍没有被写下来。
- `kit/sse.go` 里那个范例只 select `ctx.Done()`，忽略了 `Stopping` 通告，和 `kit/drain.go` 里的范例自相
  矛盾——而写 SSE 的人读的是前者。现在它两个都看。
- 门禁元审计找出的两处"空绿"已经修掉（没有 tag 的检出、带外刷新快照），还有一处那个版本没修：
  `api_surface` 与 `contract_snapshots/*.sha256` 以 digest 存储，没有任何东西迫使审查者去读改了什么。
  这一条转入里程碑 22。

## 里程碑 22（完成）：失败本身就是审查的那种门禁

目标：门禁挡住一处改动时，失败信息和被存下来的文件加在一起，要说清改了什么。一道 diff 没人能读的
门禁，它的刷新命令就是它这辈子会得到的全部审查。

它之后没有发布。这里所有东西都在只属于 workspace 的 `tools` module 和文档里，所以发布出去的 module
和 v2.21.0 逐字节相同，打一个 tag 反而是在说别的。

### 工作包 1：导出面是一份清单，不是一个 hash

- `api_surface.sha256` 每个包存一份 SHA-256。失败只能证明"某个包里有东西动了"；它里面、以及任何
  审查者读过的 diff 里，都不包含那条变化了的声明。刷新一下就绿了，而刷新就是全部的审查。
- 被评审的形式现在是 `api_surface.txt`：声明本身，按 `## <import path>` 分节，34 个包共 2487 行——
  和 Go 自己的 `api/go1.*.txt` 是同一种形状，也小到可以让 `git diff` 就是审查。
- 失败会点名哪些包的分节不同，并引用出现和消失的声明行，按包设上限。它有意不打印整份文件：两段
  2500 行会以另一种方式重犯 digest 的错。
- 验证方式：从快照里删掉一行声明，读失败信息——它点名了 `apperror` 并引用了那一行。

验收：

```bash
go -C ./tools test . -run TestPublicAPISurfaceSnapshot -count=1
```

### 工作包 2：生成契约是一棵 golden 树

- 三份 `contract_snapshots/*.sha256` 对整个生成产物取哈希——manifest、OpenAPI 文档、JSON Schema 包、
  IDL、两个 SDK。失败只说"六份文档里有一份变了"，而回答"怎么变的"意味着手工把生成器跑到一个临时目录
  里。里程碑 21 里那些 OpenAPI 缺陷就是这么审的：这道门禁抓住它们抓了两个版本，却没人能看见它们。
- 被评审的形式现在是产物本身，放在 `tools/testdata/contract_snapshots/<source>/` 下，三个来源共 4235 行。
  每个来源有一份 `files.txt` 列出在审的产物，于是生成器不再产出某个产物时会失败，而不是留下一个没人读的
  golden 文件；同理，新增一个公开产物必须被签字，而不能悄悄出现。
- 失败会点名产物、引用第一处不同的行及其行号，然后指向这个文件自己的 diff。刷新会先清空目录：一棵还留着
  生成器已不再写出的文件的 golden 树，描述的是一个已经不存在的生成器。
- 验证方式：改掉被评审的 `openapi.json` 里的一行——失败点名了文件、行号和两边的内容。

验收：

```bash
go -C ./tools test . -run TestMicrogen -count=1
```

### 对这个里程碑做自我审计发现了什么

上面这些工作里有两处缺陷，都是靠审计它们发现的，不是靠门禁：

- 刷新会先清空某个来源的目录再重建，而拥有这些目录的三个集成测试是与
  `TestEveryContractSnapshotHasALiveCaller` 并行跑的，后者要求每个来源都有 `files.txt`。于是
  `go test ./... -args -update-contract-snapshots` 可能在"正确写入的中途"失败在自己身上。现在
  改成先写索引、之后再剪枝，既保住了"不留过期文件"的保证，也没有那个窗口。
- `AGENTS.md` 声称 `make verify` "就是 CI 跑的东西"。并不是：CI 还在套件旁边跑
  `releasecheck -check-tags`，而这个目标没跑。与其把那句话说软，不如让目标真的跑它——于是这句话
  成立，而本地的绿也不可能比 CI 更绿。
- 审计还查了一处真实风险，结论是没有：`race` 这个 CI job 检出时不拉 tag，而 v2.21.0 让"没有 tag
  的检出"会让 `TestAPICompatibilityWithLastRelease` 失败。那个 job 只对 `v2` module 跑
  `go test -race`，从不跑门禁那个 module，所以两者碰不上。

## 里程碑 23（进行中）：服务发现子树为自己作答

目标：`sd` 从未被审计过。两次审计——负载均衡路径（`balancer`、`selector`、`endpointer`、`instance`）
与韧性路径（`retry`、`feedback`、`health`、`client`）——逐条问"每种情形下部署方损失什么"，答案按
严重程度依次处理。

选择内核比"从未审计"这个说法要好：每个策略都在做任何算术之前用带类型的错误拒绝空集合；每个发布出去
的快照都是先构造完整再整体替换，而不是原地修改；每个 goroutine 都有归属。缺陷集中在算术、事件顺序、
以及并发的读-改-写。

### 工作包 1：带重试的调用不会丢掉它已经收到的答案

- 那个循环同时 select 调用 context 和结果 channel。两者都就绪时 select 等概率选择，于是恰好在预算
  到期时完成的尝试有一半概率被丢弃、并报成 deadline 错误。对非幂等请求这是不可恢复的：写已经发生，
  而调用方被告知没有。
- 改了两处，因为一处不够。尝试会在 outcome 回调**之前**把结果交给循环——`Done` 里的部署代码原本可以
  把这个窗口拉到任意宽——并且循环在服从 context 之前先把已送达的结果取出来。
- 预算也改为在派发之前检查，而不是只在之后检查，于是已经放弃的调用方、或迟到返回的退避计时器，不再
  导致又一次没人读结果的上游请求。
- 两处修复都带 `Stable:` 标记。那个"取出已送达结果"的动作有意不带：它覆盖的窗口——无关的 goroutine
  在 channel 发送的同一瞬间取消——无法被测试确定性地构造出来，而"点名的测试并不断言其文字"的 promise，
  正是里程碑 19 花力气清掉的东西。

验收：

```bash
go test ./sd/... -count=1
```

### 工作包 2：在并发下仍然成立的摘除上限

- `Ejector.apply` 在锁外判定候选、在锁外检查 `MaxEjectionPercent`，然后重新加锁去应用。而这个 filter
  就在选择路径上，所以它每次 pick 都跑一遍，并且天然是并发的。四实例池上的两次 pick 可以各自基于
  "不包含对方决定"的视图通过 50% 检查并一起摘除——在这个上限本就是为了防止的情形里把池子的 75% 摘出
  服务，panic 模式根本不触发，而调用方看到的是 `ErrNoEndpoints`。
- 同一个窗口还让两次 pick 摘除**同一个**实例。摘除是被计数用来加长窗口的，于是"第一次犯错"被记成两次，
  实例被摘出的时长是配置基准的两倍，而那条翻倍阶梯此后一直领先于真实的失败历史。
- 现在判定与应用在同一个临界区里。测量值在进入临界区之前读，而不是在里面读：它们来自 `Table` 自己的
  互斥锁，而 `Table.Follow` 会驱动 `Ejector.Retain`，所以在 `e.mu` 之下读 table 会把这个包已经依赖的
  加锁顺序倒过来。"已返回实例先 reset 再判定"这一点也保住了，否则把它摘出去的那些陈旧测量值会在同一次
  调用里再把它摘一遍。
- 这个测试需要的是一道栅栏，而不只是几个 goroutine。它的第一版用的是普通假时钟，结果在有缺陷的实现上
  也通过了——四个各干几微秒活的 goroutine 不会按需重叠。把它们在时钟调用上对齐之后，重复计数在第一轮
  就失败。

验收：

```bash
go test ./sd/feedback/ -count=1
```

### 工作包 3：一次抖动不等于一个新实例

- `health.Checker` 会删掉缺席地址的状态，并在它返回时用 `initiallyHealthy` 和"零条失败记录"重建。刚被它
  摘出服务的后端因此被重新发布为可服务，并且在 `unhealthyThreshold` 轮之内无法再被摘除——所以一个持续抖动
  的注册中心，能让一个已死的后端在它生命的大部分时间里留在集合中。
- 现在状态能扛过一次缺席，第二次缺席才丢弃。代价是多留一代过期条目；另一种做法（记录离开时间戳）需要
  Checker 目前没有的时钟接缝。
- "遗忘"这一半比字面上要弱，而这点写进了测试：会抑制"与上一次完全相同的事件"的来源（`instance.Cache`
  就是），只有在集合再次变化时才会送出第二次缺席。对于"离开之后集合再也不变"的服务，这会留下每个已离开
  地址一条过期 map 条目。这属于内存卫生而不是正确性，所以记在这里，而不是用一个时钟去解决。

验收：

```bash
go test ./sd/health/ -count=1
```

### 工作包 4：订阅者不会落在已存状态之后

- `instance.Cache.Update` 在锁内写入新状态，释放锁之后才广播。两个并发调用方可以按 A、B 写入，却按
  B、A 送达；而 `sendLatest` 让情况更糟：发现缓冲已满时它会排掉再写入，于是较旧的事件替换掉了较新的。
  此后 `State` 与每个订阅者不一致，直到下一次更新；而由于"与已存状态相同的事件"会被丢弃，陈旧列表再来
  一次会让这种分歧持续下去。
- 现在广播在锁内完成。这是安全的而不是阻塞风险，因为 `sendLatest` 里每个 channel 操作都有 default
  分支——而这恰恰就是当初让"锁外广播"看起来有必要的那条性质。
- 被它替换掉的那个测试推的是二十个**完全相同**的事件，除了"没有 race / panic"什么都不断言。相同事件
  按设计会被丢弃，所以其中十九个从未到达订阅者，而它名字里那个交错场景从未发生。新的测试推的是互不相同
  的事件，并把订阅者手里的东西和 `State` 对比；在锁外广播的实现上，它大约在 200 轮里的第 100 轮失败。
- 值得为下一次审计记一笔：`-race` 对这个缺陷是沉默的。这里没有未同步访问，只有"送达顺序与写入顺序不一致"。
  race 跑干净，不构成关于顺序的证据。

验收：

```bash
go test ./sd/instance/ -count=1
```

### 工作包 5：三处较小的发现

- 加权选择在求和时不设上界（`sd/selector/weighted.go`）。两个注册权重接近 `MaxInt` 的实例会让总和绕成
  负数，"没有可选实例"的守卫触发，一个健康的池子通过 `ErrNoEndpoints` 变得不可路由——而它按分类是临时
  错误，所以调用方会先花完重试预算，且没有任何东西点出那个权重。现在每个权重都被夹到
  `MaxInt / len(instances)`，这不会改变任何现实中的分布，同时让总和不可能溢出。
- `endpointer.Filter` 与 `Prefer` 接受 nil source，把 panic 留给第一个请求
  （`sd/endpointer/filter.go`）。现在两者都在装配时拒绝，和 `selector.Filter`、`selector.New`、
  `balancer.New` 一致。
- `feedback.Follow` 按引用保留了 provider 的实例切片（`sd/feedback/feedback.go`），而在完全相同的边界上
  `health.accept` 做了复制并写明了原因。会重用底层数组的 provider 可以改写一份已经交给 retainer 的集合。
- 三个测试都对着各自的缺陷验证过：pick 0 就报 `ErrNoEndpoints`、nil source 被接受、以及被保留的集合变成
  `[rewritten-a:80 rewritten-b:80]`。

验收：

```bash
go test ./sd/... -count=1
```

### 工作包 6：一次带重试的失败，它的每一个原因都可达

- `retry.Error.Unwrap` 只能给出一个原因，而 `Final` 一旦设置就优先。预算到期会把它设成 context 错误，
  于是上游的 `apperror` kind——它被记在每一次尝试里——对 `errors.As` 不可见，按 kind 分发的映射器会为一个
  "真实类别一直在 `result.Attempts` 里"的失败发出笼统的超时。`Callback` 的替换错误以同样方式遮住了真正的
  错误。
- 现在 `Error` 实现了 `Is` 与 `As`，而 `errors.Is`／`errors.As` 会在走 unwrap 链之前先咨询它们。各次尝试
  与最终原因都可达，且最近的尝试在前。
- 第一次尝试解决它时我把 `Unwrap` 改成返回 `[]error`，`TestAPICompatibilityWithLastRelease` 拒绝了：一个
  已发布的签名变了。门禁是对的——新增两个方法能达到同样的性质而不移除任何东西——而这已是本次会话里它第三次
  拦下一个本可以用"反正还没冻结"糊过去的改动。

验收：

```bash
go test ./sd/retry/ -count=1
go -C ./tools test . -run TestAPICompatibilityWithLastRelease -count=1
```

### 这两次审计里仍未处理的

- `sd/retry` 直接睡眠、直接读时钟（`retry.go`），而这个仓库自己拥有 `endpoint.Clock`，并且在 `feedback`
  和 `endpoint.RetryMiddleware` 里都用了它。后果是：没有任何东西断言这个循环产生的时间表——只断言了孤立的
  `backoff.Next`——所以"延迟如何跨尝试累积"、以及"预算里有多少花在睡觉而不是调用上"，都没被测过。
- 修它需要的是一个 API 决定，而不只是一个接缝。那四个构造函数是位置参数式的（`Retry`、`WithCallback`、
  `WithClassifier`，以及它们的 timeout/balancer 参数）；再加一个接收时钟的位置参数形式，就是同一个调用的
  第四个变体，而这个仓库其余部分对这件事用的是函数式选项——`feedback.WithClock`、
  `feedback.WithEjectorClock`。诚实的形状是 `retry.New(balancer, opts...)`，把现有四个保留为便捷包装。
  那是一次公开 API 新增，带着它自己的快照与文档工作，所以记在这里，而不是在一次会话的末尾半途搭起来。

## 维护规则

- 只在里程碑范围、顺序或验收标准变化时更新本文件。
- 已完成的行为记录在 `CHANGELOG.md` 中，而不是在这里不断增长的状态笔记。
- 具体用法放在 `README*` 或 `MICROGEN.md`，包设计放在 `ARCHITECTURE.md`。
- 每个进行中的里程碑在实现被认为完成之前，都必须有专项测试和端到端验证路径。
