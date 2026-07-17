# Implementation Roadmap / 实施路线图

This is the authoritative implementation sequence for go-kit v2. It tracks
durable product milestones, not session notes or release history.

本文是 go-kit v2 唯一实施路线图，只记录长期产品里程碑，不记录临时会话过程。

## Product Direction / 产品方向

- Keep `Service -> Endpoint -> Transport` as the only runtime architecture.
- Let applications adopt individual packages or generate a complete service
  through `microgen`.
- Prefer explicit ownership, validated configuration, deterministic generation,
  cancellation-aware lifecycle, and safe concurrency defaults.
- Add only capabilities that are reusable across unrelated services. Optional
  integrations stay outside the core dependency path.

## Completed Foundation / 已完成基础

- Independent `/v2` Go module and context-owned lifecycle.
- Component-consistent `kit`, endpoint middleware, HTTP/gRPC transports, service
  discovery, interaction runtime, and MCP Streamable HTTP.
- Read-only database introspection and opt-in migration.
- Deterministic UTF-8 project generation with external build coverage.
- One normalized IR driving routes, Go clients, Go SDKs, OpenAPI 3.1, JSON
  Schema 2020-12, TypeScript Fetch clients, and AI discovery metadata.
- Incremental service/model/middleware extension with user-file preservation.

## Milestone 1 (Complete): Generated Project Identity / 生成项目身份

Goal: replace feature inference as the primary source of truth for generated
projects.

- A versioned `.microgen/manifest.json` is generated.
- It records source mode, module path, enabled capabilities, route prefix,
  services, models, middleware, and generator-owned artifacts.
- `microgen extend -check` validates the manifest against the filesystem and
  reports actionable drift.
- Full generation and every extend operation refresh the manifest.

Completed: generated projects now explain their configuration and ownership
without scanning Go source for configuration clues.

## Milestone 2: Contract Quality / 契约质量

- Validate generated OpenAPI and JSON Schema documents in integration tests.
- Type-check generated TypeScript clients in the release workflow.
- Keep Go and TypeScript path, query, body, and error behavior aligned through
  shared contract tests.
- Add deterministic contract snapshots for Go IDL, Protobuf, and database
  sources.

Done when every published contract artifact is machine-validated and generated
from one IR without transport annotations or hand-maintained duplicates.

## Milestone 3: Optional Operations Adapters / 可选运维适配

- Add standard-library `slog` adapters without replacing the core logger API.
- Add OpenTelemetry tracing and metrics adapters as optional packages.
- Keep provider setup and exporter selection in application assembly.

Done when applications can adopt standard observability without adding those
dependencies to services that do not use them.

## Milestone 4: Optional HTTP Security / 可选 HTTP 安全

- Provide composable trusted-proxy/IP, CORS, CSRF, and security-header handlers.
- Keep authentication and application authorization policy outside framework
  core.
- Document proxy trust and streaming endpoint interactions.

Done when common HTTP hardening can be enabled explicitly without changing
endpoint or transport contracts.

## Milestone 5: v2 Release Closure / v2 发布收口

- Run full tests, focused race tests, generated-project builds, TypeScript type
  checks, UTF-8 checks, and documentation-link checks on a clean worktree.
- Review exported APIs and generated ownership boundaries.
- Ensure README examples and migration instructions match the final CLI.
- Publish v2 only after all earlier milestone acceptance criteria are met.

## Maintenance Rules / 维护规则

- Update this file only when milestone scope, order, or acceptance criteria
  change.
- Record completed behavior in `CHANGELOG.md`, not as growing status notes here.
- Put concrete usage in `README*` or `MICROGEN.md` and package design in
  `ARCHITECTURE.md`.
- Every active milestone must have focused tests and an end-to-end verification
  path before implementation is considered complete.
