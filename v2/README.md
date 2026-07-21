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

v2.0.0 is the stable v2 contract. Exported APIs, CLI and configuration behavior,
generated ownership, and documented protocol behavior follow semantic
versioning. The repository root remains the v1 module.

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

Install the released generator:

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
		kit.WithRateLimit(100),
	)
	if err != nil {
		log.Fatal(err)
	}

	kit.HandleJSON[HelloRequest](svc, "/hello", func(
		ctx context.Context,
		req HelloRequest,
	) (any, error) {
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

The default HTTP server protects header reads with a 5-second timeout, limits
headers to 1 MiB, and keeps `WriteTimeout` disabled so SSE and other streaming
responses are not terminated unexpectedly. Override the complete policy with
`kit.WithHTTPServerConfig` when a service needs different limits.

Use `kit.HandleJSON` or `kit.HandleJSONEndpoint` for business routes that need
endpoint middleware. Use `Service.Handle` and `Service.HandleFunc` only for raw
HTTP integrations.

## Components

| Package | Responsibility |
| --- | --- |
| `kit` | Small-service assembly and lifecycle |
| `endpoint` | Transport-independent endpoint and middleware composition |
| `transport/http` | HTTP server and client adapters |
| `transport/grpc` | gRPC server and client adapters |
| `sd` | Service discovery, endpoint updates, balancing, and retry execution |
| `interaction` | Tools, resources, prompts, sessions, and policy hooks |
| `interaction/mcp` | MCP Streamable HTTP adapter |
| `log` | Framework logging adapter |
| `observability/slog` | Optional standard-library `slog` endpoint logging |
| `observability/otel` | Optional OpenTelemetry endpoint tracing and metrics module |
| `security/http` | Optional trusted-proxy/IP, CORS, CSRF, and security headers |
| `cmd/microgen` | Contract-driven project generator |

Service-discovery constructors return both a callable endpoint and an owned
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
assembly. `observability/slog` is part of the main module and uses only the
standard library; `observability/otel` is a separate module, so the main v2
source does not import the adapter or its provider setup. Test both adapters with:

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
enabled.

## Validate Changes

```bash
cd v2
go test ./...
go test -race ./kit ./interaction/... ./transport/... ./sd/... ./cmd/microgen/generator
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
