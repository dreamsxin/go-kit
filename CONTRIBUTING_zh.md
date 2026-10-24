# 为 go-kit 做贡献

[English](CONTRIBUTING.md) | 简体中文

感谢你愿意看进内部来。本框架用一套不同寻常的、机器校验的承诺来维护；知道护栏在哪，
贡献会落地得更快。仓库根目录的 `AGENTS.md` 是完整指南，一个工具或一个人一次就能读完；
这里是给首次贡献者的短版。

## 写代码之前

- 读你的改动涉及的那章手册或包 README。树里的 doc 注释带着推理——一个缺陷通常
  已经写在拥有它的那个东西旁边。
- 查 `v2/internal/docs/ROADMAP.md`。如果你的想法被记成了一条有意的不决策，那问题
  就不是"没人想过"，而是"为什么不做"——说出什么变了。

## 变更环

1. 从 `main` 拉分支。
2. 改动让性质成立的最小一份。
3. 在 `v2/` 下验证：`make verify` 会跑 CI 跑的一切（需要已提交的工作树）。
   迭代过程中 `make test-runtime`、`make test-microgen`、`make test-contracts`
   更省时间。
4. 行为变更记入 `v2/CHANGELOG.md`（英文与中文成对维护），发现与有意的不决策记入
   `v2/internal/docs/ROADMAP.md`。
5. 开 pull request。模板会问：性质是什么、没有你哪些测试会失败、快照 diff 是什么。

## 门禁会盯住你的事情

- **格式与 vet。** `make verify` 在每个受维护的 module 上都跑这两样。
- **快照。** 导出 API 面、包路径、协议行为、生成布局与生成契约都是被评审的文件。
  你的改动若移动了其中之一，用 `make update-snapshots` 刷新并读 diff——刷新即是
  评审。
- **稳定承诺。** 源码里标了 `// Stable:` 的行为是承诺；它点名了守着它的测试。
  撤回一条，发布门禁会来问为什么。
- **文档化表面。** 每个导出声明都有 doc 注释；每份文档都是双语对
  （`X.md` / `X_zh.md`）。
- **兼容性。** 冻结前允许有意的破坏，但必须列进 `deliberateBreaks`
  （`v2/tools/api_compatibility_test.go`）并在提交标题加 `!`。冻结之后，破坏需要
  新的 major module。

## 提交信息

带 `v2` scope 的 Conventional Commits（`fix(v2):`、`feat(v2):`……），不兼容的
发布行为加 `!`，标题写性质而不是机制。`AGENTS.md` 里有例子。

## 评审

维护者评审三件事：性质是否成立、让性质成立的是否是最小改动、记录是否到位。
风格分歧由门禁裁决而不是由口味——门禁全绿且性质成立，改动就可以发布。
