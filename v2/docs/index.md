# go-kit v2 Book

English | [简体中文](index_zh.md)

Use this book to complete a task end to end. Use package README files when you
need the complete API reference.

## Choose A Path

| I want to... | Read first | Then use |
| --- | --- | --- |
| run a service in minutes | [Getting started](getting-started.md) | [examples](../examples/README.md) |
| generate a project | [microgen tutorial](tutorial-microgen.md) | [microgen guide](../MICROGEN.md) |
| assemble a small service by hand | [Getting started](getting-started.md) | [`kit` README](../README.md#build-with-kit) |
| understand the request path | [Core concepts](concepts.md) | [Middleware](middleware.md), [Errors](errors.md) |
| add HTTP, gRPC, SSE, or custom codecs | [Transport README](../transport/README.md) | [Custom transport](custom-transport.md) |
| add discovery, balancing, retry, or health checks | [Service discovery](service-discovery.md) | [SD README](../sd/README.md) |
| add authentication or browser protections | [Auth tutorial](tutorial-auth.md) | [Security README](../security/http/README.md) |
| add logs, metrics, or tracing | [Observability](observability.md) | [Production guide](../PRODUCTION.md) |
| expose tools to AI clients | [MCP tutorial](tutorial-mcp.md) | [Interaction README](../interaction/README.md) |
| deploy and operate a service | [Production guide](../PRODUCTION.md) | [Lifecycle](lifecycle.md), [Troubleshooting](troubleshooting.md) |
| upgrade an existing project | [Migration notes](../MIGRATION.md) | [Changelog](../CHANGELOG.md) |

## Tutorials

Read these in the order that matches your goal:

- [Getting started](getting-started.md): first HTTP service and request.
- [Generating a service](tutorial-microgen.md): Go IDL to a runnable project.
- [A CRUD service](tutorial-crud.md): storage, typed routes, and shutdown.
- [Authentication](tutorial-auth.md): bearer credentials and roles.
- [An MCP server](tutorial-mcp.md): Streamable HTTP and tool calls.

## Core Chapters

- [Core concepts](concepts.md): layers, ownership, context, and boundaries.
- [Middleware](middleware.md): composition, ordering, and flow control.
- [Error handling](errors.md): classification, status mapping, and safe messages.
- [Lifecycle](lifecycle.md): startup, shutdown, health, and jobs.
- [Configuration](configuration.md): generated configuration and precedence.
- [Service discovery](service-discovery.md): snapshots, selection, feedback, and retry.
- [Customization](customization.md): custom middleware, codecs, logs, and errors.
- [Custom transports](custom-transport.md): HTTP codecs and new protocol adapters.
- [Testing](testing.md): unit, HTTP, middleware, and integration tests.
- [Troubleshooting](troubleshooting.md): symptom-first diagnosis.
- [Observability](observability.md): logs, metrics, traces, correlation, and cardinality.

## Package References

The detailed contracts live beside the implementation:

- [`endpoint`](../endpoint/README.md)
- [`transport`](../transport/README.md)
- [`kit`](../README.md#build-with-kit)
- [`sd`](../sd/README.md)
- [`interaction`](../interaction/README.md)
- [`security/http`](../security/http/README.md)
- [`observability/slog`](../observability/slog/README.md)
- [`observability/otel`](../observability/otel/README.md)
- [`integrations/grpc`](../integrations/grpc/README.md)
- [`integrations/consul`](../integrations/consul/README.md)
- [`integrations/etcd`](../integrations/etcd/README.md)
- [`microgen`](../MICROGEN.md)

## Release And Maintenance

- [Production guide](../PRODUCTION.md)
- [Migration notes](../MIGRATION.md)
- [Architecture and boundaries](../ARCHITECTURE.md)
- [Documentation index](../DOCS_INDEX.md)
- [Maintainer guide](../internal/docs/MAINTAINING.md)
- [Release guide](../internal/docs/RELEASE.md)
