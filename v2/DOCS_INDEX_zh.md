# 文档导航

[English](DOCS_INDEX.md) | 简体中文

按任务选择最短路径：[手册](docs/index_zh.md)负责端到端流程；各包 README 是
API 参考；根目录指南负责生成、架构、生产部署和升级。

## 从这里开始

| 目标 | 文档 |
| --- | --- |
| 第一个服务 | [快速上手](docs/getting-started_zh.md) |
| 生成项目 | [microgen 教程](docs/tutorial-microgen_zh.md) |
| 手工组装服务 | [根 README](README_zh.md#使用-kit) |
| 生产部署 | [生产指南](PRODUCTION_zh.md) |
| 升级项目 | [变更记录](CHANGELOG_zh.md) |

## 按任务查找

| 任务 | 文档 |
| --- | --- |
| 理解分层与所有权 | [架构](ARCHITECTURE_zh.md)、[核心概念](docs/concepts_zh.md) |
| 组合端点中间件 | [中间件](docs/middleware_zh.md)、[endpoint README](endpoint/README_zh.md) |
| 提供或调用 HTTP | [transport README](transport/README_zh.md) |
| 添加 gRPC | [transport README](transport/README_zh.md)、[gRPC README](integrations/grpc/README_zh.md) |
| 添加自定义 body 或协议 | [自定义传输](docs/custom-transport_zh.md)、[transport README](transport/README_zh.md) |
| 自定义错误和信封 | [错误处理](docs/errors_zh.md)、[自定义](docs/customization_zh.md) |
| 添加服务发现与重试 | [服务发现](docs/service-discovery_zh.md)、[SD README](sd/README_zh.md) |
| 添加认证与浏览器安全 | [认证教程](docs/tutorial-auth_zh.md)、[security/http README](security/http/README_zh.md) |
| 添加日志、指标或追踪 | [可观测性](docs/observability_zh.md) |
| 提供 MCP 工具 | [MCP 教程](docs/tutorial-mcp_zh.md)、[interaction README](interaction/README_zh.md) |
| 测试服务 | [测试](docs/testing_zh.md) |
| 排查故障 | [排障](docs/troubleshooting_zh.md) |
| 配置生成项目 | [配置](docs/configuration_zh.md)、[microgen 指南](MICROGEN_zh.md) |

## 组件参考

| 组件 | 参考 |
| --- | --- |
| 组装与生命周期 | [`kit`](README_zh.md#使用-kit)、[生命周期](docs/lifecycle_zh.md) |
| 端点与中间件 | [`endpoint`](endpoint/README_zh.md)、[中间件](docs/middleware_zh.md) |
| HTTP 与 gRPC 传输 | [`transport`](transport/README_zh.md)、[`integrations/grpc`](integrations/grpc/README_zh.md) |
| 服务发现与均衡 | [`sd`](sd/README_zh.md)、[服务发现](docs/service-discovery_zh.md) |
| interaction 与 MCP | [`interaction`](interaction/README_zh.md) |
| 安全 | [`security/http`](security/http/README_zh.md) |
| 可观测性 | [`observability/slog`](observability/slog/README_zh.md)、[`observability/otel`](observability/otel/README_zh.md)、[`integrations/zap`](integrations/zap/README_zh.md) |
| provider | [`consul`](integrations/consul/README_zh.md)、[`etcd`](integrations/etcd/README_zh.md) |
| 项目生成器 | [`microgen`](MICROGEN_zh.md) |

## 维护者

- [维护指南](internal/docs/MAINTAINING_zh.md)
- [发布流程](internal/docs/RELEASE_zh.md)
- [依赖报告](internal/docs/DEPENDENCY_REPORT_zh.md)
- [路线图](internal/docs/ROADMAP_zh.md)
- [变更记录](CHANGELOG_zh.md)

## 所有权规则

- 运行时行为：根 README、各包 README 和 `docs/` 章节。
- 生成项目行为：`cmd/microgen/templates/readme.tmpl`。
- 设计与范围：`ARCHITECTURE.md`、`PRODUCTION.md`。
- 历史：`CHANGELOG.md`。
- 英文与中文文件成对维护；公开行为或 API 变化时同步更新两种语言。
