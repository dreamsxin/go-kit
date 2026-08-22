# go-kit v2 使用手册

[English](MANUAL.md) | 简体中文

本手册是 go-kit v2 的地图：给出穿过框架的最短路径，并把每个主题指向
[书本](docs/index_zh.md)中的详细章节或对应包指南。

## 从这里开始

大约五分钟，从零到一个可运行的服务：[快速开始](docs/getting-started_zh.md)。

## 三个不变量

**请求链路。** `Service -> Endpoint -> Transport`：业务逻辑是
`(context, Request) -> (Response, error)` 的普通函数；传输层拥有协议；
中间件组合横切关注点。详见[核心概念](docs/concepts_zh.md)。

**错误分类。** 业务错误用 `apperror`
（`github.com/dreamsxin/go-kit/v2/apperror`）分类；传输层自动把错误种类
映射为协议状态码。详见[错误处理](docs/errors_zh.md)。

**生命周期所有权。** 进程入口持有信号与根 context；服务持有监听器与优雅
停机。详见[生命周期](docs/lifecycle_zh.md)。

## 组件

沿请求链路从核心到边缘。每个组件都有带 API 参考的包指南和可运行示例。

| 组件 | 一句话说明 | 指南 | 章节 / 示例 |
| --- | --- | --- | --- |
| `endpoint` | 类型化端点与中间件目录 | [指南](endpoint/README_zh.md) | [中间件](docs/middleware_zh.md)、[examples/middleware](examples/README_zh.md) |
| `transport/http` | JSON server/client、SSE、multipart、分页、信封 | [指南](transport/README_zh.md) | [examples/envelope](examples/README_zh.md) |
| `kit` | 服务组装、健康检查、生命周期 | [指南](README_zh.md) | [生命周期](docs/lifecycle_zh.md)、[examples/quickstart](examples/README_zh.md) |
| `sd` | 服务发现、负载均衡、重试 | [指南](sd/README_zh.md) | [examples/sd](examples/README_zh.md) |
| `interaction` | AI 工具运行时与 MCP Streamable HTTP | [指南](interaction/README_zh.md) | [教程](docs/tutorial-mcp_zh.md) |
| `security/http` | CORS、CSRF、安全响应头、IP 策略 | [指南](security/http/README_zh.md) | [教程](docs/tutorial-auth_zh.md) |
| `observability` | slog 与 OpenTelemetry 适配器 | [指南](observability/slog/README_zh.md) | [生产指南](PRODUCTION_zh.md) |
| `microgen` | 项目生成，带 Go/TS SDK 与 OpenAPI | [指南](MICROGEN_zh.md) | [教程](docs/tutorial-microgen_zh.md) |

## 书本

主题章节与完整教程在 [docs/](docs/index_zh.md) 下：

- 教程：[快速开始](docs/getting-started_zh.md)、
  [CRUD 服务](docs/tutorial-crud_zh.md)、
  [生成服务](docs/tutorial-microgen_zh.md)、
  [认证](docs/tutorial-auth_zh.md)、[MCP 服务器](docs/tutorial-mcp_zh.md)
- 章节：[核心概念](docs/concepts_zh.md)、[错误处理](docs/errors_zh.md)、
  [中间件](docs/middleware_zh.md)、[生命周期](docs/lifecycle_zh.md)、
  [配置](docs/configuration_zh.md)、[测试](docs/testing_zh.md)

## 生产部署

- [生产指南](PRODUCTION_zh.md)：部署、告警、后台任务
- [升级说明](MIGRATION_zh.md)：版本间升级动作
- [变更日志](CHANGELOG_zh.md)：逐版本变更
