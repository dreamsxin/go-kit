# go-kit - Go 微服务框架

[![Go Version](https://img.shields.io/badge/go-1.25+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE.txt)

[English](README.md) | 简体中文

`go-kit` 是一个围绕清晰分层构建的 Go 微服务框架：

```text
Service -> Endpoint -> Transport
```

你只需要定义一次服务能力，`microgen` 就可以生成一个可运行项目，包括 HTTP 路由、可选 gRPC、配置、SDK，以及 AI 工具元数据。

## 发布状态

当前定位：

```text
v0.8 Beta
```

本框架适合内部服务、原型项目，以及团队接受 pre-v1 演进的受控生产试点。它还不是工业级 v1.0 正式发布。

下一阶段目标是 `v0.9 AI Interaction Preview`，重点是 gRPC 流式接口、WebSocket transport，以及 AI 交互运行时。详见 [RELEASE.md](RELEASE.md) 和 [AI_FIRST_ROADMAP.md](AI_FIRST_ROADMAP.md)。

## 从这里开始：本地生成一个服务

如果你想创建一个新服务，并让人或 AI 继续开发，推荐走这条路径。

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

### 5. 检查生成服务

另开一个终端：

```bash
curl http://localhost:8080/health
curl http://localhost:8080/debug/routes
curl http://localhost:8080/skill
curl "http://localhost:8080/skill?format=mcp"
```

刚生成的业务方法会返回脚手架的 “not implemented” 错误。真实业务逻辑写在：

```text
service/helloservice/service.go
```

## 让 AI 快速接手

生成项目后，优先把这些文件和运行时信息交给 AI 编程助手：

- 生成项目里的 `README.md`
- `idl.go`，服务契约快照
- `service/<name>/service.go`，业务逻辑文件
- `GET /debug/routes`，实时路由表
- `GET /skill?format=mcp`，MCP 工具视图

推荐提示词：

```text
先阅读 README.md 和 idl.go。业务逻辑只写在 service/<name>/service.go。
不要手动修改 cmd/generated_*.go、endpoint/*/generated_chain.go、skill/ 等生成器管理文件。
使用 /debug/routes 和 /skill?format=mcp 理解当前服务能力。
```

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

### 从数据库生成

```bash
microgen -from-db -driver mysql -dsn "user:pass@tcp(localhost:3306)/dbname" -out . -import example.com/mysvc
```

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

环境变量使用 `APP_` 前缀，例如 `APP_HTTP_ADDR`、`APP_LOG_LEVEL`、`APP_REMOTE_ENABLED`。

## AI 与 MCP 集成

启用 skill 生成后，生成服务会暴露 AI 可读的工具定义。默认会启用。

- OpenAI 风格工具：`GET /skill`
- MCP 风格工具：`GET /skill?format=mcp`

响应包含 `metadata`：

- `schemaVersion`，当前是 `microgen.skill.v1`
- `source`，当前是 `microgen-ir`
- `services`
- `formats`

这样 AI agent 不需要反向分析 HTTP handler，就能发现服务方法并作为工具调用。

规划中的 preview 能力：

- gRPC server-stream、client-stream、bidirectional-stream 流式方法
- 面向浏览器和 Agent 交互循环的 WebSocket transport
- 支持 session、event、tool call、取消和审计 hook 的 AI 交互运行时

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

生产服务推荐使用 `microgen`。小型原型或极简服务可以直接使用 `kit` 包。

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

	svc.Handle("/hello", kit.JSON[HelloReq](func(ctx context.Context, req HelloReq) (any, error) {
		return HelloResp{Message: "Hello, " + req.Name + "!"}, nil
	}))

	svc.Run()
}
```

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

- 从 [MAINTAINER_GUIDE.md](MAINTAINER_GUIDE.md) 开始。
- 用 [DOCS_INDEX.md](DOCS_INDEX.md) 查看文档地图。
- 阅读 [PROJECT_SNAPSHOT.md](PROJECT_SNAPSHOT.md) 了解当前状态。
- 使用 [PROJECT_WORKFLOW.md](PROJECT_WORKFLOW.md) 选择验证命令。

## License

[MIT](LICENSE.txt)
