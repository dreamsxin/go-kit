# tools（工具集）

[English](README.md) | 简体中文

面向 v2 框架和 `microgen` 的集成与文档探针。

## 这里有什么

- `integration_test.go`：共享的进程与示例冒烟测试辅助工具。
- `microgen_*_test.go`：CLI、生成、运行时、配置、扩展、Proto 以及数据库的集成测试。
- `readme_quickstart_test.go`：生成 README 的工作流检查。
- `documentation_links_test.go`、`documentation_api_test.go`：markdown 链接与锚点解析，以及文档中提到的每个框架符号是否真实存在。
- `testdata/`：生成项目的测试夹具与源码契约。

## 运行测试

在 v2 模块下：

```bash
# 全部集成测试。
go test ./tools -count=1

# CLI 与生成项目流程。
go test ./tools -run 'TestMicrogen' -count=1

```

完整测试套件可能会启动本地 HTTP/gRPC 服务器、生成临时项目、运行 `go mod tidy`、编译生成的命令，并在 `protoc` 可用时使用它。

## 生成器覆盖范围

tools 测试套件覆盖：

- Go IDL 的默认、最小化、带前缀以及组件化流程；
- Protobuf HTTP/gRPC 生成与流式契约；
- SQLite 数据库内省（introspection）与可运行的输出；
- 本地、混合以及严格远程配置；
- append-service、append-model、中间件以及只读 extend 检查；
- 生成的客户端、SDK、OpenAPI/JSON Schema 契约以及交互适配器；
- 重复生成的所有权与确定性。

`testdata/` 下被跟踪的目录是期望输出的测试夹具。只能通过拥有它们的生成测试来更新这些夹具，并验证第二次运行不会产生新的差异。
