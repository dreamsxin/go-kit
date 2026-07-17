# microgen User Guide

`microgen` generates runnable Go services from a contract. It is the recommended
entry point for a new service; runtime packages remain independently usable.

## Install

From the repository root during v2 development:

```bash
go -C v2 install ./cmd/microgen
```

Inspect the exact CLI supported by the current checkout:

```bash
microgen -h
microgen extend -h
```

## Source Modes

Exactly one initial source mode is required.

### Go IDL

```bash
microgen -idl idl.go -out ./service -import example.com/service
```

The input contains service interfaces and request/response types. `microgen`
copies the contract into the generated project and maps methods into the common
IR used by HTTP, clients, SDKs, OpenAPI 3.1, and skill metadata.

### Protobuf

```bash
microgen \
  -idl service.proto \
  -out ./service \
  -import example.com/service \
  -protocols http,grpc
```

Review the generated files under `pb/`, then generate Go stubs with the `protoc`
command written in the generated README. Supported unary and streaming gRPC
shapes are derived from the same parsed contract.

### Database Schema

```bash
microgen \
  -from-db \
  -driver mysql \
  -dsn 'user:pass@tcp(localhost:3306)/catalog' \
  -dbname catalog \
  -tables users,products \
  -out ./catalog-svc \
  -import example.com/catalog-svc
```

Database introspection supports MySQL, PostgreSQL, and SQLite in the CLI. It is
read-only against the source database. Generated models preserve discovered
columns; startup migration is disabled by default.

## Common Options

| Option | Meaning |
| --- | --- |
| `-out` | Output directory |
| `-import` | Generated Go module path |
| `-protocols` | `http` or `http,grpc` |
| `-prefix` | HTTP route prefix |
| `-config` | Generate configuration support, default `true` |
| `-config-mode` | `file`, `hybrid`, or `remote` |
| `-remote-provider` | Remote provider; currently `consul` |
| `-db` | Generate database runtime wiring, default `true` |
| `-driver` | Generated database driver |
| `-model` | Generate model/repository output, default `true` |
| `-docs` | Generate project documentation, default `true` |
| `-tests` | Generate project tests |
| `-skill` | Generate AI discovery metadata, default `true` |
| `-interaction` | Generate interaction runtime and `/mcp` endpoint |
| `-openapi` | Generate OpenAPI 3.1, `/openapi.json`, and Swagger UI |

Use `microgen -h` as the authoritative option list.

## OpenAPI Contract

Enable contract output with `-openapi`. The generated contract is OpenAPI 3.1;
Swagger UI is only the bundled viewer.

`microgen` writes:

- `docs/openapi.json`: paths, operations, request/response schemas, and
  `components.schemas` generated directly from the common IR;
- `docs/docs.go`: an embed wrapper that serves the generated JSON;
- `GET /openapi.json`: the runtime contract endpoint;
- `GET /swagger/`: Swagger UI configured to read `/openapi.json`.

HTTP transport files do not contain duplicated annotation contracts. The
generated document uses relative URLs, so generated configuration has no
`swagger_host` setting. Generation and extend mode refresh both files under
`docs/`; treat them as generator-owned.

## Minimal And Full Generation

Minimal HTTP project:

```bash
microgen \
  -idl idl.go \
  -out . \
  -import example.com/service \
  -config=false \
  -model=false \
  -db=false
```

HTTP and gRPC project with generated config and tests:

```bash
microgen \
  -idl service.proto \
  -out . \
  -import example.com/service \
  -protocols http,grpc \
  -config=true \
  -config-mode=file \
  -tests
```

Interaction/MCP project:

```bash
microgen \
  -idl idl.go \
  -out . \
  -import example.com/agent-service \
  -interaction \
  -db=false
```

MCP uses long-lived HTTP responses. Configure the generated HTTP server write
timeout to `0` or to a value compatible with the maximum session duration.

## Generated Configuration

When `-config=true`, runtime configuration loads in this order:

```text
defaults
  -> local YAML
  -> environment bootstrap for remote connection settings
  -> optional remote config
  -> final environment overrides
  -> Config.Validate
```

Environment variables use the `APP_` prefix, for example:

```text
APP_HTTP_ADDR
APP_LOG_LEVEL
APP_LOG_FORMAT
APP_DB_DSN
APP_DB_AUTO_MIGRATE
APP_REMOTE_ENABLED
```

Modes:

| Mode | Behavior |
| --- | --- |
| `file` | Local file plus environment; remote loading disabled |
| `hybrid` | Remote loading enabled with local fallback |
| `remote` | Remote loading required; startup fails on remote error |

Examples:

```bash
microgen -idl idl.go -out . -import example.com/svc -config-mode=file

microgen \
  -idl idl.go \
  -out . \
  -import example.com/svc \
  -config-mode=hybrid \
  -remote-provider=consul
```

The final merged config is validated before logger, database, middleware, and
servers are created. Do not place credentials in generated YAML; inject them
through the deployment environment or an application-owned provider.

## Database Migration

Generated services skip `AutoMigrate` by default. Enable it only when startup
schema mutation is intentional:

```text
database.auto_migrate: true
APP_DB_AUTO_MIGRATE=true
-auto-migrate
```

Production schema changes should normally use a dedicated migration process.

## Extend An Existing Project

Extend mode currently accepts a full combined Go IDL contract. Protobuf extend
input is not supported.

Run the read-only compatibility scan first:

```bash
microgen extend -check -out .
```

Append one service or model, or a supported middleware set:

```bash
microgen extend \
  -idl full_combined.go \
  -out . \
  -append-service OrderService

microgen extend \
  -idl full_combined.go \
  -out . \
  -append-model Product

microgen extend \
  -idl full_combined.go \
  -out . \
  -append-middleware tracing,error-handling,metrics
```

Extend mode updates new files and generator-owned aggregation files. It refuses
projects that do not expose the required ownership seams.

## File Ownership

### User-owned

- `service/<service>/service.go`
- `endpoint/<service>/custom_chain.go`
- `cmd/custom_routes.go`
- local config values and application-specific integration packages

### Generator-owned

- `cmd/generated_*.go`
- `endpoint/<service>/generated_chain.go`
- `model/generated_*.go`
- `repository/generated_*.go`
- generated clients, SDKs, skill metadata, OpenAPI assets, and protobuf assets

Do not rely on templates or packages under `cmd/microgen` as runtime extension
APIs.

## After Generation

```bash
cd <output>
go mod tidy
go test ./...
go run ./cmd
```

Read the generated `README.md`, inspect the copied source contract, and implement
business methods before exposing the service.

## Generator Guarantees

- Generated Go files are formatted before they are written.
- Template or formatting errors do not leave partially written output files.
- Generated text uses deterministic trailing whitespace and final newlines.
- External projects use the v2 module path and do not receive an invalid local
  `replace` directive.
- Database introspection does not mutate the source schema.

Generator changes are accepted only after fixture tests, repeat-generation
checks, and an external `go mod tidy` plus `go test ./...` smoke test pass.
