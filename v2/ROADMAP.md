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
  Schema 2020-12, TypeScript Fetch clients, and optional MCP tools.
- Incremental service/model/middleware extension with user-file preservation.
- Minimal opt-in generator defaults, strict IDL validation, bounded client
  responses, and transport-owned interaction session cleanup.

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

## Milestone 2 (Complete): Contract Quality / 契约质量

- Generated OpenAPI 3.1 documents are parsed into a v3 model and generated JSON
  Schema 2020-12 bundles are compiled in integration tests for Go IDL,
  Protobuf, and database sources.
- The release workflow type-checks generated TypeScript clients with a pinned
  compiler version.
- Go and TypeScript SDKs execute the same path, query, body, header, and
  non-2xx error behavior contract in the release workflow.
- Go IDL, Protobuf, and database sources have reviewed SHA-256 snapshots for
  generator-owned public contract artifacts.

Completed: published contract artifacts are machine-validated, behavior-checked,
and protected from unreviewed deterministic drift.

## Milestone 3 (Complete): Optional Operations Adapters / 可选运维适配

- `observability/slog` provides standard-library structured endpoint logging
  without replacing the core zap logger API.
- `observability/otel` is an independent module for endpoint tracing and
  metrics, with no direct adapter dependency in the main v2 module.
- Provider setup, resources, exporters, sampling, and shutdown remain in
  application assembly.

Completed: applications can adopt standard observability explicitly, while
services that do not use these adapters keep the core dependency path small.

## Milestone 4 (Complete): Optional HTTP Security / 可选 HTTP 安全

- `security/http` provides composable trusted-proxy/client-IP, IP policy, CORS,
  signed double-submit CSRF, and security-header middleware.
- Keep authentication and application authorization policy outside framework
  core.
- Proxy trust, browser-cookie scope, middleware order, and SSE/MCP interactions
  are documented and covered by focused tests.

Completed: common HTTP hardening can be enabled explicitly with standard
`http.Handler` composition and without changing endpoint or transport contracts.

## Milestone 5 (Complete): v2 Release Closure / v2 发布收口

- `make verify-release` runs full functional validation, generated-project and
  contract checks, pinned TypeScript checks, focused race tests, vet, module
  tidy checks, UTF-8/link checks, and the reviewed public API snapshot.
- README examples, migration instructions, CLI behavior, generated ownership,
  and exported runtime packages are covered by executable checks or snapshots.
- `make release-check-clean` verifies the committed v2 scope before tagging.
- Runtime closure now includes MCP lifecycle/version/origin/capability checks,
  single-stream SSE delivery, per-session logging levels, and tool-result error
  semantics; HTTP/gRPC metadata and streaming resource ownership; cancellable
  Consul blocking queries; and streaming-safe `kit` defaults.
- Generator closure now includes bounded SDK response reads, URL resolution,
  repository ordering whitelists, effective logging/timeout wiring, opt-in
  inbound middleware, safe low-rate limiter bursts, pre-bound server listeners,
  database resource closure, and streaming-safe generated HTTP defaults.
- Full regeneration protects user-owned service, assembly, config, and README
  files while manifests enumerate all generator-owned endpoint and transport
  artifacts.
- MCP transport sessions own and release one runtime session; generic JSON
  clients bound successful response bodies; invalid Go IDL fails generation.

Completed: the v2.0.0 compatibility contract and release notes are frozen. The
immutable root tag `v2.0.0` points at the verified release commit, and
`github.com/dreamsxin/go-kit/v2@v2.0.0` resolves through the public Go proxy.
The historical incorrect `v2/v2.0.0` tag has been removed.

## Maintenance Rules / 维护规则

- Update this file only when milestone scope, order, or acceptance criteria
  change.
- Record completed behavior in `CHANGELOG.md`, not as growing status notes here.
- Put concrete usage in `README*` or `MICROGEN.md` and package design in
  `ARCHITECTURE.md`.
- Every active milestone must have focused tests and an end-to-end verification
  path before implementation is considered complete.
