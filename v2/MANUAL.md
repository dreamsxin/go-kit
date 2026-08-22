# go-kit v2 Manual

English | [简体中文](MANUAL_zh.md)

This manual is the map of go-kit v2. It gives the shortest path through the
framework and points each topic at its detailed chapter in the
[book](docs/index.md) or its package guide.

## Start here

From zero to a running service in about five minutes:
[getting started](docs/getting-started.md).

## The three invariants

**The request path.** `Service -> Endpoint -> Transport`: business logic is a
plain function of `(context, Request) -> (Response, error)`; transport owns
protocol; middleware composes cross-cutting concerns. Details:
[core concepts](docs/concepts.md).

**Error classification.** Business errors are classified with `apperror`
(`github.com/dreamsxin/go-kit/v2/apperror`); transports map kinds to protocol
statuses automatically. Details: [error handling](docs/errors.md).

**Lifecycle ownership.** The process entry point owns signals and the root
context; the service owns listeners and graceful shutdown. Details:
[lifecycle](docs/lifecycle.md).

## Components

Follow the request path from core to edge. Every component has a package
guide with its API reference and a runnable example.

| Component | In one sentence | Guide | Chapter / example |
| --- | --- | --- | --- |
| `endpoint` | typed endpoints and the middleware catalog | [guide](endpoint/README.md) | [middleware](docs/middleware.md), [examples/middleware](examples/README.md) |
| `transport/http` | JSON server/client, SSE, multipart, pagination, envelopes | [guide](transport/README.md) | [examples/envelope](examples/README.md) |
| `kit` | service assembly, health checks, lifecycle | [guide](README.md#build-with-kit) | [lifecycle](docs/lifecycle.md), [examples/quickstart](examples/README.md) |
| `sd` | service discovery, balancing, retry | [guide](sd/README.md) | [examples/sd](examples/README.md) |
| `interaction` | AI tool runtime and MCP Streamable HTTP | [guide](interaction/README.md) | [tutorial](docs/tutorial-mcp.md) |
| `security/http` | CORS, CSRF, security headers, IP policy | [guide](security/http/README.md) | [tutorial](docs/tutorial-auth.md) |
| `observability` | slog and OpenTelemetry adapters | [guide](observability/slog/README.md) | [PRODUCTION](PRODUCTION.md) |
| `microgen` | project generation with Go/TS SDKs and OpenAPI | [guide](MICROGEN.md) | [tutorial](docs/tutorial-microgen.md) |

## The book

Topic chapters and complete tutorials live under [docs/](docs/index.md):

- Tutorials: [getting started](docs/getting-started.md),
  [a CRUD service](docs/tutorial-crud.md),
  [generating a service](docs/tutorial-microgen.md),
  [authentication](docs/tutorial-auth.md), [an MCP server](docs/tutorial-mcp.md)
- Chapters: [core concepts](docs/concepts.md), [error handling](docs/errors.md),
  [middleware](docs/middleware.md), [lifecycle](docs/lifecycle.md),
  [configuration](docs/configuration.md), [testing](docs/testing.md)

## Production

- [Production guide](PRODUCTION.md): deployment, alerting, background jobs
- [Upgrade notes](MIGRATION.md): actions between releases
- [Changelog](CHANGELOG.md): per-release changes
