# Documentation Index

English | [简体中文](DOCS_INDEX_zh.md)

Choose the shortest document for the job. The [book](docs/index.md) gives the
end-to-end path; package READMEs are the API reference; the root guides cover
generation, architecture, production, and upgrades.

## Start Here

| Goal | Document |
| --- | --- |
| first service | [Getting started](docs/getting-started.md) |
| generated project | [microgen tutorial](docs/tutorial-microgen.md) |
| hand-built service | [root README](README.md#build-with-kit) |
| production deployment | [Production guide](PRODUCTION.md) |
| upgrade | [Changelog](CHANGELOG.md) |

## Task Index

| Task | Document |
| --- | --- |
| understand layers and ownership | [Architecture](ARCHITECTURE.md), [Core concepts](docs/concepts.md) |
| compose endpoint middleware | [Middleware](docs/middleware.md), [endpoint README](endpoint/README.md) |
| expose or call HTTP | [transport README](transport/README.md) |
| add gRPC | [transport README](transport/README.md), [gRPC README](integrations/grpc/README.md) |
| add a custom body or protocol | [Custom transports](docs/custom-transport.md), [transport README](transport/README.md) |
| customize errors and envelopes | [Error handling](docs/errors.md), [Customization](docs/customization.md) |
| add service discovery and retry | [Service discovery](docs/service-discovery.md), [SD README](sd/README.md) |
| add authentication and browser security | [Auth tutorial](docs/tutorial-auth.md), [security/http README](security/http/README.md) |
| add logs, metrics, or tracing | [Observability](docs/observability.md) |
| expose MCP tools | [MCP tutorial](docs/tutorial-mcp.md), [interaction README](interaction/README.md) |
| test a service | [Testing](docs/testing.md) |
| diagnose a failure | [Troubleshooting](docs/troubleshooting.md) |
| configure a generated project | [Configuration](docs/configuration.md), [microgen guide](MICROGEN.md) |

## Component Reference

| Component | Reference |
| --- | --- |
| assembly and lifecycle | [`kit`](README.md#build-with-kit), [Lifecycle](docs/lifecycle.md) |
| endpoint and middleware | [`endpoint`](endpoint/README.md), [Middleware](docs/middleware.md) |
| HTTP and gRPC transport | [`transport`](transport/README.md), [`integrations/grpc`](integrations/grpc/README.md) |
| discovery and balancing | [`sd`](sd/README.md), [Service discovery](docs/service-discovery.md) |
| interaction and MCP | [`interaction`](interaction/README.md) |
| security | [`security/http`](security/http/README.md) |
| observability | [`observability/slog`](observability/slog/README.md), [`observability/otel`](observability/otel/README.md), [`integrations/zap`](integrations/zap/README.md) |
| providers | [`consul`](integrations/consul/README.md), [`etcd`](integrations/etcd/README.md) |
| project generator | [`microgen`](MICROGEN.md) |

## Maintainers

- [Maintaining](internal/docs/MAINTAINING.md)
- [Release process](internal/docs/RELEASE.md)
- [Roadmap](internal/docs/ROADMAP.md)
- [Changelog](CHANGELOG.md)

## Ownership Rules

- Runtime behavior: root README, package READMEs, and `docs/` chapters.
- Generated-project behavior: `cmd/microgen/templates/readme.tmpl`.
- Design and scope: `ARCHITECTURE.md` and `PRODUCTION.md`.
- History: `CHANGELOG.md`.
- English and Chinese files are maintained as pairs. Update both when behavior or
  public API changes.
