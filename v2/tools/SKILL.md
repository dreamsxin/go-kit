---
description: "Build and generate Go services with go-kit v2"
---

# go-kit v2 Framework Skill

Use this guide when editing a service that imports
`github.com/dreamsxin/go-kit/v2` or when generating one with `microgen`.

## Choose The Path

- New production service: start with `microgen`.
- Small service or prototype: use `kit`.
- Existing custom application: compose `endpoint`, `transport`, `sd`, and
  `interaction` independently.

All paths preserve:

```text
Service -> Endpoint -> Transport
```

Business logic belongs in service code. Endpoint middleware owns reusable
request policy. Transport owns protocol decode/encode and status mapping.

## Small Service With `kit`

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

Use `HandleJSON` for business routes so endpoint middleware runs. Use
`Service.Handle` only for raw HTTP integrations.

## Generate A Service

Go interface:

```bash
microgen -idl idl.go -out . -import example.com/service
```

Protobuf with HTTP and gRPC:

```bash
microgen \
  -idl service.proto \
  -out . \
  -import example.com/service \
  -protocols http,grpc
```

Database schema:

```bash
microgen \
  -from-db \
  -driver mysql \
  -dsn 'user:pass@tcp(localhost:3306)/catalog' \
  -dbname catalog \
  -tables users,products \
  -out . \
  -import example.com/catalog
```

Minimal service without generated config/model/database wiring:

```bash
microgen \
  -idl idl.go \
  -out . \
  -import example.com/service \
  -config=false \
  -model=false \
  -db=false
```

After generation:

```bash
go mod tidy
go test ./...
go run ./cmd
```

Use `microgen -h` for the current option list.

## Generated Project Ownership

Edit:

- `service/<service>/service.go`
- `endpoint/<service>/custom_chain.go`
- `cmd/custom_routes.go`
- `config/config.yaml`

Do not hand-edit:

- `cmd/generated_*.go`
- `endpoint/<service>/generated_chain.go`
- `model/generated_*.go`
- `repository/generated_*.go`
- generated `client/`, `sdk/`, `skill/`, and `pb/` files

Before extending a generated project:

```bash
microgen extend -check -out .
```

Extend mode accepts a full combined Go IDL contract:

```bash
microgen extend -idl full.go -out . -append-service OrderService
microgen extend -idl full.go -out . -append-model Product
microgen extend -idl full.go -out . -append-middleware tracing,error-handling,metrics
```

## AI And MCP

Generated `GET /skill` and `GET /skill?format=mcp` responses are discovery
metadata. They describe capabilities but do not execute tools.

Use `interaction` plus `interaction/mcp` for executable tools, resources,
prompts, sessions, notifications, and MCP Streamable HTTP. Long-lived MCP
responses require an HTTP write timeout compatible with the session duration.

## Configuration

Generated config precedence is:

```text
defaults -> local YAML -> optional remote config -> final environment overrides -> validation
```

Environment variables use `APP_`. Database migration is disabled by default.
Do not enable `AutoMigrate` against an existing production database without an
explicit migration decision.

## Important API Rules

- Handle errors from `kit.New`, `Service.Run`, and `Service.GRPCServer`.
- Keep process signal handling in `main`.
- Do not retry writes without idempotency and explicit error classification.
- `NewJSONClientWithRetry` currently adds timeout, not retries; use `sd` for
  retrying discovered calls.
- Use bounded JSON bodies and strict decoding for public endpoints.
- Do not add IAM, outbox, jobs, object storage, secret management, or a complete
  transaction platform to framework core.

## Validation

For application changes:

```bash
go test ./...
```

For framework changes from the v2 module:

```bash
go test ./...
go test -race ./kit ./interaction ./sd/... ./cmd/microgen/generator
```

Generator changes also require a generated external project to pass
`go mod tidy` and `go test ./...`.
