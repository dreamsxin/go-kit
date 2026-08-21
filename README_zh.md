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
github.com/dreamsxin/go-kit/v2@v2.4.1
```

`v2.4.1` 是当前已发布版本，完全向后兼容：全部为新增能力与
行为修复。各版本变更记录在[变更日志](v2/CHANGELOG.md)；升级说明见
[升级指南](v2/MIGRATION_zh.md)。

## 开始使用

安装核心框架和生成器：

```bash
go get github.com/dreamsxin/go-kit/v2@v2.4.1
go install github.com/dreamsxin/go-kit/v2/cmd/microgen@v0.2.3
```

安装、组件选择、代码生成、示例和开发命令统一以 [v2 README](v2/README_zh.md)
为准。

## 文档

- [v2 README](v2/README_zh.md)：用户入口
- [架构说明](v2/ARCHITECTURE_zh.md)：包边界和扩展模型
- [microgen](v2/MICROGEN_zh.md)：生成器行为和生成文件归属
- [升级说明](v2/MIGRATION_zh.md)：版本间升级动作
- [生产指南](v2/PRODUCTION_zh.md)：运行、安全和可观测性
- [发布规范](v2/internal/docs/RELEASE.md)：兼容性与发布流程

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
