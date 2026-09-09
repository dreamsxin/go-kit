# 面向 Agent 的仓库指南

[English](AGENTS.md) | 简体中文

改动之前先读这里。它只写"把仓库读一遍也不会知道"的东西，其余各自指向真正拥有那个主题的文档，
而不是把它们抄一遍。

## 结构

- 只有一个发布的 module：`github.com/dreamsxin/go-kit/v2`，根在 `v2/`。运行时、生成器、
  provider 与适配器都从它发布。
- 只在 workspace 里、永不打 tag：`v2/tools`（门禁与发布工具）、`v2/examples`、
  `v2/tools/contractcheck`。多出第二个可发布 module 会让门禁失败。
- 门禁在自己的 module 里，所以从 `v2` 跑 `go test ./...` 跑不到它们。用
  `go -C ./v2/tools test ./...`。

## 改动的流程

1. **先读。** 除了改一行以外的任何事，都要读将被改的代码和调用它的代码。这棵树里的文档注释
   承载着推理——一处缺陷往往已经被写在它旁边了。
2. **只改让那个性质成立所需的最小部分。** 不顺手改周围的代码、不顺手改命名、不加没人要求的
   可配置项。
3. **验证**（见下）。能编译不等于改完了。
4. **记录。** 行为写进 `v2/CHANGELOG.md` 当前打开的候选小节；发现、计划、以及"有意不做"的
   决定写进 `v2/internal/docs/ROADMAP.md`。两者都有 `_zh` 那一半，只改一边就是缺陷。
5. **提交并推送**（见下）。

"排除"要被记录，而不是被跳过：当你决定不修某个已经发现的问题时，写下是什么、以及为什么。
不写在纸上，下一个读者无法把"考虑过之后的省略"和"漏掉了"区分开。

## 说"改完了"之前

在 `v2/` 下：

```bash
make verify
```

这就是 CI 跑的东西。没有 `make` 时：

```bash
go -C ./tools run ./releaseverify -root .. -suites fmt,test,standalone,vet,tidy,race
```

要在已提交的树上跑。`tidy` 这一档会 diff 工作区，所以任何未提交的改动都会让它失败——包括
你正要提交的那一份。`race` 需要 cgo，也就是 `PATH` 上要有 C 工具链。

迭代过程中用更省时间的定向目标：`make test-runtime`、`make test-microgen`、
`make test-contracts`、`make test-snapshots`、`make test-boundaries`。

## 提交信息

用带 `v2` scope 的 Conventional Commits——`fix(v2):`、`feat(v2):`、`refactor(v2):`、
`docs(v2):`、`chore(v2):`——发布出去的行为发生不兼容变化时加 `!`，在兼容性冻结之前这是允许的。

标题写"性质"，不写"机制"：`fix(v2)!: the request's Host is not a credential`，而不是
`fix(v2): change validateOrigin`。正文写清哪里错了、为什么那是错的、以及你验证了什么——
改了什么 diff 自己会说。如果这处修的是本仓库自己引入的回归，就直接写明。

把信息写进一个文件，然后 `git commit -F`。多行正文通过 `-m` 传，在 PowerShell 下会丢掉格式。

## 门禁挡住你的时候

要么改动是错的，要么门禁承诺的东西需要被有意地重新表述。为了让自己的改动通过而削弱它，
两者都不是。如果那个承诺确实变了，就在一个单独的提交里改门禁，并说清是哪个承诺变了、为什么。

你新增的门禁必须有能力失败。故意让它失败一次，读一遍你自己写的失败信息，然后撤销——一道
没人见过它失败的门禁，是一道没人知道形状的门禁。

## 快照，以及关于它们的唯一一条规则

被评审的文件在 `v2/tools/testdata/` 下，各自由 `-args` 之后的一个 flag 刷新：

- `v2/tools/testdata/api_surface.txt` — `-update-api-snapshot`
- `v2/tools/testdata/package_paths.txt` — `-update-package-paths`
- `v2/tools/testdata/protocol_behaviour.txt` — `-update-protocol-behaviour`
- `v2/tools/testdata/generated_layout.txt` — `-update-generated-layout`
- `v2/tools/testdata/contract_snapshots/` — `-update-contract-snapshots`

规则是：**刷新就是审查本身。** 它们每一个存的都是自己钉住的东西——声明、路径、承诺、目录结构、
以及生成产物本身——所以对文件做 `git diff` 就是审查，失败信息也会引用动了的那一行。这里已经不再
有 digest，也不该再出现 digest。`make update-snapshots` 会一起刷新它们；提交之前先读 diff。

## 一个承诺就是源码里的一个标记

框架承诺的行为，写在它被实现的地方：

```go
// Stable: http.sse-headers — a stream answers 200 with text/event-stream, ...
// Covered by: TestSSEServer_WritesEventsAndHeaders
```

新增一个的做法：把标记写在非测试源码里、紧挨着那个行为；点名同一个包里确实存在的测试；刷新
`protocol_behaviour.txt`；如果这个承诺属于 `v2/internal/docs/RELEASE.md` 列举的契约面之一，
就同时在那里点名它的门禁——因为有一个测试在数它们。

## 门禁强制的 import 规则

- `v2/endpoint` 只能 import 标准库。那里的契约是结构化的，不是靠命名的。
- `v2/security/http` 不能从本 module import 任何东西。
- 其余包之间的分层由 `TestArchitectureDependencyGates` 钉住。

## 写下来的东西放哪

每份文档都是一对：`X.md` 有 `X_zh.md`。行为变化记进 `v2/CHANGELOG.md`，计划与发现记进
`v2/internal/docs/ROADMAP.md`。不要为了这些文档已经拥有的内容在顶层新建 markdown 文件，
也不要写没人要求的"工作总结"文档。

## 开启一个发布候选

`v2/RELEASE_MANIFEST.json` 是唯一来源：phase 为 `candidate`、版本号、以及 tag。版本号还重复
在下面这些地方，其中任何一处不一致都会让 `TestReleaseManifestMatchesRepository` 失败：

- `v2/RELEASE_MANIFEST.json` — `coreVersion` 与 `tag`
- `v2/Makefile` — `VERSION`
- `v2/cmd/microgen/internal/generator/options.go` — `defaultGoKitVersion`
- `v2/cmd/microgen/internal/generator/generator_test.go` — 生成的 `go.mod` 的期望值
- `v2/examples/go.mod` — 对框架的 `require`
- `v2/README.md`、`v2/ARCHITECTURE.md`、`v2/internal/docs/RELEASE.md` 以及它们的 `_zh`
  那一半——"已发布 / 候选中"那句话
- `v2/CHANGELOG.md` 与 `v2/CHANGELOG_zh.md`——一个标记为候选的新标题

## 切出并记录一次发布

流程、以及哪些环节不允许绕过，在 `v2/internal/docs/RELEASE.md`。简述：

1. 候选提交上 `make verify` 全绿，Linux 与 Windows 都要。
2. `make release-check-clean`——phase 是 `candidate` 且 tag 尚不存在。
3. `git tag -a vX.Y.Z -m "go-kit vX.Y.Z"`，并推送这个 tag。
4. `make verify-published`——通过公共代理解析这个版本。不要用 `GOPROXY=direct`、本地
   `replace`、或"本地 tag 已存在"来替代它：这三者都不能说明 module 已经发布。
5. 记录：manifest 的 phase 改为 `released` 并写上日期，changelog 标题写上日期，
   `v2/internal/docs/ROADMAP.md` 里那个里程碑标记为完成。

只落在 `v2/tools` 或文档里的改动不打 tag。发布出去的 module 会逐字节相同，而一个说得不一样的
版本号，是由使用方来付账的谎。

## 设计立场

任何部署方会想定制的东西，都应该是它自己实现的一个接缝；接缝留空就意味着框架不施加任何策略。
标准库当年发布过某个错误默认值，不构成继续错下去的理由。其余在 `v2/ARCHITECTURE_zh.md`。
