# Documentation / 文档导航

The v2 documentation is task-oriented. Current behavior belongs in usage and
architecture documents; temporary plans and session snapshots do not belong in
the maintained documentation set.

v2 文档按任务组织。当前行为写入使用与架构文档；临时计划和会话快照不进入长期
维护的文档集合。

## Start Here

| Goal | Document |
| --- | --- |
| Generate or extend a service | [MICROGEN.md](MICROGEN.md) |
| Build a small service with `kit` | [README.md](README.md#build-with-kit) / [中文](README_zh.md#使用-kit) |
| Understand package boundaries | [ARCHITECTURE.md](ARCHITECTURE.md) |
| Prepare a service for production | [PRODUCTION.md](PRODUCTION.md) |
| Move from v1 to v2 | [MIGRATION.md](MIGRATION.md) |
| Change or release the repository | [MAINTAINING.md](MAINTAINING.md) and [RELEASE.md](RELEASE.md) |

## Package Guides

- [endpoint](endpoint/README.md)
- [transport](transport/README.md)
- [service discovery](sd/README.md)
- [interaction](interaction/README.md)
- [examples](examples/README.md)
- [test tools](tools/README.md)

## Document Ownership

- User-facing behavior: `README*`, `MICROGEN.md`, package guides.
- Design and scope: `ARCHITECTURE.md`, `PRODUCTION.md`.
- Contributor process: `MAINTAINING.md`, `RELEASE.md`.
- Version history: `CHANGELOG.md`, `MIGRATION.md`.
- Generated-project documentation is owned by `cmd/microgen/templates/readme.tmpl`.

When behavior changes, update the nearest authoritative document instead of
adding another roadmap, design draft, or status snapshot.
