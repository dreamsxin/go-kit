# Documentation / 文档导航

English | [简体中文](DOCS_INDEX_zh.md)

The v2 documentation is task-oriented. Current behavior belongs in usage and
architecture documents; the durable implementation sequence belongs only in
`ROADMAP.md`. Temporary plans and session snapshots do not belong in the
maintained documentation set.

**Start here**: the [book](docs/index.md) is the complete guide — topic
chapters and complete tutorials organized as Quick Start -> Core Concepts ->
Components -> Production. The tables below are the task index into the same
documentation.

## Quick Start

From zero to a running service in about fifteen minutes:

| Step | Goal | Start here |
| --- | --- | --- |
| 1 | Install and generate a runnable service | [README: Generate A Service](README.md#generate-a-service) |
| 2 | Understand what was generated and what you own | [MICROGEN.md](MICROGEN.md) |
| 3 | Exercise the request path on runnable examples | [examples](examples/README.md) |
| 4 | Compose a small service by hand | [README: Build With kit](README.md#build-with-kit) |

## Component Tour

Read in order to follow the request path from core to edge. Every guide is
self-contained and assumes only the steps above; the last column points at a
runnable example for the component.

| Order | Component | Guide | Runnable example |
| --- | --- | --- | --- |
| 1 | Endpoints, typed endpoints, and middleware composition | [endpoint](endpoint/README.md) | [examples/middleware](examples/README.md) |
| 2 | HTTP transport: server, client, SSE, multipart, pagination, tracing propagation | [transport](transport/README.md) | [examples/quickstart](examples/README.md) |
| 3 | Service assembly: `kit`, health checks, lifecycle, Server-Sent Events | [README: Build With kit](README.md#build-with-kit) | [examples/quickstart](examples/README.md) |
| 4 | Resilience: validation, timeout, fallback, bulkhead, backpressure | [endpoint: Built-In Middleware](endpoint/README.md#built-in-middleware) | [examples/best_practice](examples/README.md) |
| 5 | Response assembly: envelopes and error formats at the transport boundary | [transport: Composition And Nesting](transport/README.md#composition-and-nesting) | [examples/envelope](examples/README.md) |
| 6 | Service discovery, balancing, and retry | [service discovery](sd/README.md) | [examples/sd](examples/README.md) |
| 7 | HTTP security: CORS, CSRF, security headers, IP policy | [security/http](security/http/README.md) | [examples/auth](examples/README.md) |
| 8 | Observability: slog, Zap, OpenTelemetry adapters | [slog](observability/slog/README.md), [otel](observability/otel/README.md), [zap](integrations/zap/README.md) | [PRODUCTION.md](PRODUCTION.md) |
| 9 | AI interaction runtime and MCP Streamable HTTP | [interaction](interaction/README.md) | [examples/mcp_basic](examples/README.md) |
| 10 | Optional integrations: Consul, gRPC (circuit breaking and rate limiting are built into `endpoint`) | [consul](integrations/consul/README.md), [transport gRPC](transport/README.md) | [examples/sd](examples/README.md) |

## By Task

| Task | Document |
| --- | --- |
| Prepare a service for production | [PRODUCTION.md](PRODUCTION.md) |
| Review upgrade actions between releases | [MIGRATION.md](MIGRATION.md) |
| Deep-dive the generator, extend mode, and contracts | [MICROGEN.md](MICROGEN.md) |
| Understand package boundaries and extension rules | [ARCHITECTURE.md](ARCHITECTURE.md) |
| Review released changes | [CHANGELOG.md](CHANGELOG.md) |

## For Maintainers

| Task | Document |
| --- | --- |
| Change or release the repository | [internal/docs/MAINTAINING.md](internal/docs/MAINTAINING.md), [internal/docs/RELEASE.md](internal/docs/RELEASE.md), [RELEASE_MANIFEST.json](RELEASE_MANIFEST.json) |
| Review the implementation sequence | [internal/docs/ROADMAP.md](internal/docs/ROADMAP.md) |
| Review dependency closure | [internal/docs/DEPENDENCY_REPORT.md](internal/docs/DEPENDENCY_REPORT.md) |
| Run verification tooling | [tools](tools/README.md) |

## Document Ownership

- User-facing behavior: `README*`, `MICROGEN.md`, package guides.
- Design and scope: `ARCHITECTURE.md`, `internal/docs/DEPENDENCY_REPORT.md`, `PRODUCTION.md`.
- Product implementation sequence: `internal/docs/ROADMAP.md`.
- Contributor process: `internal/docs/MAINTAINING.md`, `internal/docs/RELEASE.md`.
- Version history: `CHANGELOG.md`, `MIGRATION.md`.
- Generated-project documentation is owned by `cmd/microgen/templates/readme.tmpl`.
- Every maintained document has an English version and a `_zh.md` Chinese
  version; both are updated together.

When behavior changes, update the nearest authoritative document. Do not add a
second roadmap, design draft, or status snapshot.
