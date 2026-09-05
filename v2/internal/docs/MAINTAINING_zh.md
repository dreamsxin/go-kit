# 维护者指南
[English](MAINTAINING.md) | 简体中文

本指南定义修改 go-kit v2 的常规工作流。长期规则保存在这里，长期里程碑顺序保存在 [ROADMAP_zh.md](ROADMAP_zh.md) 中，临时计划或进度记录保存在 issue 和拉取请求中。

## 编辑之前

1. 阅读 [ARCHITECTURE_zh.md](../../ARCHITECTURE_zh.md) 了解包所有权。
2. 阅读最近的包 README 和测试。
3. 检查 `git status`，并保留工作树中不相关的变更。
4. 判断该变更影响运行时 API、生成输出，还是两者皆有。

v1 源码不在 `main` 上维护。实现 v2 变更时，不要重建它，也不要重写不可变的旧标签。

所有维护中的模块有意在其 `go.mod` 文件中声明相同的最低 Go 版本，当前为 Go 1.26.0。更改这一下限是仓库范围的兼容性决策：必须同步更新每个模块、CI 通道、README 徽章、生成项目检查和发布门禁。

## 范围规则

- 优先使用现有的包和辅助函数。
- 保持服务、endpoint、传输和装配的职责分离。
- 只有当多个应用都需要时才添加通用框架行为。
- 将特定于提供者或特定于部署的行为放入可选集成包。
- 不要向核心添加 IAM、outbox、任务平台、对象存储、密钥平台或完整的事务框架。

## 运行时变更工作流

1. 添加或更新一个专项行为测试。
2. 修改拥有该行为的包。
3. 如果推荐装配方式发生变化，则更新示例。
4. 如果公开契约发生变化，则更新包 README 和顶层文档。
5. 依次运行包测试、竞态测试，然后运行完整测试套件。

典型命令：

```bash
cd v2
go test ./kit ./endpoint/... ./transport/...
go test -race ./kit ./interaction/... ./transport/... ./sd/...
go test ./...
```

## microgen 变更工作流

生成输出是产品表面。在生成的项目通过验证之前，模板变更不算完成。

1. 在所属层修改 parser/IR/generator/template 代码。
2. 为生成契约添加单元断言。
3. 通过各自的测试重新生成被跟踪的夹具。
4. 当公开产物字节发生变化时，评审并显式刷新契约快照。
5. 将同一生成测试运行两次，并验证第二次运行没有差异。
6. 生成到模块之外的临时目录。
7. 在该项目中运行 `go mod tidy` 和 `go test ./...`。
8. 当用户工作流发生变化时，更新 [MICROGEN_zh.md](../../MICROGEN_zh.md) 和生成的 README 模板。

命令：

```bash
cd v2
go test ./cmd/microgen/...
go test ./tools -count=1
make test-contracts
make test-observability
make test-security
make test-api
go test ./...
```

有意的契约变更通过以下命令刷新快照：

```bash
go test ./tools -run "TestMicrogen(IDLContractIntegration|ProtoIntegration|FromDBIntegration)" \
  -count=1 -args -update-contract-snapshots
```

有意的导出 API 变更通过以下命令刷新经过评审的 API 快照：

```bash
go test ./tools -run TestPublicAPISurfaceSnapshot -count=1 \
  -args -update-api-snapshot
```

生成的 Go 代码必须通过 `go/format`。生成的非 Go 文本必须具有确定性的行尾、尾随空白和最终换行行为。

## 文档规则

维护的顶层文档集合有意保持精简：

- `README.md` 和 `README_zh.md`：首次成功使用；
- `MICROGEN.md`：生成器行为与所有权；
- `ARCHITECTURE.md`：包边界；
- `PRODUCTION.md`：部署指引；
- `ROADMAP.md`：权威实施顺序与里程碑验收；
- `MAINTAINING.md`：贡献者工作流；
- `RELEASE.md` 和 `CHANGELOG.md`：发布策略与历史。

更新权威文档，而不是新增第二份路线图、项目快照、设计草稿或重复索引。临时规划应写在 issue 或拉取请求中。

文档示例必须能基于当前 v2 API 编译。链接必须使用相对路径，并且必须能在区分大小写的文件系统上解析。

## 评审清单

### 行为

- 该变更解决的是一个通用框架问题。
- 错误、取消、超时和关闭路径均有覆盖。
- 可变的输入/输出在跨越并发所有权边界时会被复制。
- 没有任何库代码安装进程信号处理器或退出进程。

### API

- 由拥有该行为的包暴露该 API。
- 无效配置在启动流程能够处理的位置返回错误。
- 命名应描述实际行为；避免使用承诺了未实现的重试、流式处理或安全性的名称。
- 行为变更记录在 `CHANGELOG.md` 中。
- 冻结前的兼容性例外需要仓库所有者明确批准，并且必须仅限于变更日志、提交和发布说明中记录的确切 API。

### 生成器

- 受变更影响的 Go IDL、Protobuf 和数据库路径均有测试。
- 生成的所有权边界保持明确。
- 重复生成是确定性的。
- 外部生成的项目在没有无效本地 `replace` 的情况下能构建。
- 对源数据库的内省保持只读。

### 验证

- 专项测试通过。
- 相关竞态测试通过。
- `go test ./...` 通过。
- `git diff --check` 通过。
- 没有遗留临时生成文件。
- 文档链接可解析。

## 发布准备

1. 评审 `RELEASE_MANIFEST.json`，确认其阶段、模块路径、版本和根标签。
2. 评审导出 API 和生成输出的差异。
3. 更新 `CHANGELOG.md`、`RELEASE.md` 和 `ROADMAP.md`。
4. 提交最终发布候选。
5. 从该已提交的候选运行 `make verify`，或要求等价的 Linux/Windows CI 作业。
6. 从同一提交运行 `make release-check-clean`。
7. 确认不存在历史 v2 模块标签。

根模块使用唯一的 `vX.Y.Z` 标签。仓库内部模块永不打标签。

兼容性策略见 [RELEASE_zh.md](RELEASE_zh.md)。
