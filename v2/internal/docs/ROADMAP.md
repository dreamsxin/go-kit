# Implementation Roadmap / 实施路线图
English | [简体中文](ROADMAP_zh.md)

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

## Milestone 6 (Complete): Direct v2 Architecture Refactor / v2 直接架构重构

### Decision / 决策

The current `main/v2` source tree will be refactored directly. The work does not
preserve source compatibility with the published `v2.0.0` API and will not add
deprecated forwarding packages merely to retain the old package graph.

当前 `main/v2` 源码直接进行结构重构，不保留已发布 `v2.0.0` 的源码兼容性，也不
通过废弃转发包继续维持旧依赖图。`v2.0.0` tag 保持不可变，所有不兼容变化必须在
最终发布前集中记录到 `MIGRATION.md` 和 `CHANGELOG.md`。

This is an explicit compatibility-policy override for the active development
branch. On 2026-08-14, the repository owner approved publishing the result as
another `/v2` release instead of changing the module path to `/v3`. This is a
direct-refactor-specific SemVer exception and is called out in release notes;
`v2.0.0` remains immutable and the published root refactor release is `v2.1.0`.

2026-08-14，仓库所有者批准继续使用 `/v2` 发布本次不兼容重构，不切换到
`/v3`。该决定是仅针对直接重构的 SemVer 例外，并已在发布说明中明确披露；
`v2.0.0` 保持不变，本次根 module 正式发布版本为 `v2.1.0`。

### Execution Status / 实施状态

- [x] Work Package 0: tests generate into isolated temporary projects and keep
  the worktree clean.
- [x] Work Package 1 implementation: endpoint cache ownership moved to
  `sd/endpointer`, Zap middleware moved to `integrations/zap`, and the core
  endpoint package is guarded against non-standard imports.
- [x] Work Package 1 race gate: the full maintained race suite passes locally
  with a MinGW-w64 C compiler and in the Ubuntu/Windows release workflow.
- [x] Work Package 2: service-discovery contracts now live in `sd`; balancing,
  retry, endpointer, instance cache, and client composition have independent
  packages. Generic SD dependency tests reject gRPC and Consul provider imports.
- [x] Work Package 3: HTTP error encoding and HTTP extension contracts now live
  under `transport/http`; the root transport package retains only the shared
  error-handler contract. Import tests reject HTTP/gRPC cross-dependencies.
- [x] Work Package 4: `kit` is HTTP-only, optional lifecycle components use a
  neutral contract, and gRPC assembly lives in the independent `kit/grpc`
  module.
- [x] Work Package 5: circuit breaker, rate limit, gRPC, Consul, Zap,
  OpenTelemetry, and `microgen` have independent modules and the repository is
  orchestrated by `go.work`. The root runtime module has no third-party
  requirements; legacy `log` is a standard-library-only compatibility facade.
- [x] Work Package 6: generator implementation packages live under
  `cmd/microgen/internal`; generated projects use the new package topology and
  direct `slog`, and minimal HTTP projects resolve no optional provider or
  database dependencies.
- [x] Work Package 7: `kit` is the only quickstart, lower-level wiring is named
  `manual_composition`, package docs use the final graph, and migration docs map
  removed APIs and package paths.
- [x] Work Package 8: dependency boundaries are executable and the current
  closure and `v2.0.0` comparison are captured in `DEPENDENCY_REPORT.md`.
  Functional, contract, API, vet, module, standalone-module, and clean-scope
  gates pass. The `/v2` SemVer exception and `v2.1.0` root version are approved.
  The full Ubuntu/Windows workflow passed on 2026-08-14. The root and all eight
  nested-module tags were published in order and resolve through the public Go
  proxy; the final release record is complete.

### Refactor Goals / 重构目标

- Keep `Service -> Endpoint -> Transport` as the only request architecture.
- Make the base `endpoint`, HTTP transport, generic service discovery,
  interaction runtime, HTTP security, and HTTP assembly path independent of
  provider-specific dependencies.
- Ensure an HTTP-only application does not compile or resolve gRPC, Consul,
  database-driver, generator, Gobreaker, Zap, or OpenTelemetry packages.
- Put interfaces and errors in the package that consumes or owns them; remove
  generic `interfaces`, `events`, and `utils` packages.
- Keep optional integrations in independently testable modules.
- Provide one recommended first-use path: `kit` for a small HTTP service, then
  lower-level `endpoint` and `transport` packages for explicit composition.
- Make the complete validation suite deterministic, cross-platform, and clean
  with respect to the Git worktree.

### Target Package And Module Layout / 目标目录与模块

```text
v2/
  endpoint/                         # endpoint, typed endpoint, generic middleware
  transport/
    http/                           # HTTP contracts, context, query helpers
      client/
      server/
  sd/                               # Event, Instancer, Registrar, Balancer contracts
    endpointer/                     # instance-to-endpoint cache and lifecycle
    balancer/                       # balancing strategies
    retry/                          # protocol-neutral retry execution
    instance/                       # in-memory instance source
  kit/                              # lightweight HTTP assembly and lifecycle
    grpc/                           # optional gRPC lifecycle component module
  interaction/
    mcp/
  security/
    http/
  observability/
    slog/
    otel/                           # optional module
  integrations/
    circuitbreaker/                 # optional module
    grpc/                           # optional module
    ratelimit/                      # optional module
    zap/                            # optional module
    consul/                         # optional module
  cmd/
    microgen/                       # independent tool module
      internal/
        dbschema/
        generator/
        ir/
        parser/
```

The main v2 module must contain only the protocol-neutral runtime, HTTP runtime,
and standard-library integrations. The following directories become nested Go
modules with their own `go.mod`, tests, and release checks:

- `kit/grpc` or the final optional gRPC assembly location;
- `integrations/grpc`;
- `integrations/consul`;
- `integrations/zap`;
- `observability/otel`;
- `integrations/circuitbreaker`;
- `integrations/ratelimit` when it continues to use `golang.org/x/time/rate`;
- `cmd/microgen`.

Nested module paths retain the package import path where practical. Module
versions and tags are owned by `RELEASE.md`; local development uses an explicit
`go.work` file rather than committed consumer-facing `replace` directives.

### Package Migration Map / 包迁移映射

| Current API or file | Target ownership | Required action |
| --- | --- | --- |
| `endpoint.Endpoint`, `Middleware`, typed adapters | `endpoint` | Keep and simplify |
| `endpoint.EndpointCache` | `sd/endpointer` | Move and rename only if clarity improves |
| `endpoint.Factory` | `sd/endpointer` | Move with cache ownership |
| `endpoint.EndpointerOption*` | `sd/endpointer` | Move; remove from endpoint |
| `endpoint.LoggingMiddleware` and `endpoint.Logger` | `integrations/zap` | Move; endpoint stops importing Zap |
| endpoint trace/request ID context helpers | `endpoint` | Keep standard-library implementation |
| `transport.ErrorHandler` | `transport` or protocol owner | Keep only if shared without protocol types |
| `transport.ErrorEncoder` and `DefaultErrorEncoder` | `transport/http/server` | Move; root transport stops importing HTTP |
| `transport/http/interfaces/*` | `transport/http` | Move protocol contracts into the HTTP owner |
| `sd/interfaces.Instancer` and `sd/events.Event` | `sd` | Move to the owning root package |
| `sd/interfaces.Registrar` | `sd` | Replace with error-returning contract |
| `sd/interfaces.Balancer` and `ErrNoEndpoints` | `sd` | Move to root or retry consumer package |
| `sd/endpointer/balancer` | `sd/balancer` | Flatten package path |
| `sd/endpointer/executor` | `sd/retry` | Remove direct gRPC status handling |
| `utils.Exponential` | `sd/retry/internal/backoff` | Make implementation private |
| `sd/consul` | `integrations/consul` | Move into optional module |
| `log` Zap alias package | `integrations/zap` | Remove from core module |
| `kit.WithGRPC` and gRPC fields | `kit/grpc` component | Remove from HTTP core assembly |
| `kit.WithRateLimit`, `WithCircuitBreaker` | explicit endpoint middleware | Remove dependency-owning shortcuts |
| `cmd/microgen/{generator,parser,ir,dbschema}` | `cmd/microgen/internal/*` | Make generator internals non-importable |

### Work Package 0: Clean And Portable Test Harness

Goal: establish a trustworthy baseline before moving packages.

- Generate integration projects under `t.TempDir()` rather than tracked
  `tools/testdata/gen_*` directories.
- Keep only intentional input fixtures and reviewed golden snapshots tracked.
- Normalize CRLF and LF before comparing textual API and contract snapshots.
- Run SQLite introspection and generated-project tests with CGO disabled so the
  generator remains portable without a local C toolchain.
- Add a repository-cleanliness assertion around generation and release checks.
- Run every Go command with the module-declared toolchain.

Acceptance:

```bash
go test ./...
go vet ./...
git diff --check
test -z "$(git status --porcelain)"
```

### Work Package 1: Core Endpoint Extraction

Goal: make importing `endpoint` independent of logging, service discovery, and
provider SDKs.

- Retain only endpoint function types, typed adapters, middleware composition,
  timeout, backpressure, error wrapping, request correlation, and in-memory
  metrics.
- Move endpoint cache, factory, and invalidation behavior to `sd/endpointer`.
- Move Zap logging middleware and Zap field construction to
  `integrations/zap`.
- Remove the `Logger` alias and split mixed-purpose source files.
- Add an import-boundary test that rejects non-standard imports from the core
  endpoint package.

Acceptance:

```bash
go list -f '{{join .Imports "\n"}}' ./endpoint
go test -race ./endpoint
```

The endpoint import list must contain only standard-library packages.

### Work Package 2: Service Discovery Rebuild

Goal: make generic discovery and retry independent of Consul and gRPC.

- Define `Event`, `Instancer`, `Registrar`, `Balancer`, and `ErrNoEndpoints` in
  the owning `sd` packages.
- Define `Registrar.Register() error` and `Deregister() error`; add compile-time
  assertions for every implementation.
- Move endpoint reconciliation and resource closure to `sd/endpointer`.
- Flatten round-robin balancing into `sd/balancer`.
- Rebuild retry in `sd/retry` around caller-provided classification and errors
  implementing `Retryable() bool`.
- Move exponential backoff to an internal package and make cancellation part of
  its contract.
- Move gRPC status classification to the gRPC transport module.
- Move Consul implementation to `integrations/consul` and depend only on public
  SD contracts.

Acceptance:

- Generic SD packages have no imports from gRPC or Consul.
- Closing an endpointer stops its update loop and closes factory resources.
- Retry cancellation interrupts calls and backoff.
- Race tests cover cache updates, registration, balancing, retry, and shutdown.

### Work Package 3: Transport Boundary Cleanup

Goal: make every transport package own all protocol-specific behavior.

- Keep HTTP status, headers, public messages, error codes, encoders, and request
  metadata inside `transport/http`.
- Keep gRPC metadata, status conversion, interceptors, and retry classification
  inside the independent `integrations/grpc` module.
- Remove HTTP types from the root `transport` package. Delete the root package
  when no truly shared behavior remains.
- Preserve equivalent before/after/finalizer ordering across HTTP and gRPC
  without forcing identical function signatures.
- Add package import tests preventing HTTP/gRPC cross-imports.

Acceptance:

- HTTP-only tests and examples build with the gRPC module unavailable.
- gRPC transport does not import HTTP transport packages.
- Non-success client responses remain bounded and become typed transport
  errors.

### Work Package 4: Lightweight Kit Assembly

Goal: make `kit` a small HTTP assembly layer rather than an all-protocol service
container.

- Keep HTTP listener lifecycle, strict JSON registration, health checks,
  request IDs, HTTP middleware, endpoint middleware, and graceful shutdown.
- Remove direct gRPC server fields and options from the core `Service`.
- Introduce a small lifecycle component contract for optional servers.
- Implement gRPC assembly in `kit/grpc`; it may be attached explicitly when an
  application needs both protocols.
- Replace dependency-owning rate-limit and circuit-breaker shortcuts with
  explicit `WithEndpointMiddleware` composition.
- Do not create a development Zap logger implicitly. Logging is application
  owned and installed through standard or optional adapters.

Acceptance:

- A minimal `kit` HTTP application imports no gRPC, Zap, Gobreaker, Consul, or
  database packages.
- `Service.Run(ctx)` still follows caller-owned cancellation.
- Startup failure remains synchronous and shutdown remains bounded.

### Work Package 5: Optional Module Separation

Goal: make module boundaries match component boundaries.

- Create nested modules for provider SDKs, non-standard middleware engines,
  gRPC, and the generator.
- Add a repository `go.work` for development and CI orchestration.
- Remove database drivers, Proto parser, Consul, Zap, gRPC, and OpenTelemetry
  requirements from the main runtime `go.mod`.
- Give every nested module independent tidy, test, vet, and release checks.
- Reject relative `replace` directives from publishable module manifests.

Acceptance:

```bash
go mod tidy
go list -m all
go test ./...
```

Run equivalent commands in every workspace module. All module manifests must
remain unchanged after tidy.

### Work Package 6: Microgen Migration

Goal: generate only the new package topology and keep build-time dependencies
outside the runtime module.

- Move generator implementation under `cmd/microgen/internal`.
- Update Go IDL, Protobuf, and database flows to emit the new SD, transport,
  observability, and kit imports.
- Remove generated use of deleted convenience options and old interface paths.
- Update manifest schema when package ownership or generated artifact ownership
  changes.
- Regenerate all reviewed contracts in temporary directories and explicitly
  review snapshot changes.
- Build generated HTTP-only projects without access to optional modules.

Acceptance:

- Go IDL, Protobuf, and database generated projects build and run.
- Running generation twice produces byte-identical owned artifacts.
- User-owned files survive regeneration and extend operations.
- Generated HTTP-only `go.mod` files contain no unused provider dependencies.

### Work Package 7: Documentation And Examples

Goal: expose one coherent learning path matching the final package graph.

- Make `kit` the only top-level quick start.
- Rename the current low-level quickstart as an explicit manual-composition
  example.
- Update package READMEs after their final imports and ownership stabilize.
- Rewrite `ARCHITECTURE.md` to describe the resulting dependency rules rather
  than the previous graph.
- Record every removed or renamed API in `MIGRATION.md` with before/after import
  examples.
- Update `README*`, `PRODUCTION.md`, `MICROGEN.md`, and generated README
  templates together.

Acceptance:

- Every documented Go example compiles.
- Documentation links pass on a case-sensitive filesystem.
- No document recommends a removed package or convenience option.

### Work Package 8: Repository Closure And Release Decision

Goal: prove the refactor is complete before selecting the publication path.

- Run the full test, race, vet, contract, API, generation, encoding, link, and
  module-tidy suites from a clean worktree.
- Capture dependency lists for the minimal endpoint, HTTP transport, SD, kit,
  interaction, security, and optional-module entry points.
- Compare minimal HTTP build time, binary size, module graph, and dependency
  count against `v2.0.0`.
- Review the final exported API snapshot as a deliberate contract reset.
- Decide whether to publish the incompatible result as an explicitly documented
  v2 SemVer exception or change the module path and release it as v3.

Required final commands:

```bash
make verify-release
make release-check-clean
go test -race ./endpoint ./kit ./transport/http/... ./sd/... ./interaction/...
git diff --check
git status --porcelain
```

The final status output must be empty.

### Dependency Gates / 依赖门禁

These rules are enforced by tests or repository tooling, not only by review:

| Package or module | Allowed non-standard dependency |
| --- | --- |
| `endpoint` | none |
| `transport/http/...` | core endpoint and protocol-neutral transport contracts only |
| generic `sd/...` | core endpoint packages only |
| `kit` | core endpoint and HTTP transport packages only |
| `interaction` | none |
| `interaction/mcp` | interaction only |
| `security/http` | none |
| `observability/slog` | core endpoint and protocol-neutral transport contracts only |
| optional modules | only their declared provider SDK and core contracts |

### Completion Definition / 完成定义

Milestone 6 is complete only when all of the following are true:

- No core package imports provider SDKs or optional protocol modules.
- Every remaining public package has one clear owner and a package comment.
- `interfaces`, `events`, and `utils` catch-all packages no longer exist.
- HTTP-only use does not resolve or compile gRPC, Zap, Consul, database, or
  generator dependencies.
- All generated project modes use the new topology.
- The complete verification suite passes on Linux and Windows and leaves the
  worktree clean.
- Architecture, migration, release, usage, and generator documentation agree.
- The publication path and compatibility impact are explicitly approved and
  recorded before tagging.

## Milestone 7 (Complete): User Workflow Coverage / 用户工作流覆盖

Goal: close the gaps that pushed users outside the framework's request path
or left production questions unanswered.

- W3C Trace Context propagation: `endpoint.TraceContext`,
  `ParseTraceparent`, and `transport/http` extract/inject RequestFuncs;
  `TracingMiddleware` joins incoming traces and mints W3C-conformant IDs.
- Streaming and non-JSON request support: `server.NewSSEServer`/`SSEStream`
  (registered through `kit.HandleSSETyped` with endpoint middleware applied)
  for Server-Sent Events with client-disconnect cancellation;
  `server.ParseMultipartForm` and `server.WriteAttachment` for bounded file
  upload and download.
- Request conventions: `endpoint.Validatable`/`ValidationMiddleware` with
  field-level errors encoded as 400; `transport/http` pagination contract
  (`ParsePage`, `Page`, `PageResult[T]`).
- Resilience middleware: `Fallback` for degradation answers and
  per-key `BulkheadMiddleware` for concurrency isolation; rejection errors
  encode as 429.
- End-to-end examples: `examples/auth` (application-owned authentication and
  authorization) and `examples/todosvc` (SQLite CRUD with graceful database
  shutdown).
- Production guidance: deployment (containers, probes, termination budgets),
  alerting (starter alert set), and background job structure with
  `kit.Lifecycle` wiring.

Completed: every capability ships with focused tests, reviewed contract
snapshots where exported APIs changed, and documentation in the owning
document; the full multi-module verification suite passes.

## Maintenance Rules / 维护规则

- Update this file only when milestone scope, order, or acceptance criteria
  change.
- Record completed behavior in `CHANGELOG.md`, not as growing status notes here.
- Put concrete usage in `README*` or `MICROGEN.md` and package design in
  `ARCHITECTURE.md`.
- Every active milestone must have focused tests and an end-to-end verification
  path before implementation is considered complete.
