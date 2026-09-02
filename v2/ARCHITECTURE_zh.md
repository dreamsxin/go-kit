# 架构与边界

[English](ARCHITECTURE.md) | 简体中文

本文档定义 go-kit v2 的长期架构，描述所有权与扩展规则，而不是临时的实施
路线图。

## 产品范围

go-kit v2 是一个组件化框架，用于构建具有一致运行时模型和契约驱动生成器
的 Go 服务。

框架提供：

- service、endpoint、transport 三层分离；
- 承载横切请求行为的 endpoint 中间件；
- HTTP 与 gRPC 适配器；
- 服务发现、负载均衡与受控执行；
- 交互原语与 MCP 传输；
- 从 Go IDL、Protobuf 和数据库模式生成项目；
- 通过 `kit` 提供的小型服务组装 API。

核心不提供业务平台。IAM、outbox 工作流、任务租约、对象存储、密钥管理和
完整事务框架属于独立的集成模块或应用本身。

## 请求链路

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

## 包职责

### `kit`

`kit` 是面向小型服务的高层组装脚手架。传输中立的 `Host` 编排生命周期组件，不拥有任何协议；`HTTP` 组件组合标准的 endpoint 与 HTTP transport 包，并通过供应商中立的 `kit.Lifecycle` 契约挂载到 Host。

- `kit.NewHTTP` 与 `kit.NewHost` 校验配置并返回错误。
- `Host.Run(ctx)` 跟随调用方持有的 context。
- `kit.HandleJSONTyped`、`kit.HandleJSON` 和 `kit.HandleJSONEndpoint`
  保留 endpoint 中间件；具体响应类型优先使用类型化入口。
- `HTTP.Handle` 和 `HTTP.HandleFunc` 是原生 HTTP 逃生口。
- `kit/grpc` 是可选的生命周期组件，核心 `kit` 不导入它。

不应仅为减少几行 endpoint 接线代码就把应用路由挪到原生 HTTP handler。

### `endpoint`

`endpoint` 定义与传输无关的请求函数，以及标准库中间件组合、超时与指标。
供应商相关的日志、限流与熔断保持为显式适配器。

`Recorder` 是指标扩展点：`RecordingMiddleware` 把每次调用的 `Observation`
（操作名、耗时、错误）交给它，任何后端桥接都只是该接口的一个实现。`Metrics`
是内置的内存采集器，其计数器不导出且由内部锁保护，读取入口为 `Snapshot()`、
`SnapshotFor(operation)` 与 `Operations()`，都返回无锁、可复制的值。

Endpoint 中间件观察业务调用结果，不应从 HTTP 状态码或 gRPC 线上细节
推断错误。

### `transport`

传输包把 endpoint 适配到协议：

- `transport/http/server` 与 `transport/http/client`；
- `integrations/grpc/server` 与 `integrations/grpc/client`。

它们拥有受限解码、响应状态处理、协议元数据、流式接口（含 Server-Sent
Events 服务端）与传输特有错误。
它们不决定一个业务操作是否可以安全重试。

### `sd`

根 `sd` 包拥有供应商中立的服务发现契约。`sd/endpointer`、`sd/selector`、
`sd/balancer`、`sd/retry`、`sd/feedback` 与 `sd/health` 都可独立使用；
`sd/client` 是可选的便捷组合。
更新以快照形式交付，不是调用方可变的切片。`Balancer.Pick` 返回带实例身份的
`Picked` 以及 `Done(Outcome)`，因此 retry 等调用方可以把按实例结果回灌到
进程内反馈表，而不必把实时指标写入注册中心。按实例的动态状态只存放在一处
——`feedback.Table`；而"哪些地址当前被摘除"这类策略状态归策略自己所有，
因为它是按（策略，实例）而不是按实例存在的。主动探测是 `Instancer` 的装饰器
而不是独立的一层，因此加上它不需要改动下游任何一层。取消会同时中断调用与
重试退避。`Instancer.Close` 与构造函数返回的关闭器负责订阅 goroutine 和工厂
创建连接的生命周期。协议级重试分类属于协议适配器，而非通用发现层。Consul 与
etcd 支持位于独立的 integration 模块。

### `interaction`

`interaction` 定义工具、资源、提示词、会话、通知与策略钩子。
`interaction/mcp` 通过 MCP Streamable HTTP 暴露这些能力。

MCP 是一个可选的、基于标准的集成面。通用契约发现仍是 OpenAPI/JSON
Schema；框架不维护并行的私有工具发现端点。

Provider 实现必须复制调用方可变数据，并且不得在持有内部锁时回调用户
代码。

### `log`

`log` 曾是服务于直接重构之前生成项目的已废弃标准库兼容门面，现已移除。
应用直接使用 `log/slog` 并显式选择供应商适配器。库返回错误；进程入口决定
何时终止。

### 可选可观测性适配器

`observability/slog` 把 endpoint 结果与传输错误适配到标准库 `log/slog`
API。`integrations/zap` 拥有等价的 Zap 专用适配器，使核心包保持供应商
中立。`observability/otel` 是独立模块，把 endpoint 调用适配到应用自有的
OpenTelemetry tracer 与 meter。这些适配器不记录请求/响应载荷；操作名和
应用属性必须保持有界。

### 可选安全

`security` 定义传输中立的主体契约：已认证主体（`Subject`）、
`Authenticator`、主体上下文传递，以及粗粒度强制的 endpoint 中间件
（`RequireAuthenticated`、`RequireRole`）。失败统一通过 `apperror` 分类，
由各传输一致映射。凭据提取与验证保持协议专属、应用自有。

`security/http` 以可信代理解析、客户端 IP 策略、CORS、签名双提交 CSRF
和安全响应头包装标准库 handler。它围绕传输 handler 组装，不改变 endpoint
契约。认证在协议边界确立主体；业务授权保留在 endpoint 或 service 策略中。

### `cmd/microgen`

`microgen` 是构建期工具。解析器产出统一 IR，驱动 HTTP 路由、传输、Go 与
TypeScript SDK、OpenAPI 3.1、JSON Schema 2020-12 以及可选的 MCP 工具
适配器。模板从该 IR 渲染项目。运行时包不得依赖生成器内部实现。解析器、
schema、IR 与生成实现包位于 `cmd/microgen/internal` 之下；CLI 是受支持
的入口。

源模式与生成文件所有权见 [MICROGEN.md](MICROGEN_zh.md)。

## 中间件边界

Endpoint 中间件与 HTTP 中间件刻意区分：

- endpoint 中间件看到的是解码后的请求、业务响应与业务错误；
- HTTP 中间件看到的是方法、路径、头部、状态码与字节流。

通过 `kit.WithEndpointMiddleware` 安装的 endpoint 中间件作用于经
`HandleJSON` 或 `HandleJSONEndpoint` 注册的路由。原生 handler 只接收
显式安装的 HTTP 中间件。持有依赖的适配器（如 Zap、令牌桶）
由应用创建并通过这个通用选项传入。熔断与限流已内置于核心 `endpoint`
包，不持有第三方依赖。

`kit.WithHTTPMiddleware` 是标准 `http.Handler` 中间件的显式全服务器
边界。它包装健康检查、JSON endpoint、原生 HTTP 与生成路由，但不把 HTTP
策略转换成 endpoint 中间件。

熔断器作用域由应用持有。当路由之间不应共享熔断状态时，为每个路由创建
独立适配器。业务校验错误不应被当作基础设施故障，除非应用显式如此分类。

## 错误与重试契约

- 库返回错误，而不是记录 fatal 日志或安装信号处理器。
- 传输客户端把非成功协议状态视为错误。
- 重试是可选项。生产调用方应提供显式的可重试错误分类；内置默认把未知
  错误视为永久错误。
- 写操作不会仅因出错而被重试。
- 退避等待遵循 context 取消。

## 生命周期契约

进程入口持有信号与根 context。框架服务在启动成功后持有监听器与优雅停机。

```text
main creates signal context
    -> assemble dependencies
    -> start service
    -> wait for cancellation or serve error
    -> bounded graceful shutdown
    -> return final error to main
```

启动错误应尽可能同步返回。服务实例不能被启动两次，停机后也不能重启。

组件可以实现 `kit.NamedLifecycle`，为启动、异步故障与停机诊断提供稳定
名称；实现 `kit.ReadinessProvider` 可把异步预热桥接到 `/readyz` 与
`/health` 的就绪检查。组件的异步错误会持续汇报到服务错误通道直至停机。

持有资源的构造函数返回 closer。停机按消费者到提供者的顺序进行：先关闭
endpoint/endpointer 资源再停止其 Instancer，然后关闭传输与进程级依赖。

## 扩展规则

按以下顺序优先：

1. 组合既有公开包。
2. 在拥有该行为的包上增加小的选项或接口。
3. 增加可选的集成包。
4. 仅当行为被广泛需要时才修改核心契约。

避免全局注册表、隐藏 goroutine、包级进程控制，以及为单一应用加入框架
分支。

## 模块依赖层级

```text
L0 基础层（无第三方依赖，可独立使用）
   apperror · endpoint · transport（根） · transport/http · sd ·
   interaction · security

L1 传输适配层（依赖 L0）
   transport/http/server · transport/http/client · security/http

L2 组装层（依赖 L0+L1）
   kit · kit/grpc（独立 module）

L3 可选组合（独立包，不引入新依赖）
   observability/slog · sd/client

L4 独立 module 扩展层（第三方依赖，独立版本演进）
   observability/otel · integrations/zap · integrations/grpc ·
   integrations/consul

L5 构建期工具（不进运行时依赖图）
   cmd/microgen
```

依赖只允许向下指向。L0 包不得导入 L2–L5。`sd` 需要调用方提供 `Instancer`
实现（如 `integrations/consul`）；`kit` 自包含，自带 HTTP 服务器。
`cmd/microgen` 的生成产物只依赖 L0–L3。

## 上下文传递规范

- 框架的每个 context 键都以成对的导出函数交付：`WithXxx(ctx, v)` /
  `XxxFromContext(ctx)`；键类型本身永远不导出。
- 请求关联值（trace 上下文、请求 ID）归 `endpoint`；协议原生对象
  （`*http.Request`、响应写入器）归传输层，endpoint 与 service 层禁止读取。
- 认证主体经 `security.WithSubject` / `security.SubjectFromContext` 传递。
- 业务值（用户模型、事务句柄）不得占用框架保留键，一律通过请求结构体传递。

## 稳定性

`v2.6.0` 是当前已发布契约。它是经批准的架构进化版本：装配层（`kit.Service`
拆分为 `kit.Host` + `kit.HTTP`）、错误模型（移除 `server.HTTPError`）与生成的自定义路由钩子发生了经明确批准的不兼容变更；例外已记录在发布策略中。之后的补丁版本修复行为，次版本新增能力。进一步的不兼容变更需要新的主 module 版本，除非另行批准。
