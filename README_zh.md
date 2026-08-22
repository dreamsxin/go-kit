# go-kit

[![Go Version](https://img.shields.io/badge/go-1.25.8+-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE.txt)

[English](README.md) | 简体中文

`go-kit` 是一个组件化 Go 服务框架，围绕一条统一请求链路构建：

```text
Service -> Endpoint -> Transport
```

只选用需要的包即可，也可以用 `microgen` 从 Go 接口、Protobuf 契约或数据库
模式生成一个完整可运行的服务。

## 快速示例

```go
svc, err := kit.New(":8080", kit.WithRequestID())
if err != nil {
	log.Fatal(err)
}
kit.HandleJSONTyped(svc, "POST /greet", func(
	_ context.Context, req GreetRequest,
) (GreetResponse, error) {
	return GreetResponse{Message: "Hello, " + req.Name + "!"}, nil
})
// 健康检查、优雅停机与严格 JSON 解码随服务自带
```

完整演练：[快速开始](v2/docs/getting-started_zh.md)。

## 组件

| 组件 | 提供的能力 | 指南 |
| --- | --- | --- |
| `endpoint` | 类型化端点、中间件（校验、超时、熔断、限流、降级、舱壁、追踪） | [指南](v2/endpoint/README_zh.md) |
| `transport/http` | JSON server/client、SSE、multipart、分页、响应信封 | [指南](v2/transport/README_zh.md) |
| `kit` | 服务组装、健康检查、生命周期 | [指南](v2/README_zh.md) |
| `sd` | 服务发现、负载均衡、重试 | [指南](v2/sd/README_zh.md) |
| `interaction` | AI 工具运行时与 MCP Streamable HTTP | [指南](v2/interaction/README_zh.md) |
| `security/http` | CORS、CSRF、安全响应头、IP 策略 | [指南](v2/security/http/README_zh.md) |
| `observability` | slog 与 OpenTelemetry 适配器 | [指南](v2/observability/slog/README_zh.md) |
| `microgen` | 项目生成，带 Go/TS SDK 与 OpenAPI | [指南](v2/MICROGEN_zh.md) |

## 文档

- [使用手册](v2/MANUAL_zh.md)：完整使用指南
- [书本](v2/docs/index_zh.md)：主题章节与完整教程（CRUD、生成、认证、MCP）
- [示例](v2/examples/README_zh.md)：每个组件的可运行服务
- [生产指南](v2/PRODUCTION_zh.md)：部署、告警、后台任务
- [升级说明](v2/MIGRATION_zh.md)、[变更日志](v2/CHANGELOG_zh.md)

## 当前版本

当前维护的产品线是 [`v2/`](v2/) 目录下的独立 v2 module：

```bash
go get github.com/dreamsxin/go-kit/v2@v2.4.3
go install github.com/dreamsxin/go-kit/v2/cmd/microgen@v0.2.5
```

`v2.4.3` 完全向后兼容：全部为新增能力与行为修复。

## 开发验证

```bash
go -C v2 test ./...
go -C v2/tools run ./releaseverify -root .. -suites test,standalone,vet
```

## License

MIT，见 [LICENSE.txt](LICENSE.txt)。
