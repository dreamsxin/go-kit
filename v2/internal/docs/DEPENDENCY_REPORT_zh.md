# 依赖闭包报告
[English](DEPENDENCY_REPORT.md) | 简体中文

本报告记录当前 v2 依赖闭包。可执行的规则位于 `tools/dependency_boundaries_test.go`；本文档
记录经过评审的结果，以及用于诊断的历史测量基线。

## 环境

- 当前源码：v2.8.0 发布候选工作树
- 对比基线：历史 v2 测量（仅作诊断）
- Go：`go1.25.8`
- 平台：`windows/amd64`
- CGO：已禁用
- 工作区：捕获包闭包时启用，独立 HTTP 对比时禁用

包计数包括标准库和平台特定包。非标准计数包括 go-kit 包以及提供者包。

## 核心入口

根 module 为其中发布的可选 package 声明 provider 依赖。每个核心入口仍然只解析标准库
和所列的 go-kit package；只有导入可选 package 时才会出现 provider 成本。

| 入口 | 编译包数 | go-kit 包数 | 允许的 go-kit 依赖区域 |
| --- | ---: | ---: | --- |
| `endpoint` | 65 | 1 | `endpoint` |
| `transport/http/...` | 187 | 5 | `endpoint`、`transport`、`transport/http` |
| `sd/...` | 83 | 8 | `endpoint`、`sd` |
| `kit` | 187 | 5 | `endpoint`、`transport`、`transport/http`、`kit` |
| `interaction` | 65 | 1 | `interaction` |
| `interaction/mcp` | 184 | 2 | `interaction`、`interaction/mcp` |
| `security/http` | 181 | 1 | `security/http` |
| `observability/slog` | 77 | 3 | `endpoint`、协议无关的 `transport`、`observability/slog` |

`go -C ./tools test . -run TestArchitectureDependencyGates` 拒绝这些区域之外的导入，包括提供者 SDK 和协议交叉依赖。

## 可选入口

可选集成属于同一 module 中的 provider package。只有导入相应 package，应用的依赖闭包才会包含其提供者成本。

| Package 入口 | 编译包数 | 非标准包数 | 提供者家族 |
| --- | ---: | ---: | --- |
| `integrations/consul` | 207 | 18 | HashiCorp Consul API |
| `integrations/etcd` | 353 | 154 | etcd client v3（会带入 gRPC） |
| `integrations/grpc` | 307 | 114 | gRPC 与 Protobuf |
| `integrations/zap` | 196 | 13 | Uber Zap |
| `kit/grpc` | 303 | 110 | gRPC 与 Protobuf |
| `observability/otel` | 209 | 24 | OpenTelemetry |

计数来自仓库工作区下的 `go list -deps`，因此使用的是本地核心契约。可发布模块还会在 `GOWORK=off` 下测试，且发布边界测试拒绝本地 `replace` 指令。

## 最小 HTTP 对比

两次测量都基于已发布的 kit 示例源码构建同一个功能服务。当前测量仅将 go-kit 模块替换为重构后的本地根模块。每个二进制都使用以下命令构建了两次：

```bash
GOWORK=off CGO_ENABLED=0 go build -a -trimpath
```

表格报告较短一次强制构建的耗时。耗时仅为诊断信息，不是绝对的发布阈值；依赖和二进制测量是稳定的闭包信号。

| 指标 | 历史基线 | 当前候选 | 变化 |
| --- | ---: | ---: | ---: |
| 编译包数 | 326 | 190 | 41.7% |
| 非标准包数 | 131 | 6 | 95.4% |
| 模块数（含探针模块） | 73 | 2 | 97.3% |
| 强制构建耗时 | 9.205 s | 7.013 s | 23.8% |
| Windows 二进制大小 | 18,372,096 字节 | 8,898,048 字节 | 51.6% |

仅 HTTP 探针只解析 go-kit 根 module，不会解析任何可选 provider package。仓库还保留
examples 和 tooling 两个仅供工作区使用的 module；它们不发布也不打 tag。

## 复查

在架构或模块变更之后评审本报告前，先运行维护中的门禁：

```bash
go -C ./tools test . -run TestArchitectureDependencyGates -count=1
go -C ./tools test . -run TestKitHTTPAssemblyDoesNotResolveOptionalDependencies -count=1
go -C ./tools test . -run TestPublishableModulesDoNotUseLocalReplacements -count=1
```

审慎地刷新测量值，并记录 Go 版本、平台、源码提交、构建标志和对比标签。不要把缓存的构建时间当作可比较的结果。
