# go-kit

[![Go Version](https://img.shields.io/badge/go-1.25.8+-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE.txt)

English | [简体中文](README_zh.md)

`go-kit` is a component-oriented Go service framework built around one request
path:

```text
Service -> Endpoint -> Transport
```

Use only the packages you need, or let `microgen` generate a complete,
runnable service from a Go interface, Protobuf contract, or database schema.

The maintained product line is the independent [`v2/`](v2/) module. Start with
the [v2 README](v2/README.md), use the [documentation index](v2/DOCS_INDEX.md)
to choose a task, or open the [complete book](v2/docs/index.md).

## Quick Example

```go
svc, err := kit.NewHTTP(":8080", kit.WithRequestID())
if err != nil {
	log.Fatal(err)
}
kit.HandleJSONTyped(svc, "POST /greet", func(
	_ context.Context, req GreetRequest,
) (GreetResponse, error) {
	return GreetResponse{Message: "Hello, " + req.Name + "!"}, nil
})
host, err := kit.NewHost(kit.WithLifecycle(svc))
if err != nil {
	log.Fatal(err)
}
// host.Run(ctx) serves with health checks, graceful shutdown, and strict JSON
```

Full walkthrough: [getting started](v2/docs/getting-started.md).

## Components

| Component | What it provides | Guide |
| --- | --- | --- |
| `endpoint` | typed endpoints, middleware (validation, timeout, recovery, circuit breaker, rate limit, fallback, bulkhead, tracing) | [guide](v2/endpoint/README.md) |
| `apperror` | transport-neutral error classification mapped by every transport | [guide](v2/docs/errors.md) |
| `transport/http` | JSON server/client, SSE, multipart, pagination, response envelopes | [guide](v2/transport/README.md) |
| `kit` | service assembly, health checks, lifecycle | [guide](v2/README.md#build-with-kit) |
| `sd` | service discovery, balancing, retry | [guide](v2/sd/README.md) |
| `interaction` | AI tool runtime and MCP Streamable HTTP | [guide](v2/interaction/README.md) |
| `security` | transport-neutral authentication subjects and endpoint enforcement | [guide](v2/ARCHITECTURE.md#optional-security) |
| `security/http` | CORS, CSRF, security headers, IP policy | [guide](v2/security/http/README.md) |
| `observability` | slog, OpenTelemetry, correlation, and bounded telemetry | [guide](v2/docs/observability.md) |
| `microgen` | project generation with Go/TS SDKs and OpenAPI | [guide](v2/MICROGEN.md) |

## Documentation

- [Book](v2/docs/index.md): the complete guide — topic chapters and complete
  tutorials (getting started, CRUD, generation, authentication, MCP)
- [Documentation index](v2/DOCS_INDEX.md): choose a document by task
- [Custom transports](v2/docs/custom-transport.md): custom HTTP codecs and
  non-HTTP protocol adapters
- [Observability](v2/docs/observability.md): logs, metrics, traces, and request correlation
- [Examples](v2/examples/README.md): runnable services for every component
- [Production](v2/PRODUCTION.md): deployment, alerting, background jobs
- [Changelog](v2/CHANGELOG.md)

## Current Release

The maintained product line is the independent v2 module under [`v2/`](v2/):

```bash
go get github.com/dreamsxin/go-kit/v2@latest
go install github.com/dreamsxin/go-kit/v2/cmd/microgen@latest
```

`v2.8.0` is the current architecture release, and it ships as a single module:
one `require`, one tag (see [CHANGELOG](v2/CHANGELOG.md)).

## Development

```bash
go -C v2 test ./...
go -C v2 vet ./...
```

Before a release candidate, run the cross-module and standalone gates:

```bash
go -C v2/tools run ./releaseverify -root .. -suites test,standalone,vet,tidy,race
```

## License

MIT. See [LICENSE.txt](LICENSE.txt).
