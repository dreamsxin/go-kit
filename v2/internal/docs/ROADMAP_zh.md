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

## 维护规则

- 只在里程碑范围、顺序或验收标准变化时更新本文件。
- 已完成的行为记录在 `CHANGELOG.md` 中，而不是在这里不断增长的状态笔记。
- 具体用法放在 `README*` 或 `MICROGEN.md`，包设计放在 `ARCHITECTURE.md`。
- 每个进行中的里程碑在实现被认为完成之前，都必须有专项测试和端到端验证路径。
