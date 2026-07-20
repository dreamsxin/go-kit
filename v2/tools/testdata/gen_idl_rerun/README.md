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
`docs/openapi.json` is the generated OpenAPI 3.1 contract and `docs/schema.json` is the reusable JSON Schema 2020-12 bundle. The running service exposes them at `GET /openapi.json` and `GET /schema.json`, and serves embedded Swagger UI 5 at `GET /swagger/`. `sdk/typescript/` contains the generated Fetch-based unary HTTP client. Files under `docs/` and `sdk/typescript/` are refreshed by generation and extend mode; do not hand-edit them.
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
2. Check `GET /debug/routes` and, when enabled, `GET /openapi.json`, `GET /schema.json`, or the MCP `tools/list` method to see the generated route and discovery surface.
3. Put business behavior in user-owned files; do not hand-edit generator-owned files.
4. For new services, models, or middleware, run `microgen extend -check -out .` before an append command.
5. Use `interaction` runtime hooks for executable AI sessions instead of adding a parallel discovery contract.
6. Run the smallest relevant validation first, usually `go test ./...`, then start the service with `go run ./cmd`.

## Configuration

Generated config loads through `config.Load(path)`: defaults first, local YAML next, optional remote config after that, final environment overrides last, then `Config.Validate()` before runtime wiring. Environment variables are also applied once before remote loading so they can configure the remote provider connection.

The generated HTTP server defaults to `read_header_timeout: 5s` and `write_timeout: 0s`, which protects header reads without terminating long-lived streaming responses.

- Current generated config mode: `file`
- Current remote provider: `none`
- Local mode keeps remote config disabled and remains the default runnable path.
- Hybrid mode enables remote config with local fallback when the remote provider is unavailable.
- Remote mode enables strict remote loading and fails startup when remote config cannot be loaded.
- Environment overrides use the `APP_` prefix, such as `APP_HTTP_ADDR`, `APP_LOG_LEVEL`, `APP_LOG_FORMAT`, `APP_REMOTE_ENABLED`, and `APP_DB_AUTO_MIGRATE`.
- `logging.level` and `logging.format` are used by `cmd/main.go` when constructing the logger.
- Inbound circuit breaker and retry are opt-in; retry only repeats errors that explicitly implement `Retryable() bool`.
- Database `AutoMigrate` is skipped by default. Enable `database.auto_migrate`, `APP_DB_AUTO_MIGRATE`, or `-auto-migrate` only when startup migrations are intended.

## Quick Start

```bash

# Start the service
go run ./cmd

```

## API Endpoints

Runtime inspection:

- `GET /health`
- `GET /debug/routes`





### UserService


* **CreateUser**: `POST /createuser`

* **GetUser**: `GET /getuser`

* **ListUsers**: `GET /listusers`

* **DeleteUser**: `DELETE /deleteuser`

* **UpdateUser**: `PUT /updateuser`

* **FindByEmail**: `GET /findbyemail`

* **SearchUsers**: `GET /searchusers`

* **QueryStats**: `GET /querystats`

* **RemoveExpired**: `DELETE /removeexpired`

* **EditProfile**: `PUT /editprofile`

* **ModifyEmail**: `PUT /modifyemail`

* **PatchStatus**: `PUT /patchstatus`
