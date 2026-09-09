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

## 快照，以及关于它们的唯一一条规则

被评审的文件在 `v2/tools/testdata/` 下，各自由 `-args` 之后的一个 flag 刷新：

- `v2/tools/testdata/api_surface.txt` — `-update-api-snapshot`
- `v2/tools/testdata/package_paths.txt` — `-update-package-paths`
- `v2/tools/testdata/protocol_behaviour.txt` — `-update-protocol-behaviour`
- `v2/tools/testdata/generated_layout.txt` — `-update-generated-layout`
- `v2/tools/testdata/contract_snapshots/` — `-update-contract-snapshots`

规则是：**刷新就是审查本身。** 它们每一个存的都是自己钉住的东西——声明、路径、承诺、目录结构、
以及生成产物本身——所以对文件做 `git diff` 就是审查，失败信息也会引用动了的那一行。这里已经不再
有 digest，也不该再出现 digest。另外，永远不要为了让自己的改动通过而削弱一道门禁；门禁挡住你时，
要么改动是错的，要么就得有意地重新表述它承诺的东西。

## 一个承诺就是源码里的一个标记

框架承诺的行为，写在它被实现的地方：

```go
// Stable: http.sse-headers — a stream answers 200 with text/event-stream, ...
// Covered by: TestSSEServer_WritesEventsAndHeaders
```

标记必须在非测试源码里；它点名的每个测试都必须存在于同一个包；新增一个标记就要刷新
`protocol_behaviour.txt`。这三件事各有一道门禁盯着。

## 门禁强制的 import 规则

- `v2/endpoint` 只能 import 标准库。那里的契约是结构化的，不是靠命名的。
- `v2/security/http` 不能从本 module import 任何东西。
- 其余包之间的分层由 `TestArchitectureDependencyGates` 钉住。

## 写下来的东西放哪

每份文档都是一对：`X.md` 有 `X_zh.md`，只改一边就是缺陷。行为变化记进 `v2/CHANGELOG.md`，
计划与审计发现记进 `v2/internal/docs/ROADMAP.md`，两者都连带各自的 `_zh`。不要为了这些文档
已经拥有的内容，在顶层新建一个 markdown 文件。

## 发布

`v2/RELEASE_MANIFEST.json` 是版本与阶段的唯一来源。流程，以及哪些环节不允许绕过，在
`v2/internal/docs/RELEASE.md`。

## 设计立场

任何部署方会想定制的东西，都应该是它自己实现的一个接缝；接缝留空就意味着框架不施加任何策略。
标准库当年发布过某个错误默认值，不构成继续错下去的理由。其余在 `v2/ARCHITECTURE_zh.md`。
