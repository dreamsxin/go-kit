# go-kit - Go Microservice Framework

[![Go Version](https://img.shields.io/badge/go-1.25+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE.txt)

Modern, production-ready Go microservice framework with AI-first design. Built for developers who want clear architecture without the boilerplate.

## Core Pillars

1. **Clear Architecture**: Enforces a clean separation between Transport, Endpoint, and Service layers. Uses modern Go features like `any` and generics for flexibility and type safety.
2. **microgen**: A code generator that turns definitions (Protobuf, IDL, or DB schema) into complete, runnable services in seconds.
3. **AI-Ready (Skills)**: Built-in support for generating AI tool definitions that are compatible with OpenAI Tool and MCP formats.

---

## Quick Start (30 Seconds)

Define a service in one file and run it.

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

    // kit.JSON[Req] creates a typed JSON handler with automatic decode/encode.
    svc.Handle("/hello", kit.JSON[HelloReq](func(ctx context.Context, req HelloReq) (any, error) {
        return HelloResp{Message: "Hello, " + req.Name + "!"}, nil
    }))

    svc.Run()
}
```

### With Middleware

```go
svc := kit.New(":8080",
    kit.WithRateLimit(100),    // 100 req/s
    kit.WithCircuitBreaker(5), // open after 5 consecutive failures
    kit.WithTimeout(5*time.Second),
    kit.WithRequestID(),
    kit.WithLogging(logger),
    kit.WithMetrics(&metrics),
)
```

If you don't have a logger yet, `kit.WithLogging(nil)` safely degrades to a no-op logger instead of crashing.

Configuration note:

- invalid `kit` option inputs now fail fast at construction time, such as non-positive timeouts, non-positive rate limits, zero circuit-breaker thresholds, or empty gRPC listen addresses.

### With gRPC

```go
svc := kit.New(":8080", kit.WithGRPC(":8081"))

// Register your proto-generated service implementation.
pb.RegisterGreeterServer(svc.GRPCServer(), &myGreeter{})

svc.Run() // starts both HTTP and gRPC with graceful shutdown
```

---

## Three-Layer Architecture

go-kit enforces a clean separation of concerns:

```text
Transport (HTTP / gRPC)
  -> decodes requests, encodes responses, routes calls

Endpoint (middleware chain)
  -> logging, metrics, rate limiting, circuit breaking

Service (pure business logic)
  -> no framework imports, fully testable in isolation
```

Each layer has a single responsibility:

- **Service**: Implements your domain logic as a plain Go interface.
- **Endpoint**: Wraps each service method as `func(ctx, request) (response, error)`, where middleware is composed.
- **Transport**: Maps HTTP/gRPC requests to endpoints and back.

The `kit` package provides a zero-boilerplate shortcut for prototyping. For production services, use `microgen` to generate the full three-layer structure.

---

## Code Generation (microgen)

`microgen` automates the repetitive parts of microservice development.

### Installation

```bash
go install github.com/dreamsxin/go-kit/cmd/microgen@latest
```

### Modes of Operation

#### 1. From Protobuf (.proto)

Generate full HTTP/gRPC services from your contract.

```bash
microgen -idl service.proto -out . -import example.com/mysvc -protocols http,grpc
```

#### 2. From IDL (.go)

Define a Go interface, and let `microgen` build the rest.

```bash
microgen -idl idl.go -out . -import example.com/mysvc
```

#### 3. From Database (Reverse Engineering)

Generate a full CRUD service including GORM models and repositories from an existing DB.

```bash
microgen -from-db -driver mysql -dsn "user:pass@tcp(localhost:3306)/dbname"
```

### Key Features

- **AI Skill Generation**: Use the `-skill` flag to generate a `/skill` endpoint for AI agents.
- **Client SDK**: Automatically generates a ready-to-use Go client for your service.
- **Middleware**: Built-in support for circuit breakers, rate limiting, and logging.
- **Multi-service**: Supports generating multiple services into a single module using the same layout as single-service projects, with one `service/`, `endpoint/`, `transport/`, `client/`, and `sdk/` subtree per service.
- **Incremental Extension**: Existing generated projects can now be extended with `microgen extend -idl <full-combined.go> -out <project> -append-service <Name>`, `-append-model <Name>`, or `-append-middleware <Name[,Name...]>`, updating only generator-owned aggregation files plus newly generated files.

### Extend Existing Projects

`microgen` now has conservative extend paths for appending a new service, a new model, or generator-owned endpoint middleware to an existing generated Go-IDL project.

```bash
microgen extend -check -out ./myservice
microgen extend -idl full_combined.go -out ./myservice -append-service OrderService
microgen extend -idl full_combined.go -out ./myservice -append-model Product
microgen extend -idl full_combined.go -out ./myservice -append-middleware tracing,error-handling,metrics
```

Current extend-mode contract:

- `microgen extend -check -out <project>` scans an existing generated project and prints extend compatibility without changing files
- `append-service`, `append-model`, and `append-middleware` currently require a Go IDL input, not `.proto`
- the `-idl` file must contain the full combined contract for existing services plus the new service being appended
- extend mode scans the target project first and fails clearly when required generator-owned aggregation files are missing
- existing user-owned files such as `service/<svc>/service.go` are not overwritten
- `append-model` expects an existing generated project with model output enabled and updates only generated model/repository files plus generator-owned model wiring seams such as `service/<svc>/generated_repos.go`
- `append-middleware` updates only generator-owned endpoint middleware seams such as `endpoint/<svc>/generated_chain.go` and preserves user-owned `endpoint/<svc>/custom_chain.go`
- extend mode updates generator-owned aggregation files such as `cmd/generated_services.go`, `cmd/generated_routes.go`, `cmd/generated_runtime.go`, and the generator-managed `idl.go` snapshot when present

This conservative contract is intentional so `microgen` can evolve generated projects without turning extension into a handwritten-code merge engine.

Recommended workflow:

```bash
# 1. Check whether the existing project already has the required compatibility seams.
microgen extend -check -out ./myservice

# 2. If the project is ready, run one explicit append operation.
microgen extend -idl full_combined.go -out ./myservice -append-service OrderService
```

The `-check` report is especially useful for older generated projects because it tells you which generator-owned seams are present, which append paths are ready, and which compatibility seams are still missing.

Exit-code note:

- `microgen extend -check` exits with `0` when all supported append paths are ready, and `2` when the scan succeeds but compatibility seams are still missing.

---

## AI & MCP Integration

go-kit is designed for the agentic era. By enabling the skill feature, your service exposes a machine-readable definition of all its capabilities:

- **OpenAI Tool Format**: `GET /skill`
- **MCP (Model Context Protocol)**: `GET /skill?format=mcp`

Behavior notes:

- `/skill` returns OpenAI-style tool definitions by default.
- `/skill?format=openai` is equivalent to `/skill`.
- `/skill?format=mcp` returns MCP-style tools with `inputSchema`.
- unknown `format` values currently fall back to the default OpenAI-style response.
- generated services only expose `/skill` when `microgen` runs with `-skill=true` (enabled by default).

Response shape overview:

- OpenAI-style responses return `{"tools":[{"type":"function","function":{...}}]}`
- MCP-style responses return `{"tools":[{"name":"...","inputSchema":{...}}]}`

This allows an AI agent to discover your service methods as callable tools.

---

## Project Structure

A generated `go-kit` project follows this layout:

```text
.
|-- cmd/main.go          # Entry point, wires everything together
|-- cmd/generated_*.go   # Generator-owned runtime, service, and route wiring
|-- cmd/custom_routes.go # User-owned custom HTTP route hook
|-- service/<svcname>/   # Pure business logic
|-- endpoint/<svcname>/  # Endpoints plus generated/custom middleware seams
|-- transport/<svcname>/ # HTTP/gRPC handlers
|-- client/<svcname>/    # Optional: runnable generated demo client
|-- pb/                  # Optional: proto-related assets for generated gRPC services
|-- model/               # Optional: GORM database models
|-- repository/          # Optional: generated data access layer
|-- sdk/<svcname>sdk/    # Generated client SDK
|-- docs/docs.go         # Optional: Swagger stub scaffold
|-- idl.go               # Optional: copied Go IDL input (not used for .proto input)
`-- skill/               # AI tool definitions
```

When model generation is enabled, `microgen` now keeps generated model schemas and generated repositories in finer-grained files such as `model/generated_<name>.go`, `repository/generated_<name>_repository.go`, and `repository/generated_base.go`. User-customizable model hooks remain in separate `model/<name>.go` files and are not rewritten on rerun.

Generated HTTP business routes stay in generator-owned files such as `cmd/generated_routes.go`, while project-specific custom routes belong in `cmd/custom_routes.go`, which is created once and preserved on rerun.

Generated endpoint middleware chains now live in generator-owned files such as `endpoint/<svc>/generated_chain.go`, while project-specific middleware customization belongs in `endpoint/<svc>/custom_chain.go`, which is created once and preserved on rerun.

---

## Repository Workflow

If you are working on this repository itself rather than using it as a dependency:

- start with [MAINTAINER_GUIDE.md](MAINTAINER_GUIDE.md) for the shortest maintainer/AI-agent entry point
- use [DOCS_INDEX.md](DOCS_INDEX.md) for the full documentation map
- read [PROJECT_SNAPSHOT.md](PROJECT_SNAPSHOT.md) for current state and next recommended work
- use [PROJECT_WORKFLOW.md](PROJECT_WORKFLOW.md) for validation and development workflow

---

## Framework Boundaries

Use these only when you are working on the framework itself:

- Scope, stability, and review rules:
  [FRAMEWORK_BOUNDARIES.md](FRAMEWORK_BOUNDARIES.md),
  [STABILITY.md](STABILITY.md),
  [PACKAGE_SURFACES.md](PACKAGE_SURFACES.md),
  [ANTI_PATTERNS.md](ANTI_PATTERNS.md),
  [PR_CHECKLIST.md](PR_CHECKLIST.md)
- Target architecture:
  [FRAMEWORK_ARCHITECTURE.md](FRAMEWORK_ARCHITECTURE.md)
- `microgen` roadmap and implementation docs:
  [MICROGEN_INDEX.md](MICROGEN_INDEX.md),
  [MICROGEN_NEXT_PHASE.md](MICROGEN_NEXT_PHASE.md),
  [MICROGEN_CONFIG_DESIGN.md](MICROGEN_CONFIG_DESIGN.md),
  [MICROGEN_EXTEND_DESIGN.md](MICROGEN_EXTEND_DESIGN.md),
  [MICROGEN_OWNERSHIP.md](MICROGEN_OWNERSHIP.md),
  [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)

---

## License

[MIT](LICENSE.txt)
