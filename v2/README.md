# go-kit v2

[![Go Version](https://img.shields.io/badge/go-1.25.8+-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE.txt)

English | [Simplified Chinese](README_zh.md)

`go-kit/v2` is a component-oriented Go service framework built around one
consistent request path:

```text
Service -> Endpoint -> Transport
```

Use only the packages you need, or use `microgen` to generate a complete,
runnable service from a Go interface, Protobuf contract, or database schema.

## Status

This directory is the independent Go module:

```text
github.com/dreamsxin/go-kit/v2
```

`v2.2.0` is the current published v2 release. It contains one narrowly
approved SemVer exception: `endpoint.Metrics.Snapshot` now returns the
lock-free, copy-safe `MetricsSnapshot` value type. Existing v2 users must follow
[MIGRATION.md](MIGRATION.md) before upgrading. Other changes in this release are
additive or behavioral fixes. The earlier `v2.1.0` direct-refactor exception
remains documented in the migration guide. Legacy v1 source remains available
through the immutable `v1.0.0` to `v1.6.0` tags and is no longer maintained on
`main`.

Requires Go 1.25.8 or later.

## Choose An Entry Point

| Goal | Use |
| --- | --- |
| Generate a complete service | `microgen` |
| Build a small service with minimal wiring | `kit` |
| Integrate selected framework capabilities | `endpoint`, `transport`, `sd`, `interaction` |

`kit` is a concise scaffold over the same endpoint and transport components. It
does not define a separate architecture. Raw `http.Handler` registration remains
available as an escape hatch for static files, third-party handlers, probes, and
custom protocols.

## Generate A Service

Install `microgen` while developing in this repository:

```bash
# Run from the repository root.
go -C v2 install ./cmd/microgen
```

Install the refactored generator from its independently versioned module:

```bash
go install github.com/dreamsxin/go-kit/v2/cmd/microgen@v0.2.0
```

The generator CLI, including SQLite schema introspection, installs and runs
without CGO or a local C compiler. A generated application may still use a
CGO-backed database driver when that runtime adapter is explicitly selected.

The `v2.0.0` root module also contains the historical generator. It can still be
installed explicitly, but its generated package graph follows the old contract:

```bash
go install github.com/dreamsxin/go-kit/v2/cmd/microgen@v2.0.0
```

Create `idl.go`:

```go
package hello

import "context"

type HelloRequest struct {
	Name string `json:"name"`
}

type HelloResponse struct {
	Message string `json:"message"`
}

type HelloService interface {
	SayHello(context.Context, HelloRequest) (HelloResponse, error)
}
```

Generate a minimal HTTP service:

```bash
mkdir hello-svc
microgen \
  -idl idl.go \
  -out hello-svc \
  -import example.com/hello-svc \
  -protocols http

cd hello-svc
go mod tidy
go run ./cmd
```

Inspect the generated service:

```bash
cat .microgen/manifest.json
curl http://localhost:8080/health
```

Use `-openapi` when the project needs `openapi.json`, `schema.json`, and the
embedded Swagger UI. `/debug/routes` is available only in config mode after
enabling `debug.routes_enabled`.

With `-openapi`, `microgen` emits OpenAPI 3.1 directly from the same normalized IR
used by routes, clients, SDKs, and optional MCP tools. It also emits a standalone
JSON Schema 2020-12 bundle at `docs/schema.json` and `GET /schema.json`, plus a
zero-runtime-dependency Fetch client under `sdk/typescript/`.
Swagger UI is available at `/swagger/`; its Swagger UI 5 assets are embedded in
the generated binary, so it does not depend on a CDN. It is a viewer for
`/openapi.json`, not a second contract source.

Repository text files and generated JSON are UTF-8 without BOM. The repository
encoding test rejects invalid UTF-8 and replacement characters before release.

Type-check the generated TypeScript source with the release-pinned compiler:

```bash
npx --yes --package typescript@7.0.2 tsc -p sdk/typescript/tsconfig.json
```

The generated business method initially returns a not-implemented error. Add
business behavior in `service/helloservice/service.go`.

For generated config, gRPC, database introspection, interaction/MCP, and extend
mode, see [MICROGEN.md](MICROGEN.md).

## Generated Ownership

Generated projects intentionally separate files you edit from files `microgen`
owns.

Edit:

- `service/<service>/service.go`
- `endpoint/<service>/custom_chain.go`
- `cmd/custom_routes.go`
- `config/config.yaml`
- `config/custom.go`

Do not hand-edit:

- `.microgen/manifest.json`
- `cmd/generated_*.go`
- `endpoint/<service>/generated_chain.go`
- `model/generated_*.go` and `repository/generated_*.go`
- generated `client/`, `sdk/`, `pb/`, and `docs/` assets

The versioned manifest records the source mode, module path, capabilities,
route prefix, services, models, generated middleware, and generator-owned
artifacts. Run `microgen extend -check -out .` before extending a project; it
reports filesystem drift and extend refuses mutations until drift is resolved.

## Build With `kit`

`kit` is the shortest path that still preserves endpoint middleware and strict
HTTP transport behavior:

```go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dreamsxin/go-kit/v2/kit"
)

type HelloRequest struct {
	Name string `json:"name"`
}

type HelloResponse struct {
	Message string `json:"message"`
}

func main() {
	svc, err := kit.New(":8080",
		kit.WithRequestID(),
		kit.WithTimeout(5*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}

	kit.HandleJSONTyped(svc, "/hello", func(
		ctx context.Context,
		req HelloRequest,
	) (HelloResponse, error) {
		return HelloResponse{Message: "Hello, " + req.Name}, nil
	})

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()
	if err := svc.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
```

`kit.New` validates options and returns errors. `Service.Run` follows the
caller-owned context; process signal handling stays in `main`.

Dependency-specific endpoint middleware is assembled explicitly:

```go
limiter := rate.NewLimiter(100, 100)
svc, err := kit.New(":8080", kit.WithEndpointMiddleware(
	ratelimit.NewErroringLimiter(limiter),
))
```

Optional servers implement `kit.Lifecycle`. The gRPC adapter is attached
without adding gRPC to an HTTP-only application:

```go
grpcComponent, err := kitgrpc.New(":8081")
if err != nil {
	return err
}
pb.RegisterGreeterServer(grpcComponent.Server(), greeter)
svc, err := kit.New(":8080", kit.WithLifecycle(grpcComponent))
```

The default HTTP server protects header reads with a 5-second timeout, limits
headers to 1 MiB, and keeps `WriteTimeout` disabled so SSE and other streaming
responses are not terminated unexpectedly. Override the complete policy with
`kit.WithHTTPServerConfig` when a service needs different limits.

Use `kit.HandleJSONTyped` for concrete request and response types,
`kit.HandleJSON` for intentionally dynamic responses, and
`kit.HandleJSONEndpoint` for an existing endpoint. Use `Service.Handle` and
`Service.HandleFunc` only for raw HTTP integrations.

`endpoint.Metrics` is the mutable collector used by middleware. Read it through
`Snapshot()`, which returns a copyable `endpoint.MetricsSnapshot`; use
`AverageDuration()` for the mean request duration.

## Components

| Package | Responsibility |
| --- | --- |
| `kit` | Small-service assembly and lifecycle |
| `apperror` | Transport-neutral application error classification |
| `endpoint` | Transport-independent endpoint and middleware composition |
| `transport/http` | HTTP server and client adapters |
| `integrations/grpc` | Optional gRPC server and client adapters |
| `integrations/consul` | Optional Consul service-discovery provider |
| `sd` | Provider-neutral service-discovery contracts |
| `sd/endpointer`, `sd/balancer`, `sd/retry` | Independently composable discovery runtime |
| `sd/client` | Optional discovery, balancing, and retry composition |
| `interaction` | Tools, resources, prompts, sessions, and policy hooks |
| `interaction/mcp` | MCP Streamable HTTP adapter |
| `log` | Deprecated standard-library logging compatibility facade |
| `observability/slog` | Optional standard-library `slog` endpoint logging |
| `integrations/zap` | Optional Zap endpoint logging adapter |
| `observability/otel` | Optional OpenTelemetry endpoint tracing and metrics module |
| `security/http` | Optional trusted-proxy/IP, CORS, CSRF, and security headers |
| `cmd/microgen` | Contract-driven project generator |

The `sd/client` constructors return both a callable endpoint and an owned
closer. Handle the construction error and close the endpoint resources before
stopping the underlying instancer. Consul registration and deregistration return
errors, and `Instancer.Stop` cancels and joins the active blocking query.

MCP clients must initialize with protocol version `2025-06-18`, send
`notifications/initialized`, and declare `sampling` before the server may issue
sampling requests. Browser requests with an `Origin` header are limited to the
same origin or `StreamableHandler.AllowedOrigins`.

See [ARCHITECTURE.md](ARCHITECTURE.md) for ownership boundaries and extension
rules. The framework intentionally excludes business platforms such as IAM,
outbox workflows, job platforms, object storage, secret platforms, and complete
transaction frameworks.

Optional observability adapters keep provider ownership in application
assembly. `observability/slog` uses only the standard library,
`integrations/zap` owns the current Zap integration, and `observability/otel`
is a separate module. The core `endpoint` package imports none of these
providers. Test the adapters with:

```bash
make test-observability
```

Browser-facing services can compose the standard-library middleware in
[`security/http`](security/http/README.md). Configuration is validated during
assembly, and `kit.WithHTTPMiddleware` can install it across every service
route. Authentication/authorization remain application concerns. Test the
package with `make test-security`.

## Configuration

Generated configuration resolves in this order:

```text
defaults -> local YAML -> optional remote config -> final environment overrides -> validation
```

Environment variables use the `APP_` prefix. Invalid final configuration fails
before runtime wiring starts. Database generation is read-only against the
source schema, and generated services do not run `AutoMigrate` unless explicitly
enabled. Application-owned settings live in `config/custom.go`; microgen creates
that extension file once and preserves it across regeneration.

## Validate Changes

```bash
cd v2
go test ./...
go test -race ./kit ./interaction/... ./transport/... ./sd/...
go -C ./cmd/microgen test -race ./internal/generator
```

Generator changes must also prove that a generated project can run
`go mod tidy` and `go test ./...` outside this repository.

For release contract validation, with Node.js and `npx` available, run the
OpenAPI/JSON Schema validators, pinned TypeScript compiler, cross-SDK HTTP
behavior contract, and deterministic generated-contract snapshots:

```bash
make verify-release
```

After committing the release candidate, verify that the v2 scope is clean:

```bash
make release-check-clean
```

## Documentation

- [DOCS_INDEX.md](DOCS_INDEX.md): documentation map
- [MICROGEN.md](MICROGEN.md): generator usage and generated ownership
- [ARCHITECTURE.md](ARCHITECTURE.md): package boundaries and extension model
- [ROADMAP.md](ROADMAP.md): authoritative v2 implementation sequence
- [PRODUCTION.md](PRODUCTION.md): runtime, security, and observability guidance
- [MIGRATION.md](MIGRATION.md): v1 to v2 migration
- [MAINTAINING.md](MAINTAINING.md): repository workflow and validation
- [examples/](examples/README.md): runnable examples

## License

MIT. See [LICENSE.txt](LICENSE.txt).
