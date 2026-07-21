# go-kit - Go 微服务框架

[![Go Version](https://img.shields.io/badge/go-1.25+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE.txt)

[English](README.md) | 简体中文

`go-kit` 是一个围绕稳定分层构建的 Go 微服务框架：

```text
Service -> Endpoint -> Transport
```

你只需要定义一次服务契约，`microgen` 就可以生成可运行项目，包括 HTTP 路由、可选 gRPC、配置、客户端、SDK、生成文档，以及 AI 工具发现元数据。

## 发布状态

当前推荐版本：

```text
v2.0.0 Stable
github.com/dreamsxin/go-kit/v2
```

当前维护的产品线位于 [`v2/`](v2/)，它是独立 Go 模块。根目录模块继续作为
v1.6 维护与源码历史保留。

本文后续命令面向旧版 v1。新建项目和当前 `microgen` 工作流请以
[v2 使用文档](v2/README_zh.md)为准。

稳定范围：

- 核心 `service -> endpoint -> transport` 运行时分层
- 已文档化的 `kit`、`endpoint`、HTTP transport、服务发现、日志和 `microgen` CLI 行为
- 生成的 unary HTTP/gRPC 项目
- 生成配置、extend 模式、客户端、SDK 和 AI skill 元数据
- 支持的 server-stream、client-stream、bidirectional-stream Proto gRPC 生成输出
- `interaction` 和 `interaction/mcp` — AI interaction runtime，包含 session、event、tool、resource、prompt、hook 和完整 MCP 2025-06-18 Streamable HTTP 传输
- 生成的 interaction adapters

当前版本契约详见 [v2/RELEASE.md](v2/RELEASE.md)、
[v2/ROADMAP.md](v2/ROADMAP.md) 和 [v2/MIGRATION.md](v2/MIGRATION.md)。

## 快速开始：本地生成服务

如果你想创建一个新服务，并让人或 AI 编程助手继续开发，推荐走这条路径。

### 1. 安装 `microgen`

```bash
go install github.com/dreamsxin/go-kit/cmd/microgen@latest
```

### 2. 创建项目

```bash
mkdir hello-svc
cd hello-svc
```

### 3. 定义服务契约

创建 `idl.go`：

```go
package hello

import "context"

type HelloRequest struct {
	Name string `json:"name"`
}

type HelloResponse struct {
	Message string `json:"message"`
}

type HelloService interface {
	// SayHello returns a greeting.
	SayHello(ctx context.Context, req HelloRequest) (HelloResponse, error)
}
```

### 4. 生成并运行

```bash
microgen -idl idl.go -out . -import example.com/hello-svc -config=false -model=false -db=false
go mod tidy
go run ./cmd/main.go
```

### 5. 检查服务

```bash
curl http://localhost:8080/health
curl http://localhost:8080/debug/routes
curl http://localhost:8080/skill
curl "http://localhost:8080/skill?format=mcp"
```

刚生成的业务方法会返回脚手架的 `not implemented` 错误。真实业务逻辑写在：

```text
service/helloservice/service.go
```

## AI Agent 工作流

生成项目后，优先把这些文件和运行时信息交给 AI 编程助手：

- 生成项目里的 `README.md`
- `idl.go`，从 Go IDL 生成时的服务契约快照
- `service/<name>/service.go`，业务逻辑文件
- `GET /debug/routes`，实时路由表
- `GET /skill?format=mcp`，AI 工具发现视图

推荐提示词：

```text
先阅读 README.md 和 idl.go。业务逻辑只写在 service/<name>/service.go。
不要手动修改 cmd/generated_*.go、endpoint/*/generated_chain.go、skill/ 等生成器管理文件。
使用 /debug/routes 和 /skill?format=mcp 理解生成的服务能力。
```

`/skill?format=mcp` 是发现元数据，不是工具执行端点。可执行 AI session 应使用 `interaction` runtime 和 `interaction/mcp` adapter。

## 应该改哪里

生成项目会区分用户拥有的文件和生成器拥有的文件。

可以改：

- `service/<svc>/service.go`：业务逻辑
- `endpoint/<svc>/custom_chain.go`：自定义 endpoint 中间件
- `cmd/custom_routes.go`：自定义 HTTP 路由
- `config/config.yaml`：本地配置

不要手动改：

- `cmd/generated_*.go`
- `endpoint/<svc>/generated_chain.go`
- `model/generated_*.go`
- `repository/generated_*.go`
- `client/`、`sdk/`、`skill/`、生成的 `pb/` 资源

## 扩展已有生成项目

先运行只读兼容性检查：

```bash
microgen extend -check -out .
```

然后基于完整合并后的 Go IDL 契约追加一个能力：

```bash
microgen extend -idl full_combined.go -out . -append-service OrderService
microgen extend -idl full_combined.go -out . -append-model Product
microgen extend -idl full_combined.go -out . -append-middleware tracing,error-handling,metrics
```

extend 模式只更新新文件和生成器拥有的聚合接缝，设计目标是保留用户写的实现文件。

## 常见生成模式

### 从 Go IDL 生成

```bash
microgen -idl idl.go -out . -import example.com/mysvc
```

### 从 Protobuf 生成

```bash
microgen -idl service.proto -out . -import example.com/mysvc -protocols http,grpc
```

Proto 项目需要先检查 `pb/` 下生成的 proto 资源，运行 `protoc` 后再启动服务。

当 Proto 契约使用支持的 server-stream、client-stream 或 bidirectional-stream 形状时，生成的 gRPC 输出支持流式 RPC。

### 从数据库生成

```bash
microgen -from-db -driver mysql -dsn "user:pass@tcp(localhost:3306)/dbname" -out . -import example.com/mysvc
```

数据库生成对源数据库是只读的。生成的 model 会反映真实表字段，不会凭空添加表里不存在的审计字段。生成服务启动时也默认跳过 GORM `AutoMigrate`；需要迁移时必须显式设置 `database.auto_migrate: true`、`APP_DB_AUTO_MIGRATE=true` 或启动参数 `-auto-migrate`。

## 配置模式

生成配置按下面顺序加载：

```text
默认值 -> 本地 YAML -> 环境变量 -> 可选远程配置
```

生成时选择模式：

```bash
# 本地文件 + 环境变量
microgen -idl idl.go -out . -import example.com/mysvc -config-mode file

# 本地文件 + 环境变量 + 远程配置，远程失败时回退本地
microgen -idl idl.go -out . -import example.com/mysvc -config-mode hybrid -remote-provider consul

# 远程优先，远程加载失败则启动失败
microgen -idl idl.go -out . -import example.com/mysvc -config-mode remote -remote-provider consul
```

环境变量使用 `APP_` 前缀，例如 `APP_HTTP_ADDR`、`APP_LOG_LEVEL`、`APP_LOG_FORMAT`、`APP_REMOTE_ENABLED`、`APP_DB_AUTO_MIGRATE`。

生成的 `logging.level` 和 `logging.format` 会用于创建服务 logger。endpoint 限流默认开启；入站熔断和 retry 默认关闭，需要通过 `middleware.circuit_breaker.enabled` 和 `middleware.retry.enabled` 显式开启。生成的 retry 只重试显式实现 `Retryable() bool` 并返回 true 的错误，普通业务校验错误不会被重复执行。

## AI 与 MCP

启用 skill 生成后，生成服务会暴露 AI 可读的工具定义。默认会启用。

- OpenAI 风格工具描述：`GET /skill`
- MCP 风格工具描述：`GET /skill?format=mcp`

响应包含元数据：

- `schemaVersion`，当前是 `microgen.skill.v1`
- `source`，当前是 `microgen-ir`
- `services`
- `formats`

可执行 AI session 和 tool-call loop 使用 interaction runtime：

- `interaction.NewRuntime`：session、event、tool、resource、prompt 和 hook
- `interaction.AuthorizationHook` 与 `interaction.AuditHook`：策略和审计
- `interaction/mcp.NewHandler`：Streamable HTTP MCP 传输（`NewStreamableHandler` 的别名，支持 POST/GET/DELETE + SSE）

MCP 端点实现协议版本 2025-06-18，支持 tools、resources、prompts、completions、logging、sampling 和服务器发起的 notifications。

详见 [interaction/README.md](interaction/README.md)、[examples/interaction_policy](examples/interaction_policy) 和 [examples/mcp_full](examples/mcp_full)。

## 生产指导

生产采用前建议阅读：

- [RELEASE.md](RELEASE.md)：发布范围和验证
- [STABILITY.md](STABILITY.md)：stable、semi-stable、internal 表面
- [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)：生成输出兼容性
- [OBSERVABILITY.md](OBSERVABILITY.md)：tracing、metrics、logging、request correlation 和 OpenTelemetry 集成
- [SECURITY_HARDENING.md](SECURITY_HARDENING.md)：认证、授权、请求限制、审计、密钥和生成项目安全加固

## 架构

框架保持清晰职责边界：

```text
Service
  纯业务逻辑，不依赖 HTTP 或 gRPC。

Endpoint
  运行时策略：中间件、日志、指标、限流、熔断。

Transport
  协议适配：HTTP、gRPC、请求解码、响应编码。
```

生产服务推荐使用 `microgen`。小型原型或极简服务可以直接使用 `kit` 包。两条路径保持同一套 service -> endpoint -> transport 形态：`kit.HandleJSON` 是简洁路由入口，`kit.HandleJSONEndpoint` 用于把已有 `endpoint.Endpoint` 接到 HTTP transport。

## 使用 `kit` 写一个极简原型

```go
package main

import (
	"context"

	"github.com/dreamsxin/go-kit/kit"
)

type HelloReq struct {
	Name string `json:"name"`
}

type HelloResp struct {
	Message string `json:"message"`
}

func main() {
	svc := kit.New(":8080")

	kit.HandleJSON[HelloReq](svc, "/hello", func(ctx context.Context, req HelloReq) (any, error) {
		return HelloResp{Message: "Hello, " + req.Name + "!"}, nil
	})

	svc.Run()
}
```

通过 `kit.WithTimeout`、`kit.WithMetrics`、`kit.WithLogging`、`kit.WithRateLimit` 或 `kit.WithCircuitBreaker` 配置的 endpoint 中间件会应用在 `kit.HandleJSON` 和 `kit.HandleJSONEndpoint` 上。普通 `svc.Handle` 和 `svc.HandleFunc` 是原生 HTTP 逃生口，适合静态文件、第三方 handler、探针或自定义协议端点；它们会得到 HTTP context/request ID 注入，但不会运行 endpoint 中间件。

`kit.New` 默认暴露 `/health`、`/livez` 和 `/readyz`。需要进程或就绪探针之外的依赖检查时，可以添加 `kit.WithLivenessCheck` 或 `kit.WithReadinessCheck`。

## 生成项目结构

```text
.
|-- cmd/main.go
|-- cmd/generated_*.go
|-- cmd/custom_routes.go
|-- config/
|-- service/<svc>/
|-- endpoint/<svc>/
|-- transport/<svc>/
|-- client/<svc>/
|-- sdk/<svc>sdk/
|-- model/
|-- repository/
|-- pb/
|-- docs/
|-- skill/
`-- idl.go
```

## 修改本仓库本身

如果你要修改框架本身，而不是把它作为依赖使用：

- 从 [v2/MAINTAINING.md](v2/MAINTAINING.md) 开始。
- 用 [v2/DOCS_INDEX.md](v2/DOCS_INDEX.md) 查看当前文档地图。
- 阅读 [v2/ROADMAP.md](v2/ROADMAP.md) 了解实施状态和范围。
- 使用 [v2/RELEASE.md](v2/RELEASE.md) 执行发布校验和标签操作。

## License

[MIT](LICENSE.txt)
