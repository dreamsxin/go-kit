# go-kit - Go Microservice Framework

[![Go Version](https://img.shields.io/badge/go-1.25+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE.txt)

English | [简体中文](README_zh.md)

`go-kit` is a Go microservice framework built around a simple idea:

```text
Service -> Endpoint -> Transport
```

You define the service capability once, then `microgen` generates a runnable project with HTTP routes, optional gRPC, config, SDKs, and AI tool metadata.

## Start Here: Generate A Local Service

Use this path when you want a new service that a human or AI agent can continue working on.

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

### 5. Check the generated service

In another terminal:

```bash
curl http://localhost:8080/health
curl http://localhost:8080/debug/routes
curl http://localhost:8080/skill
curl "http://localhost:8080/skill?format=mcp"
```

The generated business method initially returns a scaffolded “not implemented” error. Add real behavior in:

```text
service/helloservice/service.go
```

## Let AI Continue The Work

After generation, give your AI coding agent these files and runtime surfaces first:

- `README.md` in the generated project
- `idl.go`, the source contract snapshot
- `service/<name>/service.go`, where business logic belongs
- `GET /debug/routes`, the live route map
- `GET /skill?format=mcp`, the MCP tool view

Recommended prompt:

```text
Read README.md and idl.go first. Keep business logic in service/<name>/service.go.
Do not hand-edit generator-owned files such as cmd/generated_*.go, endpoint/*/generated_chain.go, or skill/.
Use /debug/routes and /skill?format=mcp to understand the current service surface.
```

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

For proto projects, review generated proto assets under `pb/`, run `protoc`, then start the service.

### From Database

```bash
microgen -from-db -driver mysql -dsn "user:pass@tcp(localhost:3306)/dbname" -out . -import example.com/mysvc
```

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

Environment overrides use the `APP_` prefix, such as `APP_HTTP_ADDR`, `APP_LOG_LEVEL`, and `APP_REMOTE_ENABLED`.

## AI And MCP Integration

Generated services expose AI-readable tool definitions when skill generation is enabled. It is enabled by default.

- OpenAI-style tools: `GET /skill`
- MCP-style tools: `GET /skill?format=mcp`

Responses include `metadata` with:

- `schemaVersion`, currently `microgen.skill.v1`
- `source`, currently `microgen-ir`
- `services`
- `formats`

This lets AI agents discover service methods as callable tools without reverse-engineering HTTP handlers.

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

Use `microgen` for production services. Use the `kit` package for quick prototypes or tiny services.

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

	svc.Handle("/hello", kit.JSON[HelloReq](func(ctx context.Context, req HelloReq) (any, error) {
		return HelloResp{Message: "Hello, " + req.Name + "!"}, nil
	}))

	svc.Run()
}
```

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
