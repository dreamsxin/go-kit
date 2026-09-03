# Tutorial: Generating A Service

English | [简体中文](tutorial-microgen_zh.md)

This tutorial generates a complete service from a Go interface: HTTP routes,
Go and TypeScript SDKs, OpenAPI, and the manifest that tracks ownership.

## 1. Install microgen

```bash
go install github.com/dreamsxin/go-kit/v2/cmd/microgen@latest
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
microgen \
  -idl idl.go \
  -out hello-svc \
  -import example.com/hello-svc \
  -protocols http \
  -config

cd hello-svc
go mod tidy
go run ./cmd
```

microgen creates `-out` for you. `-import` is what makes the generated tree
buildable — it is the module path written into `go.mod` and used by every
generated import, so always pass it. `-config` generates the configuration
package, including the user-owned `config/custom.go` referenced below.

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
- `config/custom.go` -- application-owned configuration extensions (only when
  generated with `-config`);
- `cmd/custom_routes.go` -- extra routes beyond the generated ones.

These files are written once and never overwritten, and so is `cmd/main.go`.
Everything else -- the endpoint, transport, SDK, model, repository, and
generated config files -- is generator-owned and rewritten on every run, whether
or not its name starts with `generated_`. The manifest's `artifacts` list is the
authoritative answer to "may I edit this file".

## 5. Add OpenAPI and the TypeScript SDK

```bash
microgen -idl idl.go -out hello-svc -import example.com/hello-svc \
  -protocols http -config -openapi
```

This adds `docs/openapi.json`, `docs/schema.json`, the embedded Swagger UI at
`/swagger/`, and a zero-dependency TypeScript Fetch client under
`sdk/typescript/`. The Go SDK is not part of this: `sdk/helloservicesdk/` was
already generated in step 3, unconditionally.

Re-running generation over an existing project is the normal way to add a
capability. There is no `-force`; user-owned files are skipped because they
already exist.

## 6. Extend later

Append a service or model without regenerating the project:

```bash
microgen extend -check -out .
microgen extend -idl full_combined.go -out . -append-model Product
```

The file name `full_combined.go` is literal advice, not decoration: `-idl` must
contain the **entire** contract -- every existing service and model plus the new
one. Extend refuses to run otherwise, with `append-model requires a full
combined Go IDL contract; missing existing model definitions for: ...`.

Three more rules worth knowing before the first run:

- `-check` validates the manifest and cannot be combined with an append flag;
  run it first, on its own.
- one append per invocation: `-append-service`, `-append-model`, and
  `-append-middleware` are mutually exclusive.
- capabilities are read back from the manifest, not from flags. Passing
  `-openapi` or `-config` to `extend` has no effect; the project keeps whatever
  it was generated with.

Extend refreshes every generator-owned artifact, so user-owned files survive.

## Where to go next

- [MICROGEN guide](../MICROGEN.md): every option, extend mode, and ownership
- [tutorial: a CRUD service](tutorial-crud.md): the same shape written by hand
- [testing](testing.md): testing the generated service
