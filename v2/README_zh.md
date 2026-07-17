# go-kit v2

[![Go Version](https://img.shields.io/badge/go-1.25.8+-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE.txt)

[English](README.md) | 简体中文

`go-kit/v2` 是一个组件化 Go 服务框架，所有入口遵循同一条请求链路：

```text
Service -> Endpoint -> Transport
```

可以只选取需要的包，也可以用 `microgen` 从 Go 接口、Protobuf 契约或数据库
结构生成完整的可运行服务。

## 当前状态

v2 是独立 Go module：

```text
github.com/dreamsxin/go-kit/v2
```

v2 正在开发中。在 v2.0.0 发布契约冻结前，允许破坏性 API 和生成输出调整；
仓库根目录仍然是 v1 module。

需要 Go 1.25.8 或更高版本。

## 选择入口

| 目标 | 使用方式 |
| --- | --- |
| 生成完整服务项目 | `microgen` |
| 用最少装配构建小型服务 | `kit` |
| 只集成部分框架能力 | `endpoint`、`transport`、`sd`、`interaction` |

`kit` 是基于同一套 endpoint 和 transport 组件的简洁脚手架，不是另一套架构。
原生 `http.Handler` 注册仅作为静态文件、第三方 handler、探针和自定义协议的
逃生口。

## 生成服务

在当前仓库开发 v2 时安装 `microgen`：

```bash
# 在仓库根目录执行。
go -C v2 install ./cmd/microgen
```

v2.0.0 发布后：

```bash
go install github.com/dreamsxin/go-kit/v2/cmd/microgen@v2.0.0
```

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
	SayHello(context.Context, HelloRequest) (HelloResponse, error)
}
```

生成最小 HTTP 服务：

```bash
mkdir hello-svc
microgen \
  -idl idl.go \
  -out hello-svc \
  -import example.com/hello-svc \
  -config=false \
  -model=false \
  -db=false

cd hello-svc
go mod tidy
go run ./cmd
```

检查生成服务：

```bash
curl http://localhost:8080/health
curl http://localhost:8080/debug/routes
curl http://localhost:8080/skill
```

刚生成的业务方法会返回未实现错误。业务逻辑写在
`service/helloservice/service.go`。

生成配置、gRPC、数据库反向生成、interaction/MCP 和 extend 模式详见
[MICROGEN.md](MICROGEN.md)。

## 生成文件归属

生成项目明确区分用户维护文件和 `microgen` 管理文件。

可以修改：

- `service/<service>/service.go`
- `endpoint/<service>/custom_chain.go`
- `cmd/custom_routes.go`
- `config/config.yaml`

不要手动修改：

- `cmd/generated_*.go`
- `endpoint/<service>/generated_chain.go`
- `model/generated_*.go` 和 `repository/generated_*.go`
- 生成的 `client/`、`sdk/`、`skill/` 和 `pb/` 资源

扩展已有生成项目之前先执行 `microgen extend -check -out .`。

## 使用 `kit`

`kit` 是保留 endpoint middleware 和严格 HTTP transport 行为的最短使用路径：

```go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dreamsxin/go-kit/v2/kit"
)

type HelloRequest struct {
	Name string `json:"name"`
}

type HelloResponse struct {
	Message string `json:"message"`
}

func main() {
	svc, err := kit.New(":8080",
		kit.WithRequestID(),
		kit.WithTimeout(5*time.Second),
		kit.WithRateLimit(100),
	)
	if err != nil {
		log.Fatal(err)
	}

	kit.HandleJSON[HelloRequest](svc, "/hello", func(
		ctx context.Context,
		req HelloRequest,
	) (any, error) {
		return HelloResponse{Message: "Hello, " + req.Name}, nil
	})

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()
	if err := svc.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
```

`kit.New` 会校验 Option 并返回错误。`Service.Run` 跟随调用方提供的 context；
系统信号监听应放在 `main` 中。

需要 endpoint middleware 的业务路由使用 `kit.HandleJSON` 或
`kit.HandleJSONEndpoint`。`Service.Handle` 和 `Service.HandleFunc` 仅用于原生
HTTP 集成。

## 组件

| 包 | 职责 |
| --- | --- |
| `kit` | 小型服务装配和生命周期 |
| `endpoint` | 与 transport 无关的 endpoint 和 middleware 组合 |
| `transport/http` | HTTP server/client adapter |
| `transport/grpc` | gRPC server/client adapter |
| `sd` | 服务发现、endpoint 更新、负载均衡和重试执行 |
| `interaction` | tool、resource、prompt、session 和策略 hook |
| `interaction/mcp` | MCP Streamable HTTP adapter |
| `log` | 框架日志适配 |
| `cmd/microgen` | 契约驱动的项目生成器 |

包边界和扩展规则见 [ARCHITECTURE.md](ARCHITECTURE.md)。框架核心明确不包含
IAM、Outbox、任务平台、对象存储、Secret 平台和完整事务框架等业务平台能力。

## 配置

生成配置按以下顺序解析：

```text
默认值 -> 本地 YAML -> 可选远程配置 -> 最终环境变量覆盖 -> 配置校验
```

环境变量使用 `APP_` 前缀。最终配置无效时会在运行时装配前失败。从数据库生成
只读取源结构，生成服务默认不会执行 `AutoMigrate`，除非显式开启。

## 验证修改

```bash
cd v2
go test ./...
go test -race ./kit ./interaction ./sd/... ./cmd/microgen/generator
```

修改生成器后，还必须验证在仓库外生成的项目可以执行 `go mod tidy` 和
`go test ./...`。

## 文档

- [DOCS_INDEX.md](DOCS_INDEX.md)：文档导航
- [MICROGEN.md](MICROGEN.md)：生成器使用与生成文件归属
- [ARCHITECTURE.md](ARCHITECTURE.md)：包边界和扩展模型
- [PRODUCTION.md](PRODUCTION.md)：运行、安全和可观测性指导
- [MIGRATION.md](MIGRATION.md)：从 v1 迁移到 v2
- [MAINTAINING.md](MAINTAINING.md)：仓库维护和验证流程
- [examples/](examples/README.md)：可运行示例

## License

MIT，见 [LICENSE.txt](LICENSE.txt)。
