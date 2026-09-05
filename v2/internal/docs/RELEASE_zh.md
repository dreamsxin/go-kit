# v2 发布策略
[English](RELEASE.md) | 简体中文

## 当前状态

v2.8.1 是本次从 `main` 发布的模块版本：

```text
github.com/dreamsxin/go-kit/v2
```

运行时、生成器、provider 与适配器一起发布：一行 `require`、一个 tag，框架与插在它
上面的东西之间不存在版本错配。`v2.8.0` 是建立这个单一已发布 module 的版本。本次的
行为变更记录在 [CHANGELOG_zh.md](../../CHANGELOG_zh.md)。

已发布模块存储在仓库的 `v2` 主版本子目录中，但使用方请求的是正常的模块版本，例如
`v2.8.1`。每次发布对应一个普通根 tag，形如 `vX.Y.Z`；已发布的 tag 不可变，随着版本
推进逐个累积。

## 版本策略

v2 处于冻结前阶段。在宣布冻结之前，minor 版本允许改变行为或移除 API；每一处这样的
变更都记录在 `CHANGELOG.md` 对应版本下。

- patch：修复与文档修正；
- minor：新增能力，以及冻结前的行为或 API 变更；
- major：留给冻结之后。

不再维护逐版本升级指南。行为和 API 变更记录在引入它们的版本变更日志中；使用方应固定
到已经验证过的版本。

冻结宣布之后，不兼容变更需要新的主版本模块路径。

届时的兼容性契约覆盖：

- 导出的运行时 API；
- 模块和包路径；
- 已记录的 `microgen` 标志；
- 生成的用户所有文件位置；
- 已记录的生成配置键及其优先级；
- 记录为稳定的协议行为。

`cmd/microgen` 下的模板和包是内部实现细节，但它们生成的公开行为属于产品表面。

## 发布准入条件

v2.8.1 候选版本满足以下条件：

- `kit`、endpoint、HTTP/gRPC 传输、服务发现和交互生命周期都具有明确的错误与取消契约。
- 生成的项目使用 `/v2` 模块，并能在框架仓库之外构建。
- Go IDL、Protobuf、数据库、config、extend 和交互生成路径都具有确定性集成测试。
- 生成的配置在运行时装配之前完成校验。
- 可选的 `slog` 和 OpenTelemetry 适配器通过各自的专项包测试；核心 package 依赖门禁仍确保
  仅 HTTP 构建不会解析它们。
- 可选的 HTTP 安全中间件在构造时校验策略，并有针对可信代理、IP、CORS、CSRF、头部和流边界的专项测试。
- 数据库内省为只读，启动迁移为可选项。
- HTTP/MCP 限制、协议检查、流超时和并发行为均有测试覆盖。
- README 快速开始和发布示例可基于发布 API 编译。
- `go test ./...` 和定向竞态测试套件在干净检出上通过。
- `CHANGELOG.md` 仅包含 v2 历史，且没有未解决的发布阻塞项。

## 发布验证

安装带 `npx` 的 Node.js，然后在 `v2` 目录下运行：

```bash
make verify
```

发布目标包括常规的 Go 验证，以及生成的 OpenAPI 3.1 解析、JSON Schema 2020-12 编译、TypeScript SDK 类型检查、跨 SDK HTTP 行为检查，和针对 Go IDL、Protobuf 与数据库源模式的确定性契约快照。它还验证经过评审的公开 API 快照、文档链接与 UTF-8、专项竞态测试、`go vet`，以及根模块和仓库内部模块的 tidy 文件。

提交发布候选之后，运行：

```bash
make release-check-clean
```

它会检查已提交的 v2 范围和清单阶段，同时不会拒绝仓库根目录正在进行的不相关工作。

等价的专项 Go 命令是：

```bash
go test ./...
go test -race ./endpoint ./kit ./interaction/... ./transport/... ./sd/...
go test -race ./cmd/microgen/internal/generator
go vet ./...
```

为每个受影响的源模式生成外部冒烟项目并运行：

```bash
go mod tidy
go test ./...
```

还需验证：

- `make test-contracts` 使用固定的 TypeScript 编译器通过；
- `make test-observability` 对标准库和 OpenTelemetry 适配器通过；
- `make test-security` 对可选的面向浏览器的 HTTP 中间件通过；
- `make test-api`、`make test-boundaries`、`make test-race`、`make test-vet` 和 `make test-modules` 通过；
- Go 与 TypeScript SDK 与共享的路径/查询/请求体/错误夹具一致；
- 契约快照变更已经过评审并显式刷新；
- 重复生成不会产生第二次运行的差异；
- `git diff --check` 通过；
- 文档链接可解析；
- 没有遗留临时生成文件；
- 已批准的标签指向包含匹配主版本模块路径的已验证提交；
- 推送根标签之后，已发布检查能通过 `proxy.golang.org` 解析
  `github.com/dreamsxin/go-kit/v2`，无需本地 `replace`，且不存在历史 v2 标签。

有意的导出 API 变更必须在刷新 API 快照之前经过评审：

```bash
go -C ./tools test . -run TestPublicAPISurfaceSnapshot -count=1 \
  -args -update-api-snapshot
```

## 发布流程

`RELEASE_MANIFEST.json` 是唯一事实来源。v2 以单一模块发布，因此一次发布只有一个版本、
一个标签：

- 运行时、生成器、provider 与适配器：`github.com/dreamsxin/go-kit/v2`；
- `examples`、`tools`、`tools/contractcheck`：仅存在于工作区，永不打标签。

只有一个发布阶段。`TestOnlyOneModuleIsPublishable` 与历史标签检查会在出现新的发布
module 或旧的 v2 标签时失败。

### 打这次发布

1. 要求候选的 Linux 和 Windows `v2 verify` 作业成功。
2. 从候选提交运行 `make release-check-clean`。此时清单阶段为 `candidate`，检查要求标签
   尚不存在。
3. 创建并推送标签：

```bash
git tag -a v2.8.1 -m "go-kit v2.8.1"
git push origin v2.8.1
make verify-published
```

如果公共代理尚未传播新标签，请等待并重新运行已发布检查。不要用 `GOPROXY=direct` 或本地 `replace` 绕过它。

### 记录这次发布

将清单阶段改为 `released`，填上 `releaseDate`，用同一个日期替换变更日志中的候选标记，
然后提交。在此阶段，`make release-check-clean` 要求标签已存在。

## 发布说明

发布说明应描述用户可见的行为和已知限制。内部重构细节应写入提交或拉取请求，除非它们能解释一个可观察到的变更。
