# UserService Service

Generated with `go-kit microgen`.

## Project Map

- `.microgen/manifest.json` is the versioned project identity and generator-owned artifact contract.
- `idl.go` is the generated source contract snapshot when the project was generated from Go IDL.
- `pb/` contains generated proto contracts when gRPC/proto output is enabled.
- `service/<name>/service.go` is the primary user-owned business logic file.
- `endpoint/<name>/custom_chain.go` is the user-owned middleware hook file.
- `cmd/custom_routes.go` is the user-owned custom HTTP route hook file.
- `cmd/generated_*.go`, `endpoint/<name>/generated_chain.go`, `model/generated_*.go`, `repository/generated_*.go`, `sdk/`, `client/`, and `docs/` are generator-owned outputs.

For existing projects, run `microgen extend -check -out .` before changing generated seams. It validates the manifest against the filesystem, and extend refuses mutations while drift is present.

## Capability Contract

The service capability contract starts from the input definition and is normalized by `microgen` before output is written. The same contract drives HTTP routes, gRPC/proto assets, generated clients, SDKs, OpenAPI and JSON Schema output, README endpoint listings, and optional MCP tools.
If this project needs AI-facing sessions, tool-call execution, authorization, audit records, or an MCP-style JSON-RPC endpoint, regenerate with `-interaction` or compose the framework `interaction` package directly:

- `interaction.NewRuntime()` with `WithHooks`, `WithResources`, `WithPrompts` chaining for sessions, events, tools, and hooks.
- `interaction.AuthorizationHook` and `interaction.AuditHook` for transport-neutral policy.
- `interaction/mcp.NewHandler` (alias for `NewStreamableHandler`) for the MCP-compliant Streamable HTTP transport with `initialize`, `tools/list`, `tools/call`, `resources/list`, `resources/read`, `prompts/list`, `prompts/get`, `completion/complete`, and `logging/setLevel`.

## Extend Existing Project

Use extend mode for generated projects instead of editing generator-owned files directly.

```bash
# Check whether this project has the generated seams required for safe extension.
microgen extend -check -out .

# Append a service from a full combined Go IDL contract.
microgen extend -idl full_combined.go -out . -append-service OrderService

# Append a model from a full combined Go IDL contract.
microgen extend -idl full_combined.go -out . -append-model Product

# Append generator-owned endpoint middleware.
microgen extend -idl full_combined.go -out . -append-middleware tracing,error-handling,metrics
```

Extend mode updates new generated files plus generator-owned aggregation seams, then commits `.microgen/manifest.json` last. Keep business logic in user-owned files such as `service/<name>/service.go`, `endpoint/<name>/custom_chain.go`, and `cmd/custom_routes.go`.

## Agent Workflow

Use this loop when an AI agent or maintainer changes the generated project:

1. Read this README and inspect the source contract snapshot before editing.
2. Inspect `.microgen/manifest.json` to see the generated surface.
3. Put business behavior in user-owned files; do not hand-edit generator-owned files.
4. For new services, models, or middleware, run `microgen extend -check -out .` before an append command.
5. Use `interaction` runtime hooks for executable AI sessions instead of adding a parallel discovery contract.
6. Run the smallest relevant validation first, usually `go test ./...`, then start the service with `go run ./cmd`.

## Quick Start

```bash

# Review the generated proto contract before generating stubs

# Generate Go stubs from the proto contract first
protoc --go_out=. --go-grpc_out=. pb/userservice/userservice.proto

# Start the service
go run ./cmd

```

## API Endpoints

Runtime inspection:

- `GET /health`




## Proto Notes

- `pb/userservice/userservice.proto` is generated from the current service contract and should be reviewed before running `protoc`.
- If any unsupported shape still falls back to `TODO`, complete those message fields before generating stubs.
- Generated streaming SDK callbacks are synchronous: a slow `send` callback applies backpressure to local message delivery. Applications should use context deadlines/cancellation plus their own bounded queues for long-running work.
- Generated Go SDKs cap response bodies at 4 MiB by default; use `WithMaxResponseBodyBytes` when an endpoint contract requires a different limit.




### UserService


* **GetUser**: `GET /getuser`

* **CreateUser**: `POST /createuser`
