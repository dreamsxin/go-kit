# go-kit

[![Go Version](https://img.shields.io/badge/go-1.25.8+-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE.txt)

[English](README.md) | 简体中文

`go-kit` 是一个组件化 Go 服务框架，围绕一条统一请求链路构建：

```text
Service -> Endpoint -> Transport
```

## 当前版本

当前维护的产品线是 [`v2/`](v2/) 目录下的独立 v2 module：

```text
github.com/dreamsxin/go-kit/v2@v2.3.0
```

`v2.3.0` 是本次从 `main` 发布的版本，完全向后兼容：全部为新增能力与
行为修复，没有新的 SemVer 例外。历史上 `v2.1.0` 的直接重构例外和
`v2.2.0` 的 `Metrics.Snapshot` 例外仍记录在[迁移指南](v2/MIGRATION.md)中。

## 开始使用

安装核心框架和生成器：

```bash
go get github.com/dreamsxin/go-kit/v2@v2.3.0
go install github.com/dreamsxin/go-kit/v2/cmd/microgen@v0.2.1
```

安装、组件选择、代码生成、示例和开发命令统一以 [v2 README](v2/README_zh.md)
为准。

## 文档

- [v2 README](v2/README_zh.md)：用户入口
- [架构说明](v2/ARCHITECTURE.md)：包边界和扩展模型
- [microgen](v2/MICROGEN.md)：生成器行为和生成文件归属
- [迁移指南](v2/MIGRATION.md)：v1 和 v2.0.0 升级说明
- [生产指南](v2/PRODUCTION.md)：运行、安全和可观测性
- [发布规范](v2/RELEASE.md)：兼容性与发布流程

## v1 归档

`main` 不再维护 v1。完整的 v1 源码和文档仍保存在不可变的历史标签中，
`v1.6.0` 是最后一个 v1 版本。现有用户可以[查看归档源码](https://github.com/dreamsxin/go-kit/tree/v1.6.0)，
也可以继续固定使用该版本：

```bash
go get github.com/dreamsxin/go-kit@v1.6.0
```

新项目应直接使用 v2。

## 开发验证

运行核心测试：

```bash
go -C v2 test ./...
```

运行多 module 维护门禁：

```bash
go -C v2/tools run ./releaseverify -root .. -suites test,standalone,vet
```

## License

MIT，见 [LICENSE.txt](LICENSE.txt)。
