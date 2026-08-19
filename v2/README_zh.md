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

`v2.3.0` 是本次从 `main` 发布的版本，完全向后兼容：全部为新增能力与
行为修复，没有新的 SemVer 例外。历史上 `v2.1.0` 的直接重构例外与
`v2.2.0` 的 `Metrics.Snapshot` 例外仍记录在
[MIGRATION.md](MIGRATION.md) 中。`main` 不再维护 v1，其源码
仍可通过不可变的 `v1.0.0` 至 `v1.6.0` 标签获取。

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

从独立版本 module 安装重构版生成器：

```bash
go install github.com/dreamsxin/go-kit/v2/cmd/microgen@v0.2.1
```

生成器 CLI（包括 SQLite 结构读取）安装和运行均不依赖 CGO 或本地 C 编译器。
只有在生成应用显式选择相应运行时数据库 adapter 时，应用本身才可能需要 CGO。

`v2.0.0` 根 module 仍包含历史生成器，可以显式安装，但它生成的包结构遵循旧契约：

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
  -protocols http

cd hello-svc
go mod tidy
go run ./cmd
```

`go run ./cmd` 会阻塞并持续提供服务，直到被中断。保持服务运行，
在另一个终端检查生成服务：

```bash
cat .microgen/manifest.json
curl http://localhost:8080/health
```

需要 `openapi.json`、`schema.json` 和内嵌 Swagger UI 时显式增加 `-openapi`。
`/debug/routes` 仅在配置模式中设置 `debug.routes_enabled: true` 后提供。

启用 `-openapi` 后，`microgen` 会从路由、客户端、SDK 和可选 MCP tool 共用的统一
IR 直接生成 OpenAPI 3.1，并在 `docs/schema.json` 和 `GET /schema.json`
提供独立 JSON Schema 2020-12 bundle，同时在 `sdk/typescript/` 生成零运行时
依赖的 Fetch client。Swagger UI 位于 `/swagger/`，其
Swagger UI 5 静态资源嵌入生成二进制，不依赖 CDN。它只是
`/openapi.json` 的查看器，不是第二份契约来源。

仓库文本文件和生成 JSON 统一使用无 BOM 的 UTF-8。仓库编码测试会在发布前拒绝
无效 UTF-8 和 Unicode 替换字符。

使用发布流程固定的编译器检查生成的 TypeScript 源码：

```bash
npx --yes --package typescript@7.0.2 tsc -p sdk/typescript/tsconfig.json
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
- `config/custom.go`

不要手动修改：

- `.microgen/manifest.json`
- `cmd/generated_*.go`
- `endpoint/<service>/generated_chain.go`
- `model/generated_*.go` 和 `repository/generated_*.go`
- 生成的 `client/`、`sdk/`、`pb/` 和 `docs/` 资源

版本化 manifest 会记录生成源、模块路径、能力、路由前缀、服务、模型、生成
middleware 和生成器归属文件。扩展项目之前先执行
`microgen extend -check -out .`；它会报告文件漂移，漂移未处理前 extend 会拒绝
写入。

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
	)
	if err != nil {
		log.Fatal(err)
	}

	kit.HandleJSONTyped(svc, "/hello", func(
		ctx context.Context,
		req HelloRequest,
	) (HelloResponse, error) {
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

带第三方依赖的 endpoint middleware 由应用显式装配：

```go
limiter := rate.NewLimiter(100, 100)
svc, err := kit.New(":8080", kit.WithEndpointMiddleware(
	ratelimit.NewErroringLimiter(limiter),
))
```

可选服务器实现 `kit.Lifecycle`。gRPC adapter 通过组件挂载，不会让纯 HTTP
应用解析 gRPC 依赖：

```go
grpcComponent, err := kitgrpc.New(":8081")
if err != nil {
	return err
}
pb.RegisterGreeterServer(grpcComponent.Server(), greeter)
svc, err := kit.New(":8080", kit.WithLifecycle(grpcComponent))
```

默认 HTTP server 使用 5 秒 Header 读取超时、1 MiB Header 上限，并保持
`WriteTimeout=0`，避免 SSE 和其他流式响应被意外中断。需要不同策略时使用
`kit.WithHTTPServerConfig` 显式覆盖完整配置。

请求和响应都有具体类型时使用 `kit.HandleJSONTyped`；有意返回动态响应时使用
`kit.HandleJSON`；已有 endpoint 时使用 `kit.HandleJSONEndpoint`。
`Service.Handle` 和 `Service.HandleFunc` 仅用于原生 HTTP 集成。

`endpoint.Metrics` 是 middleware 写入的可变采集器。并发读取必须通过
`Snapshot()` 获取可复制的 `endpoint.MetricsSnapshot`，平均耗时使用
`AverageDuration()`。

## 组件

| 包 | 职责 |
| --- | --- |
| `kit` | 小型服务装配和生命周期 |
| `apperror` | 与 transport 无关的应用错误分类 |
| `endpoint` | 与 transport 无关的 endpoint 和 middleware 组合 |
| `transport/http` | HTTP server/client adapter |
| `integrations/grpc` | 可选的 gRPC server/client adapter |
| `integrations/consul` | 可选的 Consul 服务发现 provider |
| `sd` | 与 provider 无关的服务发现契约 |
| `sd/endpointer`、`sd/balancer`、`sd/retry` | 可独立组合的服务发现运行时组件 |
| `sd/client` | 可选的发现、负载均衡和重试装配入口 |
| `interaction` | tool、resource、prompt、session 和策略 hook |
| `interaction/mcp` | MCP Streamable HTTP adapter |
| `log` | 已弃用的标准库日志兼容层 |
| `observability/slog` | 可选的标准库 `slog` endpoint 日志适配器 |
| `integrations/zap` | 可选的 Zap endpoint 日志适配器 |
| `observability/otel` | 可选的 OpenTelemetry endpoint 追踪和指标模块 |
| `security/http` | 可选的可信代理/IP、CORS、CSRF 和安全 Header |
| `cmd/microgen` | 契约驱动的项目生成器 |

`sd/client` 构造函数会同时返回可调用 endpoint 和资源 closer。调用方必须处理构造
错误，并在停止底层 instancer 之前关闭 endpoint 资源。Consul 注册和注销会返回
错误，`Instancer.Stop` 会取消并等待正在执行的阻塞查询退出。

MCP 客户端必须使用协议版本 `2025-06-18` 初始化，随后发送
`notifications/initialized`；只有声明 `sampling` capability 后，服务端才能发起
采样请求。带 `Origin` 的浏览器请求只允许同源或
`StreamableHandler.AllowedOrigins` 中显式允许的来源。

包边界和扩展规则见 [ARCHITECTURE.md](ARCHITECTURE.md)。框架核心明确不包含
IAM、Outbox、任务平台、对象存储、Secret 平台和完整事务框架等业务平台能力。

可选观测适配器将 provider 的创建、资源、导出器、采样和关闭责任保留在
应用装配层。`observability/slog` 只使用标准库，`integrations/zap` 负责当前
Zap 集成，`observability/otel` 是独立 module；核心 `endpoint` 不导入这些
provider。可以使用以下命令验证观测适配器：

```bash
make test-observability
```

面向浏览器的服务可以组合 [`security/http`](security/http/README.md) 中的
标准库 middleware。配置在应用装配阶段校验，`kit.WithHTTPMiddleware` 可以将
策略安装到服务全部路由；认证和授权仍由应用负责。使用 `make test-security`
验证该包。

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
go test -race ./kit ./interaction/... ./transport/... ./sd/...
go -C ./cmd/microgen test -race ./internal/generator
```

修改生成器后，还必须验证在仓库外生成的项目可以执行 `go mod tidy` 和
`go test ./...`。

发布前契约校验需要 Node.js 和 `npx`。该流程会验证 OpenAPI/JSON Schema、
固定版本 TypeScript 编译、Go/TypeScript SDK HTTP 行为一致性，以及生成契约
确定性快照：

```bash
make verify-release
```

提交发布候选版本后，检查 v2 范围没有未提交修改：

```bash
make release-check-clean
```

## 文档

- [DOCS_INDEX.md](DOCS_INDEX.md)：文档导航
- [MICROGEN.md](MICROGEN.md)：生成器使用与生成文件归属
- [ARCHITECTURE.md](ARCHITECTURE.md)：包边界和扩展模型
- [ROADMAP.md](ROADMAP.md)：v2 唯一实施路线图
- [PRODUCTION.md](PRODUCTION.md)：运行、安全和可观测性指导
- [MIGRATION.md](MIGRATION.md)：从 v1 迁移到 v2
- [MAINTAINING.md](MAINTAINING.md)：仓库维护和验证流程
- [examples/](examples/README.md)：可运行示例

## License

MIT，见 [LICENSE.txt](LICENSE.txt)。
