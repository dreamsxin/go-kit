# go-kit — Go Microservice Framework

[![Go Version](https://img.shields.io/badge/go-1.25+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE.txt)

Modern, production-ready Go microservice framework with AI-first design. Built for developers who want clear architecture without the boilerplate.

## Core Pillars

1. **Clear Architecture**: Enforces a clean separation between Transport, Endpoint, and Service layers. Uses modern Go features like `any` and generics for maximum flexibility and type safety.
2. **microgen**: A powerful code generator that turns definitions (Protobuf, IDL, or DB Schema) into complete, runnable services in seconds.
3. **AI-Ready (Skills)**: Built-in support for generating AI-tool definitions (OpenAI Tool & MCP compatible). Your microservices can be instantly used by AI agents as "Skills."

---

## Quick Start (30 Seconds)

Define a service in one file and run it.

```go
package main

import (
    "context"
    "github.com/dreamsxin/go-kit/kit"
)

type HelloReq  struct { Name string `json:"name"` }
type HelloResp struct { Message string `json:"message"` }

func main() {
    svc := kit.New(":8080")

    // kit.JSON[Req] creates a typed JSON handler — automatic decode/encode with generics.
    // Use svc.Handle to register it and apply service-level middleware.
    svc.Handle("/hello", kit.JSON[HelloReq](func(ctx context.Context, req HelloReq) (any, error) {
        return HelloResp{Message: "Hello, " + req.Name + "!"}, nil
    }))

    svc.Run()
}
```

### With Middleware

```go
svc := kit.New(":8080",
    kit.WithRateLimit(100),           // 100 req/s
    kit.WithCircuitBreaker(5),        // open after 5 consecutive failures
    kit.WithTimeout(5*time.Second),
    kit.WithRequestID(),              // inject X-Request-ID
    kit.WithLogging(logger),
    kit.WithMetrics(&metrics),
)
```

### With gRPC

```go
svc := kit.New(":8080", kit.WithGRPC(":8081"))

// Register your proto-generated service implementation
pb.RegisterGreeterServer(svc.GRPCServer(), &myGreeter{})

svc.Run() // starts both HTTP and gRPC, graceful shutdown on SIGINT/SIGTERM
```

---

## Three-Layer Architecture

go-kit enforces a clean separation of concerns:

```
┌─────────────────────────────────────────────────────┐
│  Transport  (HTTP / gRPC)                           │
│  Decodes requests, encodes responses, routes calls  │
├─────────────────────────────────────────────────────┤
│  Endpoint   (middleware chain)                      │
│  Logging, metrics, rate limiting, circuit breaking  │
├─────────────────────────────────────────────────────┤
│  Service    (pure business logic)                   │
│  No framework imports, fully testable in isolation  │
└─────────────────────────────────────────────────────┘
```

Each layer has a single responsibility:

- **Service** — implements your domain logic as a plain Go interface.
- **Endpoint** — wraps each service method as `func(ctx, request) (response, error)`, where middleware is composed.
- **Transport** — maps HTTP/gRPC requests to endpoints and back.

The `kit` package provides a zero-boilerplate shortcut for prototyping. For production services, use `microgen` to generate the full three-layer structure.

---

## Code Generation (microgen)

`microgen` automates the boring parts of microservice development.

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
Generate a full CRUD service including GORM models and Repositories from an existing DB.
```bash
microgen -from-db -db-driver mysql -db-dsn "user:pass@tcp(localhost:3306)/dbname"
```

### Key Features
- **AI Skill Generation**: Use the `-skill` flag to generate a `/skill` endpoint for AI agents.
- **Client SDK**: Automatically generates a ready-to-use Go client for your service.
- **Middleware**: Built-in support for Circuit Breakers, Rate Limiting, and Logging.
- **Multi-service**: Supports generating multiple services into a single module with unique filenames.

---

## AI & MCP Integration

go-kit is designed for the agentic era. By enabling the **Skill** feature, your service exposes a machine-readable definition of all its capabilities:

- **OpenAI Tool Format**: Accessible via `GET /skill`
- **MCP (Model Context Protocol)**: Accessible via `GET /skill?format=mcp`

This allows an AI agent (like Accio or Claude) to "see" your service methods as tools it can call to perform actions or fetch data.

---

## Project Structure

A generated `go-kit` project follows this industry-standard layout:

```
.
├── cmd/main.go          # Entry point, wires everything together
├── service/<svcname>/   # Pure business logic (the "What")
├── endpoint/<svcname>/  # Go-kit Endpoints (the "How" - middleware, logic wrapping)
├── transport/<svcname>/ # HTTP/gRPC handlers (the "Where" - protocol specific)
├── pb/                  # (Optional) Protobuf generated code
├── model/               # (Optional) GORM database models
├── repository/          # (Optional) Data access layer
├── sdk/<svcname>sdk/    # Generated client SDK for other services to use
└── skill/               # AI Tool definitions
```

---

## License

[MIT](LICENSE.txt)
