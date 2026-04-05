---
description: "go-kit microservice framework — how to build Go microservices with AI-first design"
---

# go-kit Framework Skill

This skill teaches you how to build modern Go microservices using the **go-kit** framework and its companion code generator **microgen**.

## Core Concepts

The go-kit framework follows a clean, three-layer architecture:

1.  **Transport**: Protocol-specific handlers (HTTP, gRPC). Decodes requests and encodes responses.
2.  **Endpoint**: The "wrapper" around your business logic. This is where middleware (logging, metrics, rate limiting, circuit breaking) resides. It uses the `func(ctx, request) (response, error)` signature.
3.  **Service**: Pure business logic, independent of any protocol.

## 30-Second Service (kit)

Use the `kit/` package for rapid prototyping. It supports Go generics to automate JSON decoding/encoding.

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
    
    // Automatic JSON decoding/encoding with Generics
    svc.Handle("/hello", kit.JSON[HelloReq](func(ctx context.Context, req HelloReq) (any, error) {
        return HelloResp{Message: "Hello, " + req.Name + "!"}, nil
    }))
    
    svc.Run()
}
```

---

## Code Generation (microgen)

`microgen` is the recommended way to build production services. It can generate code from three sources:

### 1. Protobuf (.proto)
Define your contract first (Design-First).
```bash
microgen -idl service.proto -out . -import example.com/mysvc -protocols http,grpc
```

### 2. Go Interface (IDL)
Define a plain Go interface as your contract.
```bash
microgen -idl idl.go -out . -import example.com/mysvc
```

### 3. Database (Reverse Engineering)
Generate a full CRUD service from a database table.
```bash
microgen -from-db -db-driver mysql -db-dsn "user:pass@tcp(localhost:3306)/dbname"
```

---

## AI & Skill Integration (Critical for Agents)

go-kit is **AI-First**. By using the `-skill` flag during generation, `microgen` adds an AI Tool definition to your service.

### Accessing Skill Definitions
An AI agent can "discover" your service methods by calling:
- **OpenAI Tool format**: `GET /skill`
- **MCP (Model Context Protocol)**: `GET /skill?format=mcp`

### Why this matters
When an AI agent (like Accio or Claude) sees these definitions, it can **automatically call your service methods** as tools to perform actions or retrieve data for the user.

---

## Project Layout (Generated)

A standard microgen-generated project has this structure:

```
.
├── cmd/main.go          # Wires together Transport, Endpoints, and Service
├── service/             # Your core business logic goes here
├── endpoint/            # Go-kit Endpoints and middleware
├── transport/           # HTTP/gRPC protocol handlers
├── sdk/                 # Automatically generated client SDK for other Go services
├── model/               # (Optional) GORM database models
└── repository/          # (Optional) GORM-based data access layer
```

---

## Best Practices

1.  **Prefer microgen**: Don't write boilerplate transport or endpoint code by hand.
2.  **Business logic in Service**: Keep the `service/` package clean of any HTTP or gRPC imports.
3.  **Use Client SDK**: For inter-service communication, use the generated `sdk/` package — it includes built-in retries and circuit breaking.
4.  **Graceful Shutdown**: All generated services handle SIGINT/SIGTERM to close connections properly.
5.  **Environment Config**: Use `config/config.yaml` and override it with flags (e.g., `-http.addr=:9000`).

## Testing

Run integration tests using the native Go test tool:
```bash
go test -v ./tools/integration_test.go
```
This suite automatically generates services from IDL and Proto and performs smoke tests against them.
