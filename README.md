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
microgen -from-db -db-driver mysql -db-dsn "user:pass@tcp(localhost:3306)/dbname"
```

### Key Features

- **AI Skill Generation**: Use the `-skill` flag to generate a `/skill` endpoint for AI agents.
- **Client SDK**: Automatically generates a ready-to-use Go client for your service.
- **Middleware**: Built-in support for circuit breakers, rate limiting, and logging.
- **Multi-service**: Supports generating multiple services into a single module with unique filenames.

---

## AI & MCP Integration

go-kit is designed for the agentic era. By enabling the skill feature, your service exposes a machine-readable definition of all its capabilities:

- **OpenAI Tool Format**: `GET /skill`
- **MCP (Model Context Protocol)**: `GET /skill?format=mcp`

This allows an AI agent to discover your service methods as callable tools.

---

## Project Structure

A generated `go-kit` project follows this layout:

```text
.
|-- cmd/main.go          # Entry point, wires everything together
|-- service/<svcname>/   # Pure business logic
|-- endpoint/<svcname>/  # Go-kit endpoints and middleware wiring
|-- transport/<svcname>/ # HTTP/gRPC handlers
|-- client/<svcname>/    # Optional: runnable generated demo client
|-- pb/                  # Optional: proto-related assets for generated gRPC services
|-- model/               # Optional: GORM database models
|-- repository/          # Optional: Data access layer
|-- sdk/<svcname>sdk/    # Generated client SDK
|-- docs/docs.go         # Optional: Swagger stub scaffold
|-- idl.go               # Optional: copied Go IDL input (not used for .proto input)
`-- skill/               # AI tool definitions
```

---

## Repository Workflow

If you are working on this repository itself rather than using it as a dependency, see [PROJECT_WORKFLOW.md](PROJECT_WORKFLOW.md) for the recommended development workflow across framework packages, `microgen`, examples, and tooling.

If you are resuming a refactor or starting a new AI coding session, read [PROJECT_SNAPSHOT.md](PROJECT_SNAPSHOT.md) first for the current repository status and next recommended steps.

---

## Framework Boundaries

If you are deciding what should belong in the framework, what should remain internal, where customization is allowed, what patterns to avoid, and how to review changes consistently, see [FRAMEWORK_BOUNDARIES.md](FRAMEWORK_BOUNDARIES.md), [STABILITY.md](STABILITY.md), [PACKAGE_SURFACES.md](PACKAGE_SURFACES.md), [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md), [ANTI_PATTERNS.md](ANTI_PATTERNS.md), [PR_CHECKLIST.md](PR_CHECKLIST.md), and [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md).

---

## License

[MIT](LICENSE.txt)
