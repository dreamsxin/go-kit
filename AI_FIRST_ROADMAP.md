# AI-First Framework Roadmap

Purpose:
- Make `go-kit` easier for humans and AI agents to understand, generate, extend, and verify without weakening the existing `service -> endpoint -> transport` architecture.

## Direction

The framework should stay layered. The AI-first work should improve the product contract around generation, extension, and verification rather than replacing the runtime architecture.

Primary path:

1. Define the service contract.
2. Generate a runnable project with `microgen`.
3. Edit user-owned business files.
4. Inspect generated routes and skill metadata.
5. Extend the project through explicit `microgen extend` commands.
6. Verify with the smallest relevant test loop.

## Release Target

Current posture:

```text
v0.8 Beta
```

The next product target is:

```text
v0.9 AI Interaction Preview
```

The v0.9 goal is to add first-class interaction protocols without weakening the existing `service -> endpoint -> transport` model:

- gRPC streaming for server-stream, client-stream, and bidirectional-stream services
- optional WebSocket transport preview for browser and agent interaction loops after the gRPC streaming path is stable
- AI interaction runtime for sessions, event streams, tool calls, cancellation, and audit hooks
- generated docs, SDKs, and integration tests that make these interaction surfaces usable by humans and AI agents

## Phase 1: Generated Project Orientation

Status:
- Implemented in generated `README.md` through the Project Map, runtime inspection endpoints, and ownership guidance.

Goal:
- Every generated project should tell a human or AI agent where the contract lives, where business code lives, what endpoints expose runtime capability, and which files are generator-owned.

Deliverables:
- Generated `README.md` describes the project map.
- Generated `README.md` distinguishes user-owned and generator-owned files.
- Generated `README.md` points to `/debug/routes`, `/skill`, and `/skill?format=mcp` when skill output is enabled.
- Generator tests protect the orientation text so it does not drift silently.

## Phase 2: Capability Contract Tightening

Status:
- Implemented for generated README/skill output through `microgen.skill.v1` metadata and tests that protect generated capability metadata.

Goal:
- Keep IR as the single source of truth for generated runtime code, docs, client SDKs, OpenAI tools, and MCP tools.

Deliverables:
- Route, skill, SDK, README, and proto output continue deriving from IR.
- Integration coverage verifies generated capability metadata for IDL, Proto, and DB inputs.
- Unsupported contract shapes produce explicit guidance instead of vague placeholders.

## Phase 3: Extension Workflow Hardening

Status:
- Implemented in generated `README.md` through explicit `microgen extend -check`, append-service, append-model, and append-middleware guidance.

Goal:
- Make incremental change the normal workflow for existing generated projects.

Deliverables:
- `microgen extend -check` remains the first diagnostic command.
- `append-service`, `append-model`, and `append-middleware` preserve user-owned files.
- Failure output explains missing generator-owned seams and full-contract requirements.
- Docs keep extend mode framed as a product contract, not a merge helper.

## Phase 4: Config And Runtime Confidence

Status:
- Implemented in generated `README.md` for config-enabled projects through `file`, `hybrid`, and `remote` mode guidance plus environment override hints.

Goal:
- Keep generated services runnable locally while supporting production config needs.

Deliverables:
- `-config-mode file|hybrid|remote` behavior stays documented and tested.
- Remote provider validation and strict remote failure behavior are covered.
- `/debug/routes` remains available as a low-friction runtime inspection endpoint.

## Phase 5: Agent Workflow Packaging

Status:
- Implemented in generated `README.md` through the Agent Workflow loop and in repository docs through the maintainer/workflow entry points.

Goal:
- Let AI agents operate safely with a small, stable command and file map.

Deliverables:
- Repository docs keep a short "start here" path for AI sessions.
- Generated projects include enough local orientation to avoid reading framework internals first.
- Tooling docs map common changes to validation commands.

## Phase 6: Interaction Contract IR

Status:
- Implemented for unary, server-stream, client-stream, bidirectional-stream, WebSocket-session, and event-source method kinds.

Goal:
- Extend the IR so one service contract can describe unary calls, gRPC streams, WebSocket sessions, and AI interaction events.

Deliverables:
- `MethodKind` metadata for:
  - `unary`
  - `server_stream`
  - `client_stream`
  - `bidi_stream`
  - `websocket_session`
  - `event_source`
- cancellation and timeout metadata
- request, response, event, and error message metadata
- tests for Go IDL and Proto conversion into the expanded IR

Remaining:
- cancellation and timeout metadata
- request, response, event, and error envelope metadata beyond the current method shape
- Go IDL syntax for non-unary interaction shapes

## Phase 7: gRPC Streaming

Status:
- In progress. Server-stream, client-stream, and bidirectional-stream service adapters, gRPC server adapters, transport client helpers, and SDK streaming clients are generated from Proto contracts.

Goal:
- Make gRPC streaming a first-class generated transport and SDK surface.

Deliverables:
- parser support for Proto streaming RPC declarations
- generated server-stream, client-stream, and bidirectional-stream handlers
- integration tests proving generated streaming projects compile after `protoc`
- generated gRPC streaming clients and SDK helpers
- integration tests for generated SDK streaming success paths

Remaining:
- stream errors, cancellation, and slow-consumer runtime tests

## Phase 8: WebSocket Transport

Status:
- Optional preview, not required for the v1.0 industrial release gate unless a concrete browser/session product requirement is adopted.

Goal:
- Add a browser- and agent-friendly bidirectional transport for interactive services.

Deliverables:
- `transport/ws/server` and `transport/ws/client`
- a standard JSON envelope with message id, type, method, payload, error, and metadata
- heartbeat, close reason, max message size, and backpressure policy hooks
- generated WebSocket transport, demo client, and SDK support behind a preview flag
- integration tests for request/response, server event push, cancellation, and connection close paths

## Phase 9: AI Interaction Runtime

Status:
- Planned.

Goal:
- Move from tool discovery only to an interaction server runtime that can host AI-facing sessions and tool-call loops.

Deliverables:
- session lifecycle interfaces
- event stream abstraction
- tool registry and tool call execution hooks
- audit and authorization hooks
- MCP server endpoint preview, separate from the existing MCP-style schema response
- generated project orientation for interaction services

## Phase 10: Industrial v1.0 Hardening

Status:
- Planned.

Goal:
- Graduate from beta/preview to an industrial Go framework release.

Deliverables:
- release, changelog, and migration discipline as defined in [RELEASE.md](RELEASE.md)
- authentication, authorization, request limits, and audit guidance
- OpenTelemetry tracing and metrics guidance
- compatibility freeze for stable runtime APIs and default generated output
- CI and release validation matrix that includes streaming, WebSocket, and AI interaction tests
