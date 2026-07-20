# Documentation / 文档导航

The v2 documentation is task-oriented. Current behavior belongs in usage and
architecture documents; the durable implementation sequence belongs only in
`ROADMAP.md`. Temporary plans and session snapshots do not belong in the
maintained documentation set.

v2 文档按任务组织。当前行为写入使用与架构文档；长期实施顺序只写入
`ROADMAP.md`，临时计划和会话快照不进入长期维护文档。

## Start Here

| Goal | Document |
| --- | --- |
| Generate or extend a service | [MICROGEN.md](MICROGEN.md) |
| Build a small service with `kit` | [README.md](README.md#build-with-kit) / [中文](README_zh.md#使用-kit) |
| Understand package boundaries | [ARCHITECTURE.md](ARCHITECTURE.md) |
| Follow the implementation sequence | [ROADMAP.md](ROADMAP.md) |
| Prepare a service for production | [PRODUCTION.md](PRODUCTION.md) |
| Move from v1 to v2 | [MIGRATION.md](MIGRATION.md) |
| Change or release the repository | [MAINTAINING.md](MAINTAINING.md) and [RELEASE.md](RELEASE.md) |

## Package Guides

- [endpoint](endpoint/README.md)
- [transport](transport/README.md)
- [service discovery](sd/README.md)
- [interaction](interaction/README.md)
- [slog observability adapter](observability/slog/README.md)
- [OpenTelemetry observability adapter](observability/otel/README.md)
- [examples](examples/README.md)
- [test tools](tools/README.md)

## Document Ownership

- User-facing behavior: `README*`, `MICROGEN.md`, package guides.
- Design and scope: `ARCHITECTURE.md`, `PRODUCTION.md`.
- Product implementation sequence: `ROADMAP.md`.
- Contributor process: `MAINTAINING.md`, `RELEASE.md`.
- Version history: `CHANGELOG.md`, `MIGRATION.md`.
- Generated-project documentation is owned by `cmd/microgen/templates/readme.tmpl`.

When behavior changes, update the nearest authoritative document. Do not add a
second roadmap, design draft, or status snapshot.
