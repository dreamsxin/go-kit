# 文档导航 / Documentation

[English](DOCS_INDEX.md) | 简体中文

v2 文档按任务组织。当前行为写入使用与架构文档；长期实施顺序只写入
`ROADMAP.md`，临时计划和会话快照不进入长期维护文档。

**从这里开始**：[书本](docs/index_zh.md)是完整指南——主题章节与完整教程，按「快速入门 -> 核心概念 -> 组件 -> 生产部署」组织。下面的表格是同一套文档的任务索引。

## 快速入门

大约十五分钟，从零到一个可运行的服务：

| 步骤 | 目标 | 从这里开始 |
| --- | --- | --- |
| 1 | 安装并生成一个可直接运行的服务 | [README：生成服务](README_zh.md) |
| 2 | 理解生成了什么、哪些文件归你所有 | [MICROGEN.md](MICROGEN_zh.md) |
| 3 | 在可运行的示例上走通整条请求链路 | [examples](examples/README_zh.md) |
| 4 | 手工组装一个小型服务 | [README：使用 kit 构建](README_zh.md) |

## 组件导览

按顺序阅读，沿着请求链路从核心到边缘。每份指南自成一体，只依赖上面的
快速入门步骤；最后一列指向该组件的可运行示例。

| 顺序 | 组件 | 指南 | 可运行示例 |
| --- | --- | --- | --- |
| 1 | 端点、类型化端点与中间件组合 | [endpoint](endpoint/README_zh.md) | [examples/middleware](examples/README_zh.md) |
| 2 | HTTP 传输：server、client、SSE、multipart、分页、链路传播 | [transport](transport/README_zh.md) | [examples/quickstart](examples/README_zh.md) |
| 3 | 服务组装：`kit`、健康检查、生命周期、Server-Sent Events | [README：使用 kit 构建](README_zh.md) | [examples/quickstart](examples/README_zh.md) |
| 4 | 稳定性：校验、超时、降级兜底、舱壁隔离、背压 | [endpoint：内置中间件](endpoint/README_zh.md) | [examples/best_practice](examples/README_zh.md) |
| 5 | 响应组装：传输边界的信封与错误格式 | [transport：组合与嵌套](transport/README_zh.md) | [examples/envelope](examples/README_zh.md) |
| 6 | 服务发现、负载均衡与重试 | [服务发现](sd/README_zh.md) | [examples/sd](examples/README_zh.md) |
| 7 | HTTP 安全：CORS、CSRF、安全响应头、IP 策略 | [security/http](security/http/README_zh.md) | [examples/auth](examples/README_zh.md) |
| 8 | 可观测性：slog、Zap、OpenTelemetry 适配器 | [slog](observability/slog/README_zh.md)、[otel](observability/otel/README_zh.md)、[zap](integrations/zap/README_zh.md) | [PRODUCTION.md](PRODUCTION_zh.md) |
| 9 | AI 交互运行时与 MCP Streamable HTTP | [interaction](interaction/README_zh.md) | [examples/mcp_basic](examples/README_zh.md) |
| 10 | 可选集成：Consul、gRPC（熔断与限流已内置于 `endpoint`） | [consul](integrations/consul/README_zh.md)、[transport gRPC](transport/README_zh.md) | [examples/sd](examples/README_zh.md) |

## 按任务查找

| 任务 | 文档 |
| --- | --- |
| 准备服务的生产部署 | [PRODUCTION.md](PRODUCTION_zh.md) |
| 查看版本间升级动作 | [MIGRATION.md](MIGRATION_zh.md) |
| 深入生成器、extend 模式与契约 | [MICROGEN.md](MICROGEN_zh.md) |
| 理解包边界与扩展规则 | [ARCHITECTURE.md](ARCHITECTURE_zh.md) |
| 查看已发布的变更 | [CHANGELOG.md](CHANGELOG_zh.md) |

## 面向维护者

| 任务 | 文档 |
| --- | --- |
| 修改或发布仓库 | [internal/docs/MAINTAINING.md](internal/docs/MAINTAINING_zh.md)、[internal/docs/RELEASE.md](internal/docs/RELEASE_zh.md)、[RELEASE_MANIFEST.json](RELEASE_MANIFEST.json) |
| 查看实施顺序 | [internal/docs/ROADMAP.md](internal/docs/ROADMAP_zh.md) |
| 查看依赖闭包 | [internal/docs/DEPENDENCY_REPORT.md](internal/docs/DEPENDENCY_REPORT_zh.md) |
| 运行验证工具 | [tools](tools/README_zh.md) |

## 文档所有权

- 面向用户的行为：`README*`、`MICROGEN.md`、各包指南。
- 设计与范围：`ARCHITECTURE.md`、`internal/docs/DEPENDENCY_REPORT.md`、`PRODUCTION.md`。
- 产品实施顺序：`internal/docs/ROADMAP.md`。
- 贡献者流程：`internal/docs/MAINTAINING.md`、`internal/docs/RELEASE.md`。
- 版本历史：`CHANGELOG.md`、`MIGRATION.md`。
- 生成项目的文档由 `cmd/microgen/templates/readme.tmpl` 拥有。
- 每份长期维护文档都有英文版和对应的 `_zh.md` 中文版，两者同步更新。

当行为变化时，更新最近的权威文档。不要添加第二份路线图、设计草稿或
状态快照。
