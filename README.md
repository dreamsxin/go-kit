# go-kit - Go Microservice Framework

[![Go Version](https://img.shields.io/badge/go-1.25+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE.txt)

English | [简体中文](README_zh.md)

`go-kit` is a Go microservice framework built around one stable architecture:

```text
Service -> Endpoint -> Transport
```

Define the service contract once. `microgen` generates a runnable project with HTTP routes, optional gRPC, config, clients, SDKs, generated docs, and AI tool discovery metadata.

## Release Status

Current release:

```text
v1.6.0 Stable
```

Stable scope:

- core `service -> endpoint -> transport` runtime layering
- documented `kit`, `endpoint`, HTTP transport, service discovery, logging, and `microgen` CLI behavior
- generated unary HTTP/gRPC projects
- generated config, extend mode, clients, SDKs, and AI skill metadata
- generated Proto gRPC streaming for supported server-stream, client-stream, and bidirectional-stream RPC shapes
- `interaction` and `interaction/mcp` — AI interaction runtime with sessions, events, tools, resources, prompts, hooks, and full MCP 2025-06-18 Streamable HTTP transport
- generated interaction adapters

See [RELEASE.md](RELEASE.md), [STABILITY.md](STABILITY.md), and [AI_FIRST_ROADMAP.md](AI_FIRST_ROADMAP.md).

## Quick Start: Generate A Local Service

Use this path when you want a new service that a human or AI coding agent can continue working on.

### 1. Install `microgen`

```bash
go install github.com/dreamsxin/go-kit/cmd/microgen@latest
```

### 2. Create a project

```bash
mkdir hello-svc
cd hello-svc
```

### 3. Define the service contract

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
	// SayHello returns a greeting.
	SayHello(ctx context.Context, req HelloRequest) (HelloResponse, error)
}
```

### 4. Generate and run

```bash
microgen -idl idl.go -out . -import example.com/hello-svc -config=false -model=false -db=false
go mod tidy
go run ./cmd/main.go
```

### 5. Inspect the service

```bash
curl http://localhost:8080/health
curl http://localhost:8080/debug/routes
curl http://localhost:8080/skill
curl "http://localhost:8080/skill?format=mcp"
```

The generated business method initially returns a scaffolded `not implemented` error. Add real behavior in:

```text
service/helloservice/service.go
```

## AI Agent Workflow

After generation, give your AI coding agent these files and runtime surfaces first:

- generated `README.md`
- `idl.go`, the source contract snapshot when generated from Go IDL
- `service/<name>/service.go`, where business logic belongs
- `GET /debug/routes`, the live route map
- `GET /skill?format=mcp`, the AI tool discovery view

Recommended prompt:

```text
Read README.md and idl.go first. Keep business logic in service/<name>/service.go.
Do not hand-edit generator-owned files such as cmd/generated_*.go, endpoint/*/generated_chain.go, or skill/.
Use /debug/routes and /skill?format=mcp to understand the generated capability surface.
```

`/skill?format=mcp` is discovery metadata, not a tool execution endpoint. For executable AI sessions, use the `interaction` runtime and `interaction/mcp` adapter.

## Where To Edit

Generated projects separate user-owned files from generator-owned files.

Edit these:

- `service/<svc>/service.go` for business logic
- `endpoint/<svc>/custom_chain.go` for custom endpoint middleware
- `cmd/custom_routes.go` for custom HTTP routes
- `config/config.yaml` for local configuration

Avoid hand-editing these:

- `cmd/generated_*.go`
- `endpoint/<svc>/generated_chain.go`
- `model/generated_*.go`
- `repository/generated_*.go`
- `client/`, `sdk/`, `skill/`, and generated `pb/` assets

## Extend An Existing Generated Project

Run a read-only compatibility check first:

```bash
microgen extend -check -out .
```

Then append one explicit capability from a full combined Go IDL contract:

```bash
microgen extend -idl full_combined.go -out . -append-service OrderService
microgen extend -idl full_combined.go -out . -append-model Product
microgen extend -idl full_combined.go -out . -append-middleware tracing,error-handling,metrics
```

Extend mode updates new files plus generator-owned aggregation seams. It is designed to preserve user-owned implementation files.

## Common Generation Modes

### From Go IDL

```bash
microgen -idl idl.go -out . -import example.com/mysvc
```

### From Protobuf

```bash
microgen -idl service.proto -out . -import example.com/mysvc -protocols http,grpc
```

For Proto projects, review generated proto assets under `pb/`, run `protoc`, then start the service.

Proto streaming RPCs are supported for generated gRPC output when the contract uses supported server-stream, client-stream, or bidirectional-stream shapes.

### From Database

```bash
microgen -from-db -driver mysql -dsn "user:pass@tcp(localhost:3306)/dbname" -out . -import example.com/mysvc
```

Database generation is read-only against the source database. Generated models mirror the discovered columns and do not add audit fields that are not present in the table. Generated services also skip GORM `AutoMigrate` by default; enable it explicitly with `database.auto_migrate: true`, `APP_DB_AUTO_MIGRATE=true`, or `-auto-migrate`.

## Config Modes

Generated config loads in this order:

```text
defaults -> local YAML -> environment variables -> optional remote config
```

Choose the mode when generating:

```bash
# Local file + env only
microgen -idl idl.go -out . -import example.com/mysvc -config-mode file

# Local file + env + remote with fallback to local
microgen -idl idl.go -out . -import example.com/mysvc -config-mode hybrid -remote-provider consul

# Remote-first config with strict failure when remote load fails
microgen -idl idl.go -out . -import example.com/mysvc -config-mode remote -remote-provider consul
```

Environment overrides use the `APP_` prefix, such as `APP_HTTP_ADDR`, `APP_LOG_LEVEL`, `APP_LOG_FORMAT`, `APP_REMOTE_ENABLED`, and `APP_DB_AUTO_MIGRATE`.

Generated `logging.level` and `logging.format` are used when constructing the service logger. Endpoint rate limiting is enabled by default; inbound circuit breaker and retry are opt-in (`middleware.circuit_breaker.enabled` and `middleware.retry.enabled`). Generated retry only retries errors that explicitly opt in with `Retryable() bool`, so ordinary business validation errors are not repeated.

## AI And MCP

Generated services expose AI-readable tool definitions when skill generation is enabled. It is enabled by default.

- OpenAI-style tool descriptors: `GET /skill`
- MCP-style tool descriptors: `GET /skill?format=mcp`

Responses include metadata:

- `schemaVersion`, currently `microgen.skill.v1`
- `source`, currently `microgen-ir`
- `services`
- `formats`

For executable AI sessions and tool-call loops, use the interaction runtime:

- `interaction.NewRuntime` for sessions, events, tools, resources, prompts, and hooks
- `interaction.AuthorizationHook` and `interaction.AuditHook` for policy and audit
- `interaction/mcp.NewHandler` — Streamable HTTP MCP transport (alias for `NewStreamableHandler`, supports POST/GET/DELETE with SSE)

The MCP endpoint implements protocol version 2025-06-18 with tools, resources, prompts, completions, logging, sampling, and server-initiated notifications.

See [interaction/README.md](interaction/README.md), [examples/interaction_policy](examples/interaction_policy), and [examples/mcp_full](examples/mcp_full).

## Production Guidance

Read these before production adoption:

- [RELEASE.md](RELEASE.md) for release scope and validation
- [STABILITY.md](STABILITY.md) for stable, semi-stable, and internal surfaces
- [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md) for generated-output compatibility
- [OBSERVABILITY.md](OBSERVABILITY.md) for tracing, metrics, logging, request correlation, and OpenTelemetry integration
- [SECURITY_HARDENING.md](SECURITY_HARDENING.md) for authn/authz, request limits, audit, secrets, and generated-project hardening

## Architecture

The framework keeps concerns separated:

```text
Service
  Pure business logic. No HTTP or gRPC imports.

Endpoint
  Runtime policy: middleware, logging, metrics, rate limits, circuit breakers.

Transport
  Protocol adapters: HTTP, gRPC, request decoding, response encoding.
```

Use `microgen` for production services. Use the `kit` package for quick prototypes or tiny services. Both paths keep the same service -> endpoint -> transport shape: `kit.HandleJSON` is the concise route API, and `kit.HandleJSONEndpoint` attaches an existing `endpoint.Endpoint` to the HTTP transport.

## Tiny Prototype With `kit`

```go
package main

import (
	"context"

	"github.com/dreamsxin/go-kit/kit"
)

type HelloReq struct {
	Name string `json:"name"`
}

type HelloResp struct {
	Message string `json:"message"`
}

func main() {
	svc := kit.New(":8080")

	kit.HandleJSON[HelloReq](svc, "/hello", func(ctx context.Context, req HelloReq) (any, error) {
		return HelloResp{Message: "Hello, " + req.Name + "!"}, nil
	})

	svc.Run()
}
```

Endpoint middleware configured with `kit.WithTimeout`, `kit.WithMetrics`, `kit.WithLogging`, `kit.WithRateLimit`, or `kit.WithCircuitBreaker` applies to `kit.HandleJSON` and `kit.HandleJSONEndpoint`. Plain `svc.Handle` and `svc.HandleFunc` are raw HTTP escape hatches for static files, third-party handlers, probes, or custom protocol endpoints; they receive HTTP context/request ID injection but do not run endpoint middleware.

`kit.New` exposes `/health`, `/livez`, and `/readyz` by default. Add dependency
checks with `kit.WithLivenessCheck` or `kit.WithReadinessCheck` when the service
needs process or readiness probes beyond the default OK response.

## Generated Project Layout

```text
.
|-- cmd/main.go
|-- cmd/generated_*.go
|-- cmd/custom_routes.go
|-- config/
|-- service/<svc>/
|-- endpoint/<svc>/
|-- transport/<svc>/
|-- client/<svc>/
|-- sdk/<svc>sdk/
|-- model/
|-- repository/
|-- pb/
|-- docs/
|-- skill/
`-- idl.go
```

## Working On This Repository

If you are modifying the framework itself rather than using it:

- Start with [MAINTAINER_GUIDE.md](MAINTAINER_GUIDE.md).
- Use [DOCS_INDEX.md](DOCS_INDEX.md) for the documentation map.
- Read [PROJECT_SNAPSHOT.md](PROJECT_SNAPSHOT.md) for current status.
- Use [PROJECT_WORKFLOW.md](PROJECT_WORKFLOW.md) for validation commands.

## License

[MIT](LICENSE.txt)
