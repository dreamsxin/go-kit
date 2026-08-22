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

## Quick Example

```go
svc, err := kit.New(":8080", kit.WithRequestID())
if err != nil {
	log.Fatal(err)
}
kit.HandleJSONTyped(svc, "POST /greet", func(
	_ context.Context, req GreetRequest,
) (GreetResponse, error) {
	return GreetResponse{Message: "Hello, " + req.Name + "!"}, nil
})
// health checks, graceful shutdown, and strict JSON come with the service
```

Full walkthrough: [getting started](v2/docs/getting-started.md).

## Components

| Component | What it provides | Guide |
| --- | --- | --- |
| `endpoint` | typed endpoints, middleware (validation, timeout, circuit breaker, rate limit, fallback, bulkhead, tracing) | [guide](v2/endpoint/README.md) |
| `transport/http` | JSON server/client, SSE, multipart, pagination, response envelopes | [guide](v2/transport/README.md) |
| `kit` | service assembly, health checks, lifecycle | [guide](v2/README.md#build-with-kit) |
| `sd` | service discovery, balancing, retry | [guide](v2/sd/README.md) |
| `interaction` | AI tool runtime and MCP Streamable HTTP | [guide](v2/interaction/README.md) |
| `security/http` | CORS, CSRF, security headers, IP policy | [guide](v2/security/http/README.md) |
| `observability` | slog and OpenTelemetry adapters | [guide](v2/observability/slog/README.md) |
| `microgen` | project generation with Go/TS SDKs and OpenAPI | [guide](v2/MICROGEN.md) |

## Documentation

- [User manual](v2/MANUAL.md): the complete guide
- [Book](v2/docs/index.md): topic chapters and complete tutorials (CRUD,
  generation, authentication, MCP)
- [Examples](v2/examples/README.md): runnable services for every component
- [Production](v2/PRODUCTION.md): deployment, alerting, background jobs
- [Upgrade notes](v2/MIGRATION.md), [Changelog](v2/CHANGELOG.md)

## Current Release

The maintained product line is the independent v2 module under [`v2/`](v2/):

```bash
go get github.com/dreamsxin/go-kit/v2@v2.5.1
go install github.com/dreamsxin/go-kit/v2/cmd/microgen@v0.2.8
```

`v2.4.3` is backward compatible: additive capabilities and behavioral fixes
only.

## Development

```bash
go -C v2 test ./...
go -C v2/tools run ./releaseverify -root .. -suites test,standalone,vet
```

## License

MIT. See [LICENSE.txt](LICENSE.txt).
