# go-kit v2 使用手册

[English](MANUAL.md) | 简体中文

本手册是 go-kit v2 的完整使用指南，按「快速入门 -> 核心概念 -> 组件 ->
生产部署」组织。每章都可以独立阅读，但按顺序阅读能最快建立整体认知。

## 第一部分：快速入门

**目标：15 分钟内跑通第一个服务。**

1. **安装与生成** -- 安装 microgen，从 IDL 生成一个可运行的 HTTP 服务
   → [README：生成服务](README_zh.md)
2. **理解生成物** -- 哪些文件归你所有，哪些由生成器持有
   → [MICROGEN：生成项目清单](MICROGEN_zh.md)
3. **走通请求链路** -- 在可运行示例上体验 Service -> Endpoint -> Transport
   → [examples 导览](examples/README_zh.md)
4. **手工组装** -- 用 `kit` 从零构建一个小型服务
   → [README：使用 kit 构建](README_zh.md)

## 第二部分：核心概念

在写第一行代码之前，先理解框架的三个不变量。

### 2.1 请求链路

```text
Transport request
    -> decode
    -> endpoint middleware
    -> endpoint
    -> service method
    -> encode
    -> transport response
```

每一层只拥有一类决策：

| 层 | 拥有 | 不得拥有 |
| --- | --- | --- |
| Service | 业务规则与领域编排 | HTTP/gRPC 类型与状态映射 |
| Endpoint | 传输无关的请求边界与中间件 | Socket/服务器生命周期 |
| Transport | 协议编解码、头部与状态 | 业务规则与重试策略 |
| Assembly | 依赖装配与进程生命周期 | 隐藏的全局状态 |

详见 [ARCHITECTURE](ARCHITECTURE_zh.md)。

### 2.2 错误处理

业务错误在 `apperror` 中分类，传输层自动映射为协议状态码：

```go
return Todo{}, apperror.New(apperror.KindNotFound, "todo.not_found", "todo not found")
```

- `KindInvalidArgument` -> 400
- `KindUnauthenticated` -> 401
- `KindPermissionDenied` -> 403
- `KindNotFound` -> 404
- `KindAlreadyExists`/`KindConflict` -> 409
- `KindResourceExhausted` -> 429
- 未分类 -> 500（对客户端不透明）

### 2.3 生命周期

进程入口持有信号与根 context；服务在启动成功后持有监听器与优雅停机：

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
if err := svc.Run(ctx); err != nil {
    return err
}
```

可选组件（后台任务、gRPC 服务器）通过 `kit.Lifecycle` 接入同一个停机边界。

## 第三部分：组件（按请求链路）

组件导览按请求流经组件的顺序排列。每节给出经典使用示例和文档链接。

### 3.1 endpoint：端点与中间件

endpoint 是传输无关的请求函数，所有横切关注点都通过中间件组合：

```go
ep := endpoint.NewBuilder(createUser).
    WithValidation().
    WithTimeout(5*time.Second).
    Use(endpoint.NewCircuitBreaker(...).Middleware()).
    Use(endpoint.RateLimitMiddleware(limiter)).
    WithMetrics(&metrics).
    Build()
```

- [endpoint 指南](endpoint/README_zh.md)：Builder、Chain、TypedEndpoint、
  四种中间件流控模式（短路/分支/重复/替换）
- 可运行示例：[examples/middleware](examples/README_zh.md)

### 3.2 transport：协议适配

transport 把 endpoint 适配到 HTTP/gRPC，负责编解码、头部、状态、分页、
文件上传、SSE 与链路追踪传播：

```go
// 服务端
server.NewTypedJSONServer(ep, server.ServerResponseEncoder(encodeAPIResponse))

// 客户端
client.NewJSONClient(http.MethodPost, url, enc, dec,
    client.Before(transporthttp.InjectTraceparent),
)
```

- [transport 指南](transport/README_zh.md)：组合与嵌套语义、分页约定、
  多解析器组合
- 可运行示例：[examples/envelope](examples/README_zh.md)、
  [examples/httpclient](examples/README_zh.md)

### 3.3 kit：服务组装

kit 把 endpoint 和 HTTP transport 组装成可运行的服务，持有健康检查、
生命周期与优雅停机：

```go
svc, err := kit.New(":8080",
    kit.WithRequestID(),
    kit.WithTimeout(5*time.Second),
    kit.WithJSONServerOptions(
        server.ServerResponseEncoder(encodeAPIResponse),
        server.ServerErrorEncoder(encodeAPIError),
    ),
)
kit.HandleJSONTyped(svc, "/hello", hello)
```

- [使用 kit 构建](README_zh.md)
- 可运行示例：[examples/quickstart](examples/README_zh.md)、
  [examples/todosvc](examples/README_zh.md)、[examples/auth](examples/README_zh.md)

### 3.4 sd：服务发现与负载均衡

服务发现把实例集合解析为可调用的 endpoint，带负载均衡与受控重试：

```go
call, closer, err := sdclient.NewEndpoint(instancer, factory, logger,
    sdclient.WithMaxAttempts(3),
    sdclient.WithTimeout(500*time.Millisecond),
)
```

- [sd 指南](sd/README_zh.md)：Instancer 契约、endpointer、round-robin、retry
- 可运行示例：[examples/sd](examples/README_zh.md)

### 3.5 interaction：AI 交互与 MCP

interaction 运行时把工具、资源、提示词与策略钩子暴露为 MCP Streamable
HTTP：

```go
rt := interaction.NewRuntime().WithHooks(
    interaction.AuthorizationHook{Authorizer: allowTools("echo")},
)
mux.Handle("/mcp", interactionmcp.NewHandler(rt))
```

- [interaction 指南](interaction/README_zh.md)
- 可运行示例：[examples/mcp_basic](examples/README_zh.md)、
  [examples/interaction_policy](examples/README_zh.md)

### 3.6 security：HTTP 安全中间件

security/http 提供可信代理、CORS、CSRF、安全响应头与 IP 策略：

```go
handler := httpsecurity.Chain(
    httpsecurity.TrustedProxy(proxy),
    httpsecurity.CORS(corsPolicy),
    httpsecurity.CSRF(csrfConfig),
)(rootHandler)
```

- [security/http 指南](security/http/README_zh.md)
- 可运行示例：[examples/auth](examples/README_zh.md)

### 3.7 observability：可观测性

标准库 slog 适配器与 OpenTelemetry 适配器分别覆盖日志与追踪/指标：

```go
ep = slogadapter.LoggingMiddleware(logger, "CreateUser")(ep)
ep = oteladapter.TracingMiddleware(tracer, "GetUser")(ep)
```

- [slog 适配器](observability/slog/README_zh.md)、
  [otel 适配器](observability/otel/README_zh.md)
- 生产部署指导：[PRODUCTION：Logging/Metrics/Tracing](PRODUCTION_zh.md)

### 3.8 可选集成：Consul 与 gRPC

Consul 服务发现与 gRPC 传输是独立模块，按需安装：

```bash
go get github.com/dreamsxin/go-kit/v2/integrations/consul@v0.2.3
go get github.com/dreamsxin/go-kit/v2/integrations/grpc@v0.2.3
```

- [Consul 集成](integrations/consul/README_zh.md)
- [gRPC 集成](integrations/grpc/README_zh.md)

### 3.9 microgen：代码生成器

microgen 从 Go IDL、Protobuf 或数据库模式生成完整项目：

```bash
microgen -idl idl.go -out ./service -import example.com/service
microgen -from-db -driver sqlite -dsn ./app.db -tables todos -out ./svc
```

- [microgen 指南](MICROGEN_zh.md)：源模式、选项、生成所有权、extend 模式
- 可运行示例：[examples/usersvc](examples/README_zh.md)（IDL 输入）

## 第四部分：生产部署

- [生产指南](PRODUCTION_zh.md)：生命周期、HTTP 配置、服务发现、配置、
  认证授权、浏览器安全、日志、指标、追踪、健康检查、后台任务、部署、
  告警
- [升级说明](MIGRATION_zh.md)：版本间升级动作

## 附录

- [API 参考](https://pkg.go.dev/github.com/dreamsxin/go-kit/v2)
- [变更日志](CHANGELOG_zh.md)
- [文档导航](DOCS_INDEX_zh.md)
