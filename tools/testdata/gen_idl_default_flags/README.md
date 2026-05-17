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

When `skill/` is generated, `/skill` exposes OpenAI-style tools and `/skill?format=mcp` exposes MCP-style tools from that same contract. The response also includes metadata with schema version `microgen.skill.v1`, source, services, and supported formats.

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


