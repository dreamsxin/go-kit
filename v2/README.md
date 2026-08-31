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

**Read the [book](docs/index.md) for the complete guide and tutorials; the
[documentation index](DOCS_INDEX.md) maps every document by task.**

## Status

This directory is the independent Go module:

```text
github.com/dreamsxin/go-kit/v2
```

`v2.6.0` is the current published release. It is the architecture-evolution
release and contains intentional breaking changes to the assembly layer and
error model; see [MIGRATION.md](MIGRATION.md) for the upgrade path.
Per-release changes are recorded in [CHANGELOG.md](CHANGELOG.md).

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
go install github.com/dreamsxin/go-kit/v2/cmd/microgen@latest
```

The generator CLI, including SQLite schema introspection, installs and runs
without CGO or a local C compiler. A generated application may still use a
CGO-backed database driver when that runtime adapter is explicitly selected.

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

`go run ./cmd` blocks and serves until interrupted. Inspect the generated
service from a second terminal while the server keeps running:

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
HTTP transport behavior. The transport-neutral `Host` runs lifecycle
components; the `HTTP` component owns routes and serves them:

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
	svc, err := kit.NewHTTP(":8080",
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

	host, err := kit.NewHost(kit.WithLifecycle(svc))
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()
	if err := host.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
```

`kit.NewHTTP` and `kit.NewHost` validate options and return errors.
`Host.Run` follows the caller-owned context; process signal handling stays in
`main`. A pure worker or gRPC-only service mounts its own components into a
Host without any HTTP.

Dependency-specific endpoint middleware is assembled explicitly. Circuit
breaking and rate limiting are built into `endpoint` and hold no third-party
dependencies:

```go
breaker := endpoint.NewCircuitBreaker(
	endpoint.WithBreakerFailureThreshold(5),
)
svc, err := kit.NewHTTP(":8080", kit.WithEndpointMiddleware(
	breaker.Middleware(),
	endpoint.RateLimitMiddleware(limiter), // limiter implements endpoint.RateLimiter
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
host, err := kit.NewHost(kit.WithLifecycle(svc, grpcComponent))
```

The default HTTP server protects header reads with a 5-second timeout, limits
headers to 1 MiB, and keeps `WriteTimeout` disabled so SSE and other streaming
responses are not terminated unexpectedly. Override the complete policy with
`kit.WithHTTPServerConfig` when a service needs different limits.

Use `kit.HandleJSONTyped` for concrete request and response types,
`kit.HandleJSON` for intentionally dynamic responses, and
`kit.HandleJSONEndpoint` for an existing endpoint. For per-route middleware,
use `kit.HandleJSONTypedWithMiddleware` or `kit.HandleJSONWithMiddleware`;
route middleware composes inside the component-level chain. Register
Server-Sent Events streams with `kit.HandleSSETyped` so endpoint middleware
applies to the stream, or `HTTP.HandleSSE` for a raw streaming handler. Use
`HTTP.Handle` and `HTTP.HandleFunc` only for raw HTTP integrations.

`endpoint.Metrics` is the collector middleware writes into. Its counters are
unexported and guarded internally; read them through `Snapshot()`, which
returns a copyable `endpoint.MetricsSnapshot`, and use `AverageDuration()` for
the mean request duration.

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
| `security` | Transport-neutral authentication subjects and endpoint enforcement |
| `observability/slog` | Optional standard-library `slog` endpoint logging and telemetry assembly |
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
assembly. `observability/slog` uses only the standard library and ships
`NewTelemetry`, which assembles the canonical tracing → metrics → logging
chain; `integrations/zap` owns the current Zap integration, and
`observability/otel` is a separate module. The core `endpoint` package
imports none of these providers. Test the adapters with:

```bash
make test-observability
```

Browser-facing services can compose the standard-library middleware in
[`security/http`](security/http/README.md). Configuration is validated during
assembly, and `kit.WithHTTPMiddleware` can install it across every service
route. The `security` package carries the authenticated subject across
layers and enforces coarse endpoint-level requirements
(`RequireAuthenticated`, `RequireRole`); credential extraction stays
application owned at the protocol boundary. Test the package with
`make test-security`.

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

Every document below also has a Chinese `*_zh.md` version. The book under
`docs/` is the complete guide; [DOCS_INDEX.md](DOCS_INDEX.md) maps every
document by task.

- [Book](docs/index.md): the complete guide
  - Tutorials: [getting started](docs/getting-started.md),
    [a CRUD service](docs/tutorial-crud.md),
    [generating a service](docs/tutorial-microgen.md),
    [authentication](docs/tutorial-auth.md),
    [an MCP server](docs/tutorial-mcp.md)
  - Chapters: [core concepts](docs/concepts.md),
    [error handling](docs/errors.md), [middleware](docs/middleware.md),
    [lifecycle](docs/lifecycle.md), [configuration](docs/configuration.md),
    [testing](docs/testing.md)
- [DOCS_INDEX.md](DOCS_INDEX.md): quick start, component tour, documentation map
- [MICROGEN.md](MICROGEN.md): generator usage and generated ownership
- [ARCHITECTURE.md](ARCHITECTURE.md): package boundaries and extension model
- [PRODUCTION.md](PRODUCTION.md): runtime, security, and observability guidance
- [MIGRATION.md](MIGRATION.md): upgrade notes between releases
- [examples/](examples/README.md): runnable examples

## License

MIT. See [LICENSE.txt](LICENSE.txt).
