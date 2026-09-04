# 变更日志

[English](CHANGELOG.md) | 简体中文

## [2.8.0] - 2026-09-04

这是当前 `go-kit/v2` 产品线的首次公开发布。仓库以单一 Go module 发布：

```text
github.com/dreamsxin/go-kit/v2
```

运行时 package、传输适配器、服务发现 provider、可观测性适配器和
`cmd/microgen` 统一由同一个 module 和根 tag 进行版本管理。`examples`、
`tools` 与 `tools/contractcheck` 仍是仓库内部 workspace module。

### 包含内容

- `Service -> Endpoint -> Transport` 服务架构。
- 类型化 HTTP JSON 处理器、自定义编解码、SSE、gRPC 适配器以及 Go/TypeScript 客户端。
- 传输无关的应用错误，HTTP 与 gRPC 使用一致的错误映射。
- endpoint 中间件：校验、超时、追踪、指标、恢复、限流、熔断、降级、背压、
  舱壁和重试。
- 服务发现快照、端点缓存、负载均衡、重试、健康检查、被动摘除、反馈以及长连接统计。
- 交互运行时：会话、工具、资源、提示、授权、审计钩子和 MCP Streamable HTTP。
- 配置、OpenAPI、JSON Schema、数据库脚手架和确定性的 `microgen` 扩展流程。
- 明确的生命周期所有权、有界停机、请求关联和 package 级依赖门禁。

### 发布契约

- 唯一发布 tag 是根 `v2.8.0` tag。
- 历史 v2 module tag（根标签和嵌套标签）已删除。
- 未来版本使用一个版本和一个根 tag。
- 行为与 API 变化记录在本文件中；本次首次公开基线不维护单独的迁移历史。
