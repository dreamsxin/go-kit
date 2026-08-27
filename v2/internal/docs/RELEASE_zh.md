# v2 发布策略
[English](RELEASE.md) | 简体中文

## 当前状态

v2.6.0 是本次从 `main` 为以下独立模块发布的版本：

```text
github.com/dreamsxin/go-kit/v2
```

`v2.6.0` 是经批准的架构进化版本：装配层（`kit.Service` 拆分为 `kit.Host` +
`kit.HTTP`）、错误模型（移除 `server.HTTPError`）与生成的自定义路由钩子发生了经明确批准的不兼容变更。嵌套模块发布 `v0.3.0` 标签；依赖核心的模块要求 `v2.6.0`。早期版本的历史记录（包括已记录的 SemVer 例外）见 [CHANGELOG_zh.md](../../CHANGELOG_zh.md)。

已发布模块存储在仓库的 `v2` 主版本子目录中，但使用方请求的是正常的模块版本，例如 `v2.4.0`。它的标签是根标签 `v2.4.0`，而不是 `v2/v2.4.0`。未来的 `/v3` 模块同样会使用根 `v3.0.0` 标签。

本次发布推送了根标签 `v2.4.0`，并为全部八个独立版本化的嵌套模块推送 `v0.2.2` 标签。`v0.2.2` 发布携带核心依赖升级到 `v2.4.0`：标签从嵌套候选提交创建，其中每个核心依赖模块都已要求 `v2.4.0`。已发布的 `v0.2.1` 可选模块仍是保留 `v2.2.0` 依赖的版本提升发布（保持兼容）。

## 版本策略

v2 遵循语义化版本控制，仅有经明确批准并记录在案的例外：

- patch：兼容的修复和文档修正；
- minor：向后兼容的公开能力；
- major：不兼容的运行时 API、模块、CLI、配置或生成物所有权变更。

已批准的例外仅限于：

- `v2.1.0` 交付的直接重构；
- `v2.2.0` 的 `Metrics.Snapshot() -> MetricsSnapshot` 返回类型修正；
- `v2.6.0` 的架构进化（装配层拆分为 `kit.Host` + `kit.HTTP`、移除
  `server.HTTPError`、移除 `log` 门面、SSE 下沉传输层、生成自定义路由钩子标准化库化）。

任何例外都不授权无关的破坏性变更。任何进一步的不兼容性都需要新的主版本模块路径，除非在实现之前另行批准并记录。

兼容性契约包括：

- 导出的运行时 API；
- 模块和包路径；
- 已记录的 `microgen` 标志；
- 生成的用户所有文件位置；
- 已记录的生成配置键及其优先级；
- 记录为稳定的协议行为。

`cmd/microgen` 下的模板和包是内部实现细节，但它们生成的公开行为属于产品表面。

## v2.0.0 准入条件（已完成）

v2.0.0 发布提交满足以下条件：

- `kit`、endpoint、HTTP/gRPC 传输、服务发现和交互生命周期都具有明确的错误与取消契约。
- 生成的项目使用 `/v2` 模块，并能在框架仓库之外构建。
- Go IDL、Protobuf、数据库、config、extend 和交互生成路径都具有确定性集成测试。
- 生成的配置在运行时装配之前完成校验。
- 可选的 `slog` 和 OpenTelemetry 适配器通过各自的专项包测试，且不给主模块添加直接的适配器依赖。
- 可选的 HTTP 安全中间件在构造时校验策略，并有针对可信代理、IP、CORS、CSRF、头部和流边界的专项测试。
- 数据库内省为只读，启动迁移为可选项。
- HTTP/MCP 限制、协议检查、流超时和并发行为均有测试覆盖。
- README 快速开始和迁移示例可基于发布 API 编译。
- `go test ./...` 和定向竞态测试套件在干净检出上通过。
- `CHANGELOG.md` 仅包含 v2 历史，且没有未解决的发布阻塞项。

## 发布验证

安装带 `npx` 的 Node.js，然后在 `v2` 目录下运行：

```bash
make verify-release
```

发布目标包括常规的 Go 验证，以及生成的 OpenAPI 3.1 解析、JSON Schema 2020-12 编译、TypeScript SDK 类型检查、跨 SDK HTTP 行为检查，和针对 Go IDL、Protobuf 与数据库源模式的确定性契约快照。它还验证经过评审的公开 API 快照、文档链接与 UTF-8、专项竞态测试、`go vet`，以及主模块和各嵌套模块的 tidy 模块文件。

提交发布候选之后，运行：

```bash
make release-check-clean
```

它会检查已提交的 v2 范围和清单阶段，同时不会拒绝仓库根目录正在进行的不相关工作。

等价的专项 Go 命令是：

```bash
go test ./...
go test -race ./endpoint ./kit ./interaction/... ./transport/... ./sd/...
go -C ./cmd/microgen test -race ./internal/generator
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
- 推送标签之后，清单驱动的已发布检查能通过 `proxy.golang.org` 解析每个模块，无需本地 `replace`。

有意的导出 API 变更必须在刷新 API 快照之前经过评审：

```bash
go -C ./tools test . -run TestPublicAPISurfaceSnapshot -count=1 \
  -args -update-api-snapshot
```

## v2.4.2 多模块发布

`RELEASE_MANIFEST.json` 是唯一事实来源。根模块和嵌套模块不共享同一个模块版本或同一个标签：

- 根运行时：`github.com/dreamsxin/go-kit/v2@v2.4.2`；
- microgen 和可选嵌套模块：独立的 `v0.2.4` 发布（`microgen` 为失效的 `-add-tables` 标志修复；可选模块为核心依赖更新到 `v2.3.0`）；
- 示例和仓库工具：不作为产品模块发布。

发布有意分阶段进行，因为在根标签可通过 Go 模块解析获取之前，嵌套模块无法要求新的核心版本。

### 阶段 1：核心候选

清单阶段为 `core-candidate`。所有候选标签必须不存在。依赖核心的嵌套模块暂时保留上一次发布的核心依赖，同时由工作区测试针对本地变更运行它们。

1. 要求候选的 Linux 和 Windows `v2 verify` 作业成功。
2. 从候选提交运行 `make release-check-clean`。
3. 仅创建并推送根标签：

```bash
git tag -a v2.4.2 -m "go-kit v2.4.2"
git push origin v2.4.2
make verify-published-core
```

### 阶段 2：嵌套候选

在根模块公开可解析之后：

1. 将清单阶段改为 `nested-candidate`。
2. 将每个 `dependsOnCore` 模块的依赖从 `v2.2.0` 改为 `v2.3.0`。
3. 在每个嵌套模块中运行 `go mod tidy` 和 `GOWORK=off go test ./...`。
4. 提交并重新运行完整的 Linux/Windows 发布工作流。
5. 运行 `make release-check-clean`；此时它要求根标签存在，并拒绝任何已存在的嵌套标签。
6. 从该已验证提交创建清单标签：

```bash
git tag -a v2/cmd/microgen/v0.2.4 -m "microgen v0.2.1"
git tag -a v2/integrations/circuitbreaker/v0.2.4 -m "circuitbreaker v0.2.1"
git tag -a v2/integrations/consul/v0.2.4 -m "consul integration v0.2.1"
git tag -a v2/integrations/grpc/v0.2.4 -m "gRPC integration v0.2.1"
git tag -a v2/integrations/ratelimit/v0.2.4 -m "rate-limit integration v0.2.1"
git tag -a v2/integrations/zap/v0.2.4 -m "Zap integration v0.2.1"
git tag -a v2/kit/grpc/v0.2.4 -m "kit gRPC component v0.2.1"
git tag -a v2/observability/otel/v0.2.4 -m "OpenTelemetry integration v0.2.1"
git push origin \
  v2/cmd/microgen/v0.2.4 \
  v2/integrations/circuitbreaker/v0.2.4 \
  v2/integrations/consul/v0.2.4 \
  v2/integrations/grpc/v0.2.4 \
  v2/integrations/ratelimit/v0.2.4 \
  v2/integrations/zap/v0.2.4 \
  v2/kit/grpc/v0.2.4 \
  v2/observability/otel/v0.2.4
make verify-published
```

如果公共代理尚未传播新标签，请等待并重新运行已发布检查。不要用 `GOPROXY=direct` 或本地 `replace` 绕过它。

### 阶段 3：发布记录

在每个模块都公开可解析之后，将清单阶段改为 `released`，用发布日期替换变更日志中的候选标记，并提交最终发布记录。在此阶段，`make release-check-clean` 要求每个清单标签都存在。

## 发布说明

发布说明应描述用户可见的行为、迁移操作和已知限制。内部重构细节应写入提交或拉取请求，除非它们能解释一个可观察到的变更。
