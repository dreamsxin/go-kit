# Tutorial: Generating A Service

English | [简体中文](tutorial-microgen_zh.md)

This tutorial generates a complete service from a Go interface: HTTP routes,
Go and TypeScript SDKs, OpenAPI, and the manifest that tracks ownership.

## 1. Install microgen

```bash
go install github.com/dreamsxin/go-kit/v2/cmd/microgen@v0.2.4
```

## 2. Write the contract

`idl.go` is a plain Go file: interfaces and request/response types, no
framework imports:

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

## 3. Generate

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

From a second terminal:

```bash
cat .microgen/manifest.json
curl http://localhost:8080/health
```

## 4. Understand what you own

The manifest (`.microgen/manifest.json`) records the source mode, enabled
capabilities, route prefix, and every generator-owned artifact. You edit:

- `service/helloservice/service.go` -- the business logic, which initially
  returns a not-implemented error;
- `config/custom.go` -- application-owned configuration extensions;
- `cmd/custom_routes.go` -- extra routes beyond the generated ones.

Everything named `generated_*` is regenerated; never edit it by hand.

## 5. Add OpenAPI and SDKs

```bash
microgen -idl idl.go -out hello-svc -import example.com/hello-svc \
  -protocols http -openapi
```

This adds `docs/openapi.json`, `docs/schema.json`, the embedded Swagger UI at
`/swagger/`, the Go SDK, and a zero-dependency TypeScript Fetch client under
`sdk/typescript/`.

## 6. Extend later

Append a service or model without regenerating the project:

```bash
microgen extend -check -out .
microgen extend -idl full_combined.go -out . -append-model Product
```

Extend mode validates the manifest first and refreshes every generator-owned
artifact, so user-owned files survive.

## Where to go next

- [MICROGEN guide](../MICROGEN.md): every option, extend mode, and ownership
- [tutorial: a CRUD service](tutorial-crud.md): the same shape written by hand
- [testing](testing.md): testing the generated service
