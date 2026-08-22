# Getting Started

English | [简体中文](getting-started_zh.md)

This page takes you from an empty directory to a running service that answers
HTTP requests, in about five minutes. All you need is Go 1.25.8 or later.

## Install

```bash
go get github.com/dreamsxin/go-kit/v2@v2.4.2
```

## The first service

Create `main.go`:

```go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/dreamsxin/go-kit/v2/kit"
)

type GreetRequest struct {
	Name string `json:"name"`
}

type GreetResponse struct {
	Message string `json:"message"`
}

func main() {
	svc, err := kit.New(":8080", kit.WithRequestID())
	if err != nil {
		log.Fatal(err)
	}

	kit.HandleJSONTyped(svc, "POST /greet", func(
		_ context.Context, req GreetRequest,
	) (GreetResponse, error) {
		return GreetResponse{Message: "Hello, " + req.Name + "!"}, nil
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := svc.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
```

Run it, then call it from a second terminal:

```bash
go run .
curl -X POST http://localhost:8080/greet \
  -H "Content-Type: application/json" \
  -d '{"name":"world"}'
curl http://localhost:8080/health
```

The service answers `{"message":"Hello, world!"}`, and `/health` reports the
process state for free.

## What just happened

- `kit.New` validated the configuration and registered `/health`, `/livez`,
  and `/readyz`.
- `HandleJSONTyped` registered a typed JSON route: the request body is decoded
  strictly (unknown fields and extra data are rejected), and the response is
  encoded as JSON with the matching status.
- `svc.Run(ctx)` started the server and shut it down gracefully on `SIGTERM`.

## Next steps

- Generate a complete project (clients, SDKs, OpenAPI) instead of writing it:
  [tutorial: generating a service](tutorial-microgen.md)
- Add storage and CRUD: [tutorial: a CRUD service](tutorial-crud.md)
- Learn the request path: [core concepts](concepts.md)
- Browse every runnable example: [examples](../examples/README.md)
