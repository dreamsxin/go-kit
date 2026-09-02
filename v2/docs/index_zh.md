# go-kit v2 手册

[English](index.md) | 简体中文

本手册是各包指南的面向任务的配套读物。包指南（参考：`endpoint/README.md`、
`transport/README.md` 等）回答"这个包提供什么"，而本手册回答"如何端到端地完成 X"。

## 教程

从空目录到可运行服务的完整演练：

- [快速上手](getting-started_zh.md)：安装、第一个服务、第一个请求
- [教程：一个 CRUD 服务](tutorial-crud_zh.md)：SQLite 存储、类型化 JSON
  路由、优雅停机
- [教程：生成一个服务](tutorial-microgen_zh.md)：从 IDL 生成包含客户端、SDK
  和 OpenAPI 的完整项目
- [教程：认证中间件](tutorial-auth_zh.md)：Bearer 密钥、角色与公开路由
- [教程：一个 MCP 服务器](tutorial-mcp_zh.md)：通过 MCP Streamable HTTP 向 AI
  客户端暴露工具

## 章节

跨越多个包的横向流程：

- [核心概念](concepts_zh.md)：请求路径、错误分类、生命周期归属
- [服务发现与路由](service-discovery_zh.md)：发现快照、端点选择、反馈、健康检查、
  provider 与长连接
- [错误处理](errors_zh.md)：`apperror`、传输层映射、端到端的自定义错误格式
- [中间件](middleware_zh.md)：组合、流控与内置目录
- [生命周期](lifecycle_zh.md)：启动、优雅停机、后台任务
- [配置](configuration_zh.md)：生成的配置优先级与自定义配置段
- [测试](testing_zh.md)：测试端点与服务
- [排障](troubleshooting_zh.md)：按症状排查故障——请求关联、状态码、数据库、日志、调试开关
- [自定义](customization_zh.md)：自定义日志存储、编写自定义中间件、定制错误处理

## 组件参考

各包指南拥有对应组件的详细 API 参考：

- [endpoint](../endpoint/README_zh.md)、[transport](../transport/README_zh.md)、
  [kit](../README_zh.md#使用-kit)、[sd](../sd/README_zh.md)、
  [feedback](service-discovery_zh.md#反馈与被动摘除)、[health](service-discovery_zh.md#主动健康检查)、
  [interaction](../interaction/README_zh.md)、[security](../security/http/README_zh.md)、
  [observability](../observability/slog/README_zh.md)、
  [integrations](../integrations/consul/README_zh.md)、
  [etcd](../integrations/etcd/README_zh.md)、
  [microgen](../MICROGEN_zh.md)

## 生产环境

- [生产环境指南](../PRODUCTION_zh.md)
- [升级说明](../MIGRATION_zh.md)
