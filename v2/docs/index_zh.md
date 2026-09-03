# go-kit v2 手册

[English](index.md) | 简体中文

本手册用于端到端完成任务；需要完整 API 契约时，请直接阅读对应包的 README。

## 选择路径

| 你要做什么 | 先读 | 然后使用 |
| --- | --- | --- |
| 几分钟运行服务 | [快速上手](getting-started_zh.md) | [examples](../examples/README_zh.md) |
| 生成项目 | [microgen 教程](tutorial-microgen_zh.md) | [microgen 指南](../MICROGEN_zh.md) |
| 手工组装小型服务 | [快速上手](getting-started_zh.md) | [`kit` README](../README_zh.md#使用-kit) |
| 理解请求链路 | [核心概念](concepts_zh.md) | [中间件](middleware_zh.md)、[错误处理](errors_zh.md) |
| 接入 HTTP、gRPC、SSE 或自定义编解码 | [Transport README](../transport/README_zh.md) | [自定义传输](custom-transport_zh.md) |
| 添加服务发现、均衡、重试或健康检查 | [服务发现](service-discovery_zh.md) | [SD README](../sd/README_zh.md) |
| 添加认证或浏览器安全 | [认证教程](tutorial-auth_zh.md) | [安全 README](../security/http/README_zh.md) |
| 添加日志、指标或追踪 | [可观测性](observability_zh.md) | [生产指南](../PRODUCTION_zh.md) |
| 向 AI 客户端提供工具 | [MCP 教程](tutorial-mcp_zh.md) | [Interaction README](../interaction/README_zh.md) |
| 部署和运维 | [生产指南](../PRODUCTION_zh.md) | [生命周期](lifecycle_zh.md)、[排障](troubleshooting_zh.md) |
| 升级已有项目 | [迁移说明](../MIGRATION_zh.md) | [变更记录](../CHANGELOG_zh.md) |

## 教程

按目标选择阅读：

- [快速上手](getting-started_zh.md)：第一个 HTTP 服务和请求。
- [生成一个服务](tutorial-microgen_zh.md)：从 Go IDL 到可运行项目。
- [一个 CRUD 服务](tutorial-crud_zh.md)：存储、类型化路由和停机。
- [认证](tutorial-auth_zh.md)：Bearer 凭证与角色。
- [一个 MCP 服务器](tutorial-mcp_zh.md)：Streamable HTTP 和工具调用。

## 核心章节

- [核心概念](concepts_zh.md)：分层、所有权、上下文与边界。
- [中间件](middleware_zh.md)：组合、顺序和流控。
- [错误处理](errors_zh.md)：分类、状态映射和安全消息。
- [生命周期](lifecycle_zh.md)：启动、停机、健康检查和任务。
- [配置](configuration_zh.md)：生成配置与优先级。
- [服务发现](service-discovery_zh.md)：快照、选择、反馈与重试。
- [自定义](customization_zh.md)：自定义中间件、编解码、日志和错误。
- [自定义传输](custom-transport_zh.md)：HTTP 编解码与新协议适配器。
- [测试](testing_zh.md)：单元、HTTP、中间件和集成测试。
- [排障](troubleshooting_zh.md)：按症状定位问题。
- [可观测性](observability_zh.md)：日志、指标、追踪、关联与基数控制。

## 包参考

详细契约位于实现旁边：

- [`endpoint`](../endpoint/README_zh.md)
- [`transport`](../transport/README_zh.md)
- [`kit`](../README_zh.md#使用-kit)
- [`sd`](../sd/README_zh.md)
- [`interaction`](../interaction/README_zh.md)
- [`security/http`](../security/http/README_zh.md)
- [`observability/slog`](../observability/slog/README_zh.md)
- [`observability/otel`](../observability/otel/README_zh.md)
- [`integrations/grpc`](../integrations/grpc/README_zh.md)
- [`integrations/consul`](../integrations/consul/README_zh.md)
- [`integrations/etcd`](../integrations/etcd/README_zh.md)
- [`microgen`](../MICROGEN_zh.md)

## 发布与维护

- [生产指南](../PRODUCTION_zh.md)
- [迁移说明](../MIGRATION_zh.md)
- [架构与边界](../ARCHITECTURE_zh.md)
- [文档导航](../DOCS_INDEX_zh.md)
- [维护者指南](../internal/docs/MAINTAINING_zh.md)
- [发布指南](../internal/docs/RELEASE_zh.md)
