# UserService Service

Generated with `go-kit microgen`.

## Project Map

- `idl.go` is the generated source contract snapshot when the project was generated from Go IDL.
- `pb/` contains generated proto contracts when gRPC/proto output is enabled.
- `service/<name>/service.go` is the primary user-owned business logic file.
- `endpoint/<name>/custom_chain.go` is the user-owned middleware hook file.
- `cmd/custom_routes.go` is the user-owned custom HTTP route hook file.
- `cmd/generated_*.go`, `endpoint/<name>/generated_chain.go`, `model/generated_*.go`, `repository/generated_*.go`, `sdk/`, `client/`, and `skill/` are generator-owned outputs.

For existing projects, prefer `microgen extend -check -out .` before changing generated seams.

## Capability Contract

The service capability contract starts from the input definition and is normalized by `microgen` before output is written. The same contract drives HTTP routes, gRPC/proto assets, generated clients, SDKs, README endpoint listings, and AI tool metadata.

When `skill/` is generated, `/skill` exposes OpenAI-style tools and `/skill?format=mcp` exposes MCP-style tool descriptors from that same contract. The response also includes metadata with schema version `microgen.skill.v1`, source, services, and supported formats.
`/skill?format=mcp` is discovery output, not a tool execution endpoint. If this project needs AI-facing sessions, tool-call execution, authorization, audit records, or an MCP-style JSON-RPC endpoint, build that runtime surface with the framework `interaction` package:

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

Extend mode updates new generated files plus generator-owned aggregation seams. Keep business logic in user-owned files such as `service/<name>/service.go`, `endpoint/<name>/custom_chain.go`, and `cmd/custom_routes.go`.

## Agent Workflow

Use this loop when an AI agent or maintainer changes the generated project:

1. Read this README and inspect the source contract snapshot before editing.
2. Check `GET /debug/routes` and, when enabled, `GET /skill` or `GET /skill?format=mcp` to see the generated route and tool discovery surface.
3. Put business behavior in user-owned files; do not hand-edit generator-owned files.
4. For new services, models, or middleware, run `microgen extend -check -out .` before an append command.
5. Use `interaction` runtime hooks for executable AI sessions instead of turning generated `skill/` metadata into business logic.
6. Run the smallest relevant validation first, usually `go test ./...`, then start the service with `go run ./cmd/main.go`.

## Configuration

Generated config loads through `config.Load(path)`: defaults first, local YAML next, environment overrides after that, and remote config last when enabled.

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
go run ./cmd/main.go

```

## API Endpoints

Runtime inspection:

- `GET /health`
- `GET /debug/routes`
- `GET /skill`
- `GET /skill?format=mcp`






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
