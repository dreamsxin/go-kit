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
- `observability/otel` provides endpoint tracing and metrics; no core package
  imports it.
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

- `make verify` runs full functional validation, generated-project and
  contract checks, pinned TypeScript checks, race tests, vet, module
  tidy checks, UTF-8/link checks, and the reviewed public API snapshot.
- README examples, CLI behavior, generated ownership,
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

Completed: the current public baseline is owned by the root module and one root
tag; contract, API, documentation, and dependency boundaries are enforced by
automated release gates.

## Milestone 6 (Complete): Direct v2 Architecture Refactor / v2 直接架构重构

### Decision / 决策

The current source is maintained as the v2.8.0 candidate baseline. Development
snapshots carry no source-compatibility promise; the public contract is defined
by the release manifest, changelog, and reviewed API snapshot.

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
  neutral contract, and gRPC assembly lives in the `kit/grpc` package.
- [x] Work Package 5: provider, transport, and generator boundaries are packages
  inside the single published module. Dependency closure is enforced per package
  by `TestKitHTTPAssemblyDoesNotResolveOptionalDependencies`, and
  `TestOnlyOneModuleIsPublishable` prevents a second published module.
- [x] Work Package 6: generator implementation packages live under
  `cmd/microgen/internal`; generated projects use the new package topology and
  direct `slog`, and minimal HTTP projects resolve no optional provider or
  database dependencies.
- [x] Work Package 7: `kit` is the only quickstart, lower-level wiring is named
  `manual_composition`, and package docs use the final graph.
- [x] Work Package 8: dependency boundaries are executable, and
  `dependency_boundaries_test.go` is the record of them — a package that resolves
  outside its allowed area fails the build rather than contradicting a document.
  Functional, contract, API, vet, module, standalone, and clean-scope gates pass
  for the single published module.

### Refactor Goals / 重构目标

- Keep `Service -> Endpoint -> Transport` as the only request architecture.
- Make the base `endpoint`, HTTP transport, generic service discovery,
  interaction runtime, HTTP security, and HTTP assembly path independent of
  provider-specific dependencies.
- Ensure an HTTP-only application does not compile or resolve gRPC, Consul,
  database-driver, generator, Gobreaker, Zap, or OpenTelemetry packages.
- Put interfaces and errors in the package that consumes or owns them; remove
  generic `interfaces`, `events`, and `utils` packages.
- Keep optional integrations in independently testable packages within the
  published module.
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
    grpc/                           # optional gRPC lifecycle component package
  interaction/
    mcp/
  security/
    http/
  observability/
    slog/
    otel/                           # optional provider package
  integrations/
    consul/                         # optional provider package
    etcd/                           # optional provider package
    grpc/                           # optional provider package
    zap/                            # optional provider package
  cmd/
    microgen/                       # build-time tool package
      internal/
        dbschema/
        generator/
        ir/
        parser/
```

The v2 release is one published Go module. Provider, transport, observability,
and generator boundaries are package boundaries inside that module; only
`examples`, `tools`, and `tools/contractcheck` remain repository-only workspace
modules. The root tag owns the complete v2 product, while package-level import
boundaries and dependency gates keep optional costs out of minimal assemblies.

Adapter modules for `gobreaker` and `golang.org/x/time/rate` are a non-goal.
`endpoint.CircuitBreaker` and `endpoint.RateLimitMiddleware` are dependency-free
and complete — including rate-based tripping and slow-call accounting — and the
whole of such an adapter is wrapping a third-party type in one `Middleware`,
which an application writes in a dozen lines. A module would instead cost a
`go.mod`, release checks, an API snapshot, and bilingual docs in perpetuity.

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
  inside the optional `integrations/grpc` package.
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

### Work Package 5: Optional Package Boundaries (Historical)

Goal: make module boundaries match component boundaries.

- Keep provider SDK, non-standard middleware, gRPC, and generator boundaries at
  the package level inside the single published module.
- Add a repository `go.work` for development and CI orchestration.
- Keep one version and one root tag for published packages; dependency closure
  remains enforced by package-level tests.
- Run tidy, test, vet, and release checks for the root module and repository-only
  modules.

Acceptance:

```bash
go mod tidy
go list -m all
go test ./...
```

Run equivalent commands in every workspace module. All module manifests must
remain unchanged after tidy.

### Work Package 6: Microgen Completion

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
- Build generated HTTP-only projects without importing optional provider packages.

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
- Record every removed or renamed API in `CHANGELOG.md` with the final import
  path and current usage examples.
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
  interaction, security, and optional-package entry points.
- Record minimal HTTP build dependency closure, binary size, and module graph
  as a diagnostic baseline for later releases.
- Review the final exported API snapshot as a deliberate contract reset.
- Publish the reviewed result under the release manifest's root v2 tag.

Required final commands:

```bash
make verify
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
| optional provider packages | only their declared provider SDK and core contracts |

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
- Architecture, release, usage, and generator documentation agree.
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

## Milestone 8 (Complete): Evidence-Backed Quality / 有证据支撑的质量

Goal: make every property the framework claims checkable by something that
fails — a test, a gate, or a benchmark — starting with the places where a
claim is currently made only by a document or a doc comment.

This milestone comes from a full architecture review of the v2.9.0 candidate.
Each work package below states the property to hold, not the defect to avoid,
and each carries its own acceptance command.

### Work Package 1: Browser Security Completeness

Goal: a CSRF token authorizes one session for a bounded time, and every
browser-facing decision states its own scheme and cache scope.

- Bind a CSRF token to a caller-supplied session identity and an issue time,
  and reject tokens outside a configured lifetime. `CSRFConfig` carries the
  session accessor, and an unsafe request whose session cannot be resolved is
  refused.
- Responses that mint a token declare `Cache-Control: no-store` and
  `Vary: Cookie`, so an intermediary stores one user's token for one user.
- CORS and CSRF agree on origin validity: the opaque `null` origin is a value
  both reject, and every CORS branch — allow, reject, and no-origin — declares
  `Vary: Origin`.
- HTTPS detection is declared, not inferred. `SecurityHeadersConfig` and the
  CSRF origin check state whether a trusted proxy terminates TLS, so HSTS and
  same-origin comparison hold behind a load balancer.

Acceptance:

```bash
go test ./security/... -run 'CSRF|CORS|Headers|Proxy' -count=1
go test -race ./security/...
```

Tests assert: a token minted for one session is refused for another; a token
past its lifetime is refused; a minting response carries both cache headers;
`null` fails CORS construction; a rejected preflight still varies on origin;
HSTS is emitted when the proxy declares HTTPS.

### Work Package 2: Measured Performance Baseline

Goal: performance statements rest on benchmarks, so an optimization can be
shown to work and a regression can be seen.

- Benchmarks cover the paths every request crosses: `Chain` with zero and five
  middlewares, `Server.ServeHTTP` over a JSON round trip, `balancer.Pick`,
  `feedback.Table` under 8 and 64 concurrent callers, `Metrics.Observe`, and
  `TracingMiddleware`.
- Optimizations land after their benchmark exists, and each records the
  before/after figure in `CHANGELOG.md`.
- Correlation values reach the request through one context node.
- Instance snapshots and feedback measurements are published copy-on-write and
  read without a lock or a copy, so a selection that consults every candidate
  does not wait on the path that records outcomes.
- Retry runs each attempt on its own goroutine at every attempt count. The
  goroutine is what lets a caller abandon an instance that ignores its context,
  which a deadline needs whether or not a second attempt would follow.
- A change made for performance is kept only when its benchmark shows a gain. A
  change the measurement refutes is reverted, and the figures are recorded where
  the code is so the idea is not retried blind. The `Metrics` collector's single
  mutex is the first such record.

Acceptance:

```bash
go test -run '^$' -bench . -benchmem ./endpoint ./sd/... ./transport/http/server
go test -race ./endpoint ./sd/...
```

### Work Package 3: Probe And Correlation Ownership

Goal: readiness, liveness, and request correlation belong to a component any
transport can mount, so a gRPC service has the same operational surface as an
HTTP one.

- The probe engine — per-check timeout, single-flight gating, panic
  containment, and the response schema — lives in its own package with an
  exported registry and handler.
- `kit` and `kit/grpc` both mount that registry; a `ReadinessProvider`
  attached to a `Host` reaches a probe surface whatever transports are present.
- Probe paths, and whether they share the application listener or an admin
  listener, are options.
- The HTTP half of request-ID handling lives in `transport/http`, so a service
  assembled from the transport packages gets the same header name, validation,
  and generator that `kit` uses.

Acceptance:

```bash
go test ./kit/... ./transport/http/... -count=1
go test ./tools -run TestArchitectureDependencyGates -count=1
```

A gRPC-only assembly answers its readiness probe in a test.

### Work Package 4: Observability Assembly Completeness

Goal: one call sets up OpenTelemetry correctly, telemetry names follow the
semantic conventions, and trace context crosses every transport by default.

- `observability/otel` provides provider, exporter, resource, global W3C
  propagator, and shutdown assembly, in addition to the middleware it has now.
- Instrument names and units follow the OpenTelemetry semantic conventions,
  and HTTP telemetry carries `http.route` and `http.status_code` so response
  status is alertable from metrics.
- gRPC server and client propagate `traceparent` in both directions; `kit` and
  `kit/grpc` extract it without extra wiring.
- `interaction` reports tool invocations through the same logger and
  correlation contract as the request path.
- `NewTelemetry` selects signals individually, and its middlewares carry their
  own names rather than relying on position.

Acceptance:

```bash
go test ./observability/... ./integrations/grpc/... ./interaction/... -count=1
go list -deps ./observability/slog | Select-String opentelemetry
```

The dependency check finds nothing: taking logging alone still costs nothing.

### Work Package 5: Discovery Composition

Goal: a discovery assembly that compiles is an assembly that works, and one
subscription state machine serves every consumer.

- The subscribe, error-grace, and invalidation state machine has one
  implementation shared by the selector, the endpointer, and feedback
  accounting, with one `sortInstances`.
- `sd/balancer` covers every strategy that needs no measurement, and that is the
  whole of its job: the measured ones are assembled by `sd/feedback`, so the
  optional layer stays optional in the build as well as in the API.
- Measurement-driven strategies are obtained together with the accounting that
  feeds them, so a scored, least-request, or slow-start balancer cannot be built
  without its table, its subscription, or its wrapper. Accounting follows
  registration rather than a health verdict without the caller having to know
  that it must.
- An `sd.Registrar` states its conflict semantics — overwrite, create-only, or
  compare-and-swap — as an option, and each provider documents which it
  supports.

Acceptance:

```bash
go test ./sd/... ./integrations/etcd/... ./integrations/consul/... -count=1
go test -race ./sd/...
```

### Work Package 6: HTTP Protocol Semantics

Goal: the JSON server answers protocol-level questions with protocol-level
answers.

- A request whose media type is not JSON receives 415, and media type is
  checked before the body is read.
- Decode failures carry a message written for the caller; an empty body says
  so.
- `JSONDecodeOptions` carries a post-decode hook, so schema validation runs
  where decoding happens for services that want it there.

Acceptance:

```bash
go test ./transport/http/... -count=1
```

### Work Package 7: Generated Code Type Safety

Goal: generated code fails the way hand-written framework code fails — with a
classified error.

- Generated transports and SDKs convert endpoint responses through
  `endpoint.Unwrap`, so a middleware that changes a response type produces a
  diagnosable error instead of a panic.
- `microgen -from-db` validates its own required inputs before opening a
  connection.
- The generated dependency version is derived from the release manifest, so a
  generated project always resolves.

Acceptance:

```bash
go -C ./tools run ./releaseverify -suites test
go test ./cmd/microgen/... -count=1
```

### Completion Definition / 完成定义

Milestone 8 is complete when every work package's acceptance command passes,
the reviewed API snapshot reflects the deliberate surface changes, and
`CHANGELOG.md` records each behavior change with its measured effect where the
change was made for performance.

Completed: every claim this milestone examined is now checked by something that
fails. The properties shipped in v2.9.0 — session-bound CSRF, benchmarked request
paths with their measured effects recorded, a transport-independent probe
registry, one-call OpenTelemetry assembly, one discovery subscription state
machine with measurement-driven balancing that cannot be assembled without its
accounting, protocol-level JSON answers, and generated code that reports a type
mismatch as a classified error. The performance figures include the change
measurement refuted, so the idea is not retried blind.

## Milestone 9 (Complete): A Freeze Worth Declaring / 值得宣布的冻结

Goal: the compatibility contract is enforced by something that fails before it is
promised.

v2 is pre-freeze, and `RELEASE.md` already names the six things the contract will
cover. Auditing each against the gate suite in `tools` found two enforced only by
prose and four enforced only in part. A freeze declared over that is a promise the
tooling cannot hold, and the first accidental break would be found by a consumer
rather than by CI.

Each work package below states the property to hold, and each carries its own
acceptance command.

### Work Package 1: Breaking Changes Are Named, Not Only Detected

Goal: a change to an exported API says whether it is additive or incompatible.

- `TestPublicAPISurfaceSnapshot` stores one digest per package, so adding an
  exported function and deleting one fail identically — a changed hex string — and
  the reviewer diffs `go doc` output by hand to find out which happened. A removal,
  a signature change, and a narrowed interface are reported as incompatible, and
  an addition passes.
- The comparison is against the last released tag, so the question asked is the
  one the contract asks: is this release compatible with the one consumers pinned.
- Refreshing the reviewed surface stays deliberate. `-update-api-snapshot` records
  the new surface; it must not be the way an incompatible change gets waved
  through.

Acceptance:

```bash
go -C ./tools test . -run 'TestPublicAPISurfaceSnapshot|TestAPICompatibility' -count=1
```

The compatibility test fails on a removal and passes on an addition, both proven
by a case in the test itself rather than by trying it on the real surface.

### Work Package 2: The Exported Package Set Is Pinned Deliberately

Goal: moving or renaming an exported package fails a test that exists for that
purpose.

- Today the package path set is written down only as the second column of
  `api_surface.sha256`, so it is protected as a side effect. It is pinned on
  purpose, with a failure message that names the path that moved.
- `cmd/microgen`'s exported surface is either covered or documented as exempt.
  It is excluded from the snapshot now, which is defensible for an internal
  generator but is nowhere stated.

Acceptance:

```bash
go -C ./tools test . -run 'TestExportedPackagePaths' -count=1
```

### Work Package 3: Documented Generator Flags Exist, And Existing Flags Are Documented

Goal: the `microgen` command line and its documentation cannot drift apart.

- `main.go` defines 23 flags; no test reads `MICROGEN.md`, the tutorial, or the
  README flag tables. A documented flag that is renamed breaks nothing but the
  integration tests that happen to pass it. Every flag named in the documentation
  is defined, and every defined flag is documented or explicitly marked internal.
- Extend-mode usage text is hand-written in `newExtendFlagSet`, and
  `TestNewExtendFlagSetUsage` pins the string rather than the correspondence. The
  usage text is derived from, or checked against, the flag set.

Acceptance:

```bash
go -C ./tools test . -run 'TestMicrogenFlagsAreDocumented' -count=1
go test ./cmd/microgen/... -count=1
```

### Work Package 4: Generated File Locations Are Pinned Where They Are Promised

Goal: the layout a consumer edits is the layout the contract covers.

- The generated layout is decided in one place, `internal/generator/layout.go`,
  and checked by roughly sixty scattered `mustExistFile` calls across nine test
  files. Coverage follows whichever fixture happened to need a file, so
  `cmd/generated_runtime.go`, `cmd/generated_services.go`, and every file `config/`
  emits except `config.yaml` and `custom.go` are asserted nowhere. The layout
  `layout.go` declares is pinned as one reviewed list.
- The contract snapshot pins four fixed paths plus two globs, and a glob is not a
  location pin: a directory that stops emitting shrinks the snapshot rather than
  failing it, and `make update-snapshots` then blesses the smaller set. What the
  snapshot promises is stated in terms of paths.
- Which flag combinations the fixtures exercise is stated, because a path emitted
  only under `-interaction` or `-tests` is pinned only if some fixture passes that
  flag.

Acceptance:

```bash
go -C ./tools test . -run 'TestMicrogen.*(Contract|Integration)' -count=1
```

### Work Package 5: Every Documented Configuration Key And Stage Is Exercised

Goal: the precedence chain the documentation draws is the chain the generated
loader runs.

- `TestMicrogenConfigIntegration` proves file over default, env over file, remote
  over file, and env over remote. The flag stage and `Config.Validate` after it —
  documented in `docs/configuration.md` — are not exercised, so an inverted flag
  precedence fails nothing. Every documented stage is covered.
- Of the six documented `APP_*` keys, two are touched. Each documented key is
  read by a test, and a key the loader no longer reads fails one.

Acceptance:

```bash
go -C ./tools test . -run 'TestMicrogenConfigIntegration' -count=1
```

### Work Package 6: Stable Protocol Behaviour Says So

Goal: "documented as stable" is a property of a named behaviour, not a sentence in
a release document.

- Nothing in the codebase or the docs distinguishes protocol behaviour promised to
  be stable from behaviour that merely happens to work, so the contract's sixth
  item currently covers an unnamed set. The stable behaviours are enumerated
  where they are implemented, and each is covered by a test that fails if it
  changes.
- Behaviour deliberately left unstable is marked too, so the absence of a promise
  is also written down.

Acceptance:

```bash
go -C ./tools test . -run 'TestStableProtocolBehaviour' -count=1
go test ./transport/... ./interaction/... -count=1
```

### Completion Definition / 完成定义

Milestone 9 is complete when every work package's acceptance command passes and
each of the six contract items in `RELEASE.md` names the gate that enforces it.
Declaring the freeze is the decision that follows, not part of this milestone: the
milestone's job is to make the declaration safe to make.

Completed: each of the six contract surfaces now names a gate, and
`TestCompatibilityContractNamesItsGates` fails when a named gate stops existing.
An incompatible API change is reported as incompatible rather than as a moved
digest, the published package path set and the generated project layout are
reviewed lists rather than side effects of a hash, the generator's flags and the
generated program's flags are checked against the documents that tabulate them,
and the configuration chain is exercised through the flag stage and the validation
after it, with every documented `APP_*` key proven to reach its field. The sixth
item is no longer an unnamed set: forty-nine protocol behaviours are declared
where they are implemented, each naming a test in its own package, and the two
behaviours deliberately left unstable say so in the same form.

## Milestone 10 (Complete): A Protocol Worth Freezing / 值得冻结的协议

Goal: the MCP behaviour v2 promises is the behaviour the current specification
defines.

Milestone 9 made the protocol promises checkable, and the first thing the check
showed is that one of them is out of date. MCP published 2026-07-28 on
2026-07-28: it retires the `initialize`/`initialized` handshake and the
`Mcp-Session-Id` header, moves protocol version, client identity and client
capabilities into per-request `_meta`, adds `server/discover`, requires
`Mcp-Method` and `Mcp-Name` routing headers, and makes list results cacheable
with `ttlMs`, `cacheScope` and a deterministic order. v2 speaks 2025-06-18. Older
revisions keep a twelve-month deprecation window, so both are served rather than
one replacing the other.

### Work Package 1: The Protocol Version Decides The Request Model

Goal: a request is answered by the revision it names, and the stateless one needs
nothing the request did not carry.

- `MCP-Protocol-Version` selects the request model per request. An absent header
  selects 2025-06-18, which predates the header being mandatory, so nothing that
  works today stops working.
- On 2026-07-28 no session is minted, read or required, and `initialize` and
  `notifications/initialized` are answered as retired rather than served. Client
  identity and capabilities come from `params._meta` and reach a tool the way
  session state used to. `server/discover` answers the client that wants
  capabilities before it commits to anything.
- POST is the whole transport on that revision: GET and DELETE existed for
  sessions.

Acceptance:

```bash
go test ./interaction/... -count=1
go -C ./tools test . -run 'TestStableProtocolBehaviour' -count=1
```

### Work Package 2: Routing Headers Describe The Body

Goal: what a gateway routes, meters and authorizes on is what the request does.

- `Mcp-Method` repeats the JSON-RPC method and `Mcp-Name` the target it
  addresses. A request whose headers disagree with its body is refused, because
  a per-tool rate limit or policy decided on headers would otherwise be applied
  to a different call than the one that runs.
- A method that addresses no target must not carry a name, including a method
  this server does not know.

### Work Package 3: List Results Say How Long They May Be Cached

Goal: a client can cache a catalog instead of re-fetching it on every reconnect.

- `tools/list`, `prompts/list`, `resources/list`, `resources/templates/list` and
  `resources/read` carry `ttlMs` and `cacheScope`, configurable on the handler.
  The scope defaults to `private`, because a catalog may be
  authorization-filtered.

### Work Package 4: Server-Initiated Input Travels As A Multi Round-Trip Request

Goal: a tool that needs a confirmation mid-call works without an open stream.

- A tool returns `interaction.InputRequired` with the questions it needs answered
  and the state it wants echoed; the transport answers `resultType:
  "input_required"` with `inputRequests` and an opaque `requestState`, and the
  caller repeats the call with `inputResponses`. The tool runs again from the top
  with the answers in its context, so it reads as a guard rather than as a
  suspended call: nothing is held open between rounds.
- An unfinished call is its own outcome, not a failure: no error event, and the
  log line says the call needs input rather than that it failed.
- A question the caller never declared it can answer is `-32021` naming the
  capability, because asking anyway hangs a client with no code for it.
- The state comes back from the caller, so `RequestStateKey` authenticates it:
  with a key, an edited or replayed state is refused before a tool sees it, and
  `RequestStateTTL` bounds the window. Without a key it is accepted as returned,
  which the documentation says plainly rather than implying the server remembers.
- Sampling is deprecated in this revision and stays on 2025-06-18, where
  `SendSamplingRequest` keeps working over the session stream.

Acceptance:

```bash
go test ./interaction/... -count=1
```

### Work Package 5: Authorization And Extensions Are Named Surfaces

Goal: the hardened authorization model and the extension framework are covered
the way the rest of the protocol is.

- The revision hardens authorization across six SEPs; each rule v2 implements is
  declared where it is implemented, like every other protocol promise.
- Which callers may reach which methods is a deployment property, so v2 ships the
  seam and no policy: `mcp.MethodAuthorizer` is asked about every request on both
  revisions with its method, target, header and the context it is served under,
  and a handler without one serves every implemented method. What the framework
  decides is the shape of the question and the shape of a refusal, not the answer.
- The policy reads the principal from the context, so the framework holds no
  opinion about how an identity is represented, and takes none from the request
  body: a subject a request asserts about itself stays a claim. That is the
  property most worth a gate.
- Extensions are versioned in the specification now. What v2 accepts and what it
  ignores is stated: a deployment declares its own with `mcp.Extension` and
  `RegisterExtension`, nothing is declared until something is registered, an
  unregistered extension's methods stay -32601 so a client falls back to core
  protocol, and an unrecognised `_meta` key is carried to the implementation
  rather than refused. The framework implements no extension itself, including the
  official ones: the `io.modelcontextprotocol/` namespace is refused to an
  application precisely so that claim stays true.

Acceptance:

```bash
go test ./interaction/... -count=1
go -C ./tools test -run TestStableProtocolBehaviour . -count=1
```

### Completion Definition / 完成定义

Milestone 10 is complete when a client on either revision is served by tests that
fail if it stops being, each new behaviour is declared beside the code that keeps
it, and `RELEASE.md` still names a gate for every contract surface.

## Milestone 11 (Complete): A Process That Stops On Purpose / 有意为之的停止

Goal: stopping is a sequence with a declared order and a declared end, not a
race between a cancelled context and whatever was still running.

What exists today is a `Host` that, on cancellation, immediately tears components
down in reverse attachment order under one shared deadline. That leaves three
things unanswered, and each one is visible to a user as a failed request:

- readiness still reports ready while teardown runs, so a load balancer and a
  service registry keep sending work to a process that is closing;
- a long-lived response — SSE, a streaming handler — is never told the process is
  stopping, so `http.Server.Shutdown` waits for the whole budget and then returns
  a deadline error with the connection still open;
- one slow component can spend the entire budget, and the components after it get
  a context that is already expired.

### Work Package 1: Draining Is A State, Not A Moment

Goal: a process announces that it is going away before it goes away.

- `Host.Run` enters a drain phase before teardown: readiness starts failing, and
  only after a configurable drain delay do components stop. Zero delay keeps
  today's behaviour, so nothing changes for an assembly that does not ask for it.
- Draining is observable, not guessed: readiness reports why it is failing, and a
  component that wants to stop accepting its own work implements the seam and is
  told. The framework does not decide what draining means for someone else's
  component.
- Liveness stays true while draining. A process that is finishing in-flight work
  is not a process that should be killed.

Acceptance:

```bash
go test ./kit/... ./health/... -count=1
```

### Work Package 2: A Grace Period That Ends

Goal: shutdown finishes, and says what it had to interrupt.

- A long-lived request learns that the process is stopping through its own
  context, so a stream can end itself rather than be cut.
- When the graceful attempt runs out of budget, the server closes what is left
  instead of returning while it is still open — and reports what was interrupted.
- Each component's shutdown budget is stated. A slow component may not silently
  consume the budget of the components behind it.

Acceptance:

```bash
go test ./kit/... -count=1
go test -race ./kit/... -count=1
```

### Work Package 3: Deregistration Comes First

Goal: an instance leaves discovery before it stops answering, not after.

- The order between discovery deregistration, readiness failure, and closing
  listeners is declared and tested rather than left to attachment order.
- What a registrar cannot guarantee is stated too: a registry entry that a peer
  has already cached outlives the deregistration, which is what the drain delay
  is for.

Acceptance:

```bash
go test ./kit/... -count=1
```

### Work Package 4: The Generated Service Stops Correctly Too

Goal: the generated entry point uses the same sequence, and its knobs are
documented configuration rather than constants in a template.

- Drain delay and shutdown timeout are validated configuration keys with
  documented precedence, like every other generated key.
- `PRODUCTION.md` states the operational contract: what a rolling deploy should
  set, and what happens to streams that outlive the budget.

Acceptance:

```bash
go -C ./tools test -run TestGeneratedConfigKeysAreDocumented . -count=1
go -C ./tools test -run TestMicrogenConfigIntegration . -count=1
```

### Completion Definition / 完成定义

Milestone 11 is complete when the shutdown sequence is declared beside the code
that keeps it, a test fails if any step moves out of order, and no shutdown path
can return while a connection it owns is still open.

## Milestone 12 (Complete): Numbers Someone Can Act On / 能拿来做判断的数字

Goal: an operator can scrape a v2 service and get the same numbers, under the same
names, as every other v2 service — without the framework choosing a metrics client
for the application.

v2 can already push telemetry: `observability/otel` assembles in one call and the
instrument names follow the semantic conventions. What it cannot do is answer a
scrape, which is the model most deployments actually run. Every application
therefore wires its own exporter, names its own series, and picks its own labels,
and two services in the same repository end up disagreeing about what
"request duration" means. The numbers themselves already exist — `endpoint.Metrics`
and `endpoint.Recorder` collect them — so this milestone is about exposure and
naming, not measurement.

### Work Package 1: A Scrape Surface With No Client Library In Core

Goal: a metrics endpoint any assembly can mount, without the core dependency path
gaining a metrics client.

- The exposition is rendered by v2 itself, in `observability/metrics`, from the
  numbers `endpoint.Metrics` already holds. No metrics client appears anywhere in
  the module, so there is nothing for a `tools` gate to keep out of core; the gate
  instead pins the package to `endpoint` and standard library only. An application
  that wants histograms, exemplars or its own registry implements
  `endpoint.Recorder` against its own library — the seam that predates this one.
- Mounting is the application's decision. `kit` does not import the package and the
  dependency gate keeps it that way, so the route, the listener it lives on, and
  whether it exists at all stay with the deployment. A gRPC-only assembly mounts
  the same `http.Handler` on its admin mux.

### Work Package 2: One Set Of Numbers, Two Ways Out

Goal: pull and push cannot disagree.

- Whatever the scrape surface reports is derived from the same
  `endpoint.Recorder` data the OpenTelemetry adapter reports, so a dashboard built
  on one matches an alert built on the other.
- Where the two models genuinely differ — a counter reset on restart, a histogram
  bucket layout — the difference is stated rather than smoothed over.

Acceptance:

```bash
go test ./observability/... -count=1
```

### Work Package 3: Cardinality Is Bounded And Declared

Goal: a metrics endpoint cannot take down the system scraping it.

- Labels come from the matched route pattern, never the raw path, and the set of
  label values a series can take is bounded by something the server controls.
- The bound is declared beside the code and covered by a test that fails if an
  unbounded value reaches a label.

Acceptance:

```bash
go test ./kit/ -run TestSeriesCountIsBounded -count=1
```

### Work Package 4: On By Configuration, Not By Surprise

Goal: a generated service exposes metrics because someone asked.

- A documented, validated configuration key turns the endpoint on and sets its
  path; it is off by default, because a metrics endpoint on a public listener is a
  disclosure decision the deployment makes.
- `PRODUCTION.md` states what to scrape, at what interval, and what the series
  mean.
- Constraint found while wiring this, which decides the shape of the change: the
  generated route registrars take `*http.ServeMux` and register their own patterns,
  and `httpserver.RecordingMiddleware` reads the route from `http.Request.Pattern`
  — which only the handler a mux dispatched to can see. Recording therefore has to
  be installed where the patterns are known, inside the registrars, so
  `registerRoutes` and every generated registrar take a registrar interface instead
  of the concrete mux. Mounting an exposition over a collector nothing feeds is not
  an acceptable intermediate step: it reports zeros, which reads as a service with
  no traffic, and `kit` already refuses that assembly.
- The seam that migration needs now exists: `httpserver.RouteRegistrar` and
  `httpserver.DecorateRoutes`. The generated signatures take it —
  `RegisterHTTPRoutes`, `httpRegistrars`, `registerRoutes` — and `cmd/main.go` and
  `cmd/custom_routes.go`, which are written once and never overwritten, keep
  compiling because `*http.ServeMux` satisfies the interface. No manifest migration
  was needed.

Acceptance:

```bash
go test ./cmd/microgen/... -count=1
go -C ./tools test -run TestMicrogen . -count=1
go -C ./tools test -run TestGeneratedConfigKeysAreDocumented . -count=1
```

### Completion Definition / 完成定义

Milestone 12 is complete when a scrape of a generated service returns series whose
names and labels are declared beside the code that emits them, the dependency gates
still keep the metrics client out of core, and a test fails if pull and push report
different numbers for the same request.

## Milestone 13 (Complete): A Listener You Can Put On The Internet / 能直接放到公网上的监听

Goal: a v2 service can terminate TLS itself, and everything it does or refuses to
do about protocol negotiation and hijacked connections is written down.

Today `ServeTLS`, `tls.Config`, `ListenAndServeTLS`, and `h2c` appear nowhere in the
library or in generated code. Every deployment therefore terminates TLS somewhere
else — a sidecar, an ingress, a load balancer — which is a reasonable default and an
undeclared one. Undeclared is the part that matters: it is how a service reaches
production with an assumption in place of a decision, and how the first
"why is HTTP/2 not working" is discovered by a client rather than by a reader.

### Work Package 1: TLS Is Configuration, And Its Absence Is A Statement

Goal: terminating in-process is possible, and not terminating is a documented
position rather than a gap.

- A certificate and key, or a `*tls.Config` the deployment built, are options on the
  serving component. Everything beyond a stated minimum version is the deployment's:
  cipher suites, client authentication, and rotation are policy, and policy belongs
  where the compliance requirement is.
- A certificate that cannot be loaded fails construction, synchronously, with the
  path in the error — not at the first handshake, where the failure is a client's
  problem to report. Construction rather than `Start` because the pair can be read
  before anything is listening, and the earliest honest failure is the best one.
- Plaintext remains the default, and the documentation says why and what it assumes
  about the network the service is on.

Acceptance:

```bash
go test ./kit/ -run 'TestTLS|TestNoTLS' -count=1
```

### Work Package 2: Protocol Negotiation Is Stated, Not Assumed

Goal: nobody has to run a packet capture to learn which protocol they got.

- Over TLS, HTTP/2 arrives through ALPN, which changes how streaming behaves. What
  that means for SSE and for the streaming MCP transport is written where those
  features are documented.
- Cleartext HTTP/2 is not enabled, and the reason is stated: it needs either prior
  knowledge or an upgrade exchange, and in the deployments that want it the proxy in
  front already owns that decision.

Acceptance:

```bash
go test ./kit/ -run 'TestSSEStillStreamsOverHTTP2|TestPlaintextListenerSpeaksHTTP11' -count=1
```

Shipped as `kit.streaming-survives-http2` and `kit.no-cleartext-http2`. What the
prose adds beyond the two tests: over h2 there is no 101 Switching Protocols at all,
so a hijack-based upgrade only works on the plaintext path — which is the reason the
h2c decision and the hijack decision are the same decision.

### Work Package 3: A Hijacked Connection Is Not Drained

Goal: the shutdown sequence tells the truth about what it cannot end.

- `http.Server.Shutdown` does not wait for a hijacked connection, and closing the
  listener does not close one. A WebSocket or any other upgraded connection is
  therefore outside the grace period the shutdown sequence promises, and that has to
  be declared beside the code that promises it — Milestone 11 said no shutdown path
  returns while a connection it owns is still open, and a hijacked connection is
  precisely one it no longer owns.
- The seam is the handler that upgraded: it gets the stopping signal like any other,
  and ending the upgraded connection is its job. A test pins that the signal reaches
  it.

Acceptance:

```bash
go test ./kit/ -run 'TestHijackedConnectionOutlivesShutdown|TestUpgradedHandlerIsToldTheProcessIsStopping' -count=1
```

Shipped as `kit.hijacked-connections-are-not-drained`. The test asserts the
uncomfortable half too: after `Shutdown` returns nil, the hijacked connection still
carries bytes. That is the fact a reader needs, not the one that flatters the
framework.

### Work Package 4: The Generated Service And The Operational Contract

Goal: a generated service can serve TLS by configuration, and `PRODUCTION.md` says
when it should.

- Certificate and key are documented, validated configuration keys, off by default.
- `PRODUCTION.md` states when in-process termination is the right choice and when a
  proxy is, and what each implies for readiness, drain, and upgraded connections.

Acceptance:

```bash
go -C ./tools test . -run 'TestMicrogenConfigIntegration|TestGeneratedConfigKeysAreDocumented' -count=1
```

Shipped as `server.tls_cert_file` / `server.tls_key_file` (`APP_TLS_CERT_FILE`,
`APP_TLS_KEY_FILE`). Two decisions worth keeping: half a pair fails validation rather
than silently serving plaintext, and the startup banner reports the scheme it is
actually serving — a banner that says `http://` for a TLS listener is a wrong answer
to the first question anybody asks it. `PRODUCTION.md` adds the operational
consequences the roadmap did not list: certificate files are read once, so rotation
means a restart unless the deployment supplies `GetCertificate`, and a health probe
left on `http` against a TLS port reads as an unhealthy instance.

### Completion Definition / 完成定义

Milestone 13 is complete when a service can serve TLS from configuration, a bad
certificate fails at startup with its path, the negotiation and hijack limits are
declared beside the code that has them, and a test fails if the stopping signal stops
reaching an upgraded handler.

## Milestone 14 (Complete): Two Transports, One Contract / 两个传输，一套契约

Goal: everything operational v2 promises about an HTTP listener is answered the same
way by a gRPC one, or the difference is declared.

Thirteen milestones were spent making the HTTP surface honest. The gRPC surface came
along for some of it and not the rest, and the gap is not in the RPC layer —
`integrations/grpc` has classified errors, metadata hooks, and traceparent
propagation — but in the operational contract around it:

- `kit/grpc.Component` does not implement `kit.Draining`, so `Host.Drain` skips it.
  The announcement Milestone 11 built stops at the HTTP boundary.
- `kit.Stopping` returns nil outside a kit HTTP component, so a long-lived gRPC
  stream has no in-band way to learn the process is going away. Milestone 11's
  answer to "a stream cannot be drained by asking politely" does not exist here.
- There is no gRPC counterpart to `httpserver.Recorder` and no
  `metrics.GRPCRecorder`, so Milestone 12's scrape reports nothing about RPC
  traffic. A gRPC-only service exposes an endpoint that says almost nothing.
- Generated code is worse than the library: `grpc.NewServer()` with no options, no
  TLS credentials, no health server, no interceptors, and a `GracefulStop` that
  shares the HTTP deadline. Milestone 13's certificate keys reach the HTTP listener
  only, so a generated gRPC port is always plaintext.

This is the kind of asymmetry a framework accumulates by shipping one transport
first, and the kind a reader discovers in production. Parity here does not mean
"the same code" — gRPC has no hijacking, no ALPN question, no route patterns — it
means the same questions have answers.

### Work Package 1: The Announcement Reaches Both Transports

Goal: draining a Host tells every server, not just the HTTP one.

- `kit/grpc.Component` implements `kit.Draining`: at the announcement it stops
  accepting new work in whatever way gRPC allows, and its readiness starts failing
  before `Shutdown` closes anything — the order Milestone 11 declared.
- The stopping signal becomes transport-neutral. A gRPC handler, especially a
  streaming one, learns from its own context that the process is going away, by the
  same call an HTTP handler uses. Where the mechanism cannot be identical, the
  behaviour is: `Stopping` on a context that no component owns still blocks forever
  rather than firing, so a `select` written once is correct in both places.
- What gRPC cannot promise is declared beside the code, the way the hijack limit is:
  `GracefulStop` waits for in-flight RPCs but a stream that never returns holds the
  process until the budget runs out, and then it is stopped underneath.

Acceptance:

```bash
go test ./kit/grpc/ -run 'TestDrain|TestShutdown' -count=1
```

Shipped as `grpc.drains` and `grpc.shutdown-ends`, with `kit.WithStopping` exported as
the seam any transport uses to carry the signal. Two decisions worth keeping: draining
does not start refusing calls — `UNAVAILABLE` during the drain delay only makes a
client retry at an instance the routing layer has not stopped choosing yet — and the
interceptors are installed ahead of the caller's own options, so a handler cannot end
up without the signal by adding one.

The work package's own wording about `GracefulStop` turned out to be wrong, and CI
found it: that call holds the server's mutex while waiting for handlers, and `Stop`
needs the same mutex, so the pair deadlocks instead of timing out — a bounded stop
built on it hangs the process. `Shutdown` counts calls through its own interceptors,
closes the listener, waits, and then closes the transports, reporting
`kit.ErrShutdownIncomplete` with the count. The generated entry point had the same
pattern and now fires the hard stop without waiting on it.

### Work Package 2: A Scrape Says Something About RPCs

Goal: Milestone 12's numbers exist for gRPC, under the same names and with the same
cardinality promise.

- An observation contract for the gRPC server mirroring `httpserver.Recorder`:
  method, outcome, duration — reported where the full method name is in scope.
- `metrics.GRPCRecorder` bridges it to `endpoint.Metrics`, living beside
  `metrics.HTTPRecorder` so the dependency gates stay as they are: the RPC transport
  does not learn about metrics, and core does not learn about either.
- The operation label is the full method name, which is bounded by the service
  definition — the same reasoning that made route patterns the HTTP label. An
  unrecognised method is not recorded rather than recorded as an empty series.

Acceptance:

```bash
go test ./integrations/grpc/server/ ./observability/metrics/grpc/ -count=1
```

Shipped as `grpc.recording-method-label` and `metrics.grpc-bridge-error-class`. Two
decisions the work package did not anticipate: a stream carries `Stream: true` and is
recorded only when it ends, because a lifetime and a latency should not share a mean;
and the bridge is a package of its own with its own dependency gate, because putting
it in `observability/metrics` would have made every HTTP-only service that wants a
scrape endpoint depend on the gRPC libraries.

### Work Package 3: The Generated gRPC Listener Is A Real Listener

Goal: a generated service's second port is configured, secured, and observable like
its first.

- TLS credentials from the certificate keys Milestone 13 added, so one pair secures
  both listeners, and a mismatch still fails startup with its path.
- The health service registered, reflecting the same readiness state the HTTP probes
  report, so a gRPC-only deployment has something to point a probe at.
- The traceparent interceptors and the metrics recorder installed, and the drain
  delay applied before `GracefulStop` rather than after readiness alone.
- Each server gets its own share of the shutdown budget instead of racing for one
  deadline — the rule `Host.shutdownLifecycles` already follows.

Acceptance:

```bash
go -C ./tools test . -run 'TestMicrogen' -count=1
```

Shipped in `main.tmpl`. The health service is grpc-go's own `health.NewServer`
rather than a hand-written one: `Shutdown()` already means "report NOT_SERVING for
everything", which is exactly the drain announcement, and a generated file is the
wrong place to reimplement a protocol service. Readiness therefore fails on both
transports at the same moment rather than only on `/readyz`.

### Work Package 4: The Difference Is Declared, Not Discovered

Goal: a reader can see which operational surfaces each transport answers without
reading both implementations.

- One table in the documentation: readiness, drain announcement, stopping signal,
  shutdown budget, TLS, metrics, tracing — with the honest entry where a transport
  cannot answer, and why.
- A gate that fails when a lifecycle contract is added to one component and not the
  other, so the next asymmetry is a test failure rather than a discovery.

Acceptance:

```bash
go test ./kit/ -run TestEveryKitContractIsClassifiedForBothTransports -count=1
```

Shipped as `kit/transport_parity_test.go` and a table in `docs/lifecycle*.md`. The
gate has two halves, because one alone would not hold: compile-time assertions fail
when a contract is dropped from either transport, and the test fails when `kit`
declares an exported interface nobody has classified — with the reason recorded for
each "no", since a `false` there is a decision and not an omission.

### Completion Definition / 完成定义

Milestone 14 is complete when draining a Host announces to both servers, a gRPC
stream can end itself on the stopping signal, a scrape of a gRPC-only service
reports per-method series, a generated service serves TLS and health on its gRPC
port, and a test fails if one transport gains a lifecycle contract the other lacks.

## Milestone 15 (Complete): Rotation Without A Restart / 不重启的轮换

Goal: the things a running service is handed — certificates first — can be replaced
while it serves, and what cannot be replaced is named.

Milestone 13 shipped in-process TLS and then wrote the gap down itself:
`PRODUCTION.md` says certificate files are read once, so rotation means a restart.
That sentence is honest and the behaviour is poor. A certificate expires on a
schedule; the platform that renews it — a cert-manager secret, a Vault agent, an
operator's cron — replaces files under a running process and expects the process to
notice. "Restart to pick it up" turns a routine renewal into a deploy, and a missed
renewal into an outage.

The framework's part is the seam and the ordering guarantees around it. When to look
again is a deployment's decision: this package does not watch the filesystem, poll a
timer, or install a signal handler, because each of those is a policy some deployment
would have to work around.

### Work Package 1: A Certificate The Process Can Replace

Goal: rotation is possible without a restart, and a bad rotation is not an outage.

- A certificate source is asked on every handshake, so nothing about the certificate
  is captured when the listener starts. Where it comes from is the deployment's:
  a file, a secret manager, an ACME client, a per-name map for SNI.
- A file-backed source that reloads on demand, because that is what a mounted secret
  needs, and it still fails startup with the path when the pair is unreadable — the
  promise Milestone 13 made.
- A failed reload keeps the certificate already being served and returns the error. A
  half-written secret should cost a log line, not the listener.

Acceptance:

```bash
go test ./kit/ -run 'TestCertificate|TestWithTLSCertificateSource' -count=1
```

Shipped as `kit.CertificateSource`, `kit.WithTLSCertificateSource`,
`kit.CertificateFiles` and `kit.CertificateSourceFunc`, declaring
`kit.tls-certificate-per-handshake` and `kit.tls-reload-keeps-serving`. One decision
worth recording: `CertificateFiles.Certificate` never touches the filesystem, so the
handshake path cannot be slowed or failed by disk — the test asserts that replacing
the files changes nothing until `Reload` is called.

### Work Package 2: The Generated Service Rotates On A Signal It Documents

Goal: a generated service picks up a renewed certificate without a deploy.

- The generated entry point reloads its certificate when the process is asked to, on
  a signal that is documented rather than guessed, and logs the paths and the outcome.
  A failed reload is logged and the service keeps serving.
- `PRODUCTION.md` stops saying rotation needs a restart and says what it does need,
  including what a proxy-terminated deployment does instead.

Acceptance:

```bash
go test ./cmd/microgen/... -count=1
```

Shipped in `main.tmpl`: the generated entry point serves its certificate through a
per-handshake source of its own and re-reads on `SIGHUP`. Two decisions worth
recording. The signal is `SIGHUP` because it is what an operator already reaches for
and what a renewal job can send — a timer would have the process guessing, and a
filesystem watch would add a dependency and a policy to a file the user owns. And the
generated code carries its own small `certificateFiles` rather than importing
`kit.CertificateFiles`: the generated `main` does not otherwise depend on `kit`, and
pulling that package in for twenty lines would drag its dependency closure into every
generated binary.

### Work Package 3: What Cannot Be Rotated Is Named

Goal: nobody discovers the limit by trying it.

- The listener address, the protocol options, and the cipher policy are fixed when the
  listener starts. A reader is told which of the things they configured are read once
  and which are read again, rather than inferring it from a table of options.

Shipped as a section in `docs/configuration*.md`: what is read again on a trigger you
choose (the certificate), what is read once at construction (the rest of the
`tls.Config`), what is read once at `Start` (address, timeouts, routes, probe paths),
and what is read once per process (the config file and environment). This work package
is prose by nature — there is no behaviour to gate that is not already gated — so the
honest deliverable is the list, ending where it should: changing anything else means a
new listener, which means the rolling restart the drain sequence exists to make
uneventful.

### Completion Definition / 完成定义

Milestone 15 is complete when a service can be handed a renewed certificate without
restarting, a broken renewal leaves the listener serving the previous one, a generated
service does the same on a documented signal, and the documentation lists what is
fixed at startup.

## Milestone 16 (Complete): A Health Check A Client Can Follow / 客户端能跟住的健康检查

Goal: correct a declaration that was true of two tools and false of the library, and
make the thing it excused work.

Milestone 14's parity table said `Watch` was not implemented, because "the tools that
orchestrate on gRPC health call `Check`". That is true of `grpc_health_probe` and of
Kubernetes' native gRPC probe. It is false of the consumer that matters most:
grpc-go's own client-side health checking — the one a service config turns on with
`healthCheckConfig` — calls `Watch` (`health/client.go`, `healthCheckMethod`). When it
receives `UNIMPLEMENTED` it marks the connection `Ready` and stops asking, so the
drain announcement Milestone 11 built never reached the clients that were watching for
it. A declaration with a hole that shape is worse than no declaration: it reads as a
decision.

### Work Package 1: Watch Streams What Check Answers

- `Watch` sends the current serving status immediately, then one message per change,
  from the same probe registry `Check` evaluates. A named service still gets
  `NotFound`, because the registry describes the process.
- The readiness checks are evaluated once per interval and shared by every watcher.
  Evaluating them per watcher per message would let a fleet of clients turn a database
  ping into load, which is how a health check becomes an outage.
- The poller stops when the last watcher leaves, so a service nobody watches costs
  nothing.

Acceptance:

```bash
go test ./kit/grpc/ -run TestHealthWatch -count=1
```

Shipped as `grpc.health-watch`, with `kit/grpc.HealthWatchInterval` naming the
resolution of the stream — a second, and the reason it is a constant rather than an
option is that the component registers the health service itself, so there is nothing
useful to hand a caller who wants a different one. A deployment that needs its own
health service builds its own `grpc.Server`; that limit is now the interesting one, and
it is stated here rather than discovered.

### Completion Definition / 完成定义

Milestone 16 is complete when a gRPC client with health checking enabled learns that an
instance has started draining, the parity table says so, and nobody is told that
`Watch` is unimplemented.

## Milestone 17 (Complete): Defaults That Are Not Wrong / 不错的默认值

Goal: where this framework hands a service something without being asked, the thing it
hands over should not be a liability. Where it declines to decide, it should say so at
the place a reader will look.

This milestone comes out of a global audit rather than a feature idea. Sixteen
milestones were spent pinning behaviour, and the audit's finding is that the remaining
weakness is not missing features — it is the handful of places where a default arrives
by inheritance from the standard library, or where a trap is reachable through the
supported API. Those are worth more than another capability.

What the audit confirmed as deliberate and stated, and therefore not work:
background jobs (`PRODUCTION.md` says the runner is a sketch to write, not a package
this framework ships, and gives the four rules), rate limiter implementations
(`endpoint/rate_limit.go` says the contract is the framework's and the token bucket is
the application's — the *implementation* being the application's is right; the contract
being unable to name a key was not, see Work Package 4), and every optional provider
staying off the core dependency path.

### Work Package 1: The Outbound Client Is Ours, Not The Standard Library's

- A client built by `transport/http/client` no longer uses `http.DefaultClient`. That
  variable is reachable by every library in the process, so somebody else's timeout or
  transport swap could change these calls; and `http.DefaultTransport` allows two idle
  connections per host, which is right for a one-shot tool and wrong for a service
  calling one upstream continuously.
- The pool is sized for a service, the dial and handshake are bounded, and
  `NewTransport` is exported so a deployment that needs a proxy or a `tls.Config` starts
  from these defaults rather than from the shared one.
- No `Timeout` is invented. A deadline belongs to the call, and one set here would cap
  every call in the process at a number the framework chose, invisibly from the call
  site. `PRODUCTION.md` says both halves.

Acceptance:

```bash
go test ./transport/http/client/ -count=1
```

Shipped as `httpclient.pool-is-ours` and `httpclient.no-invented-deadline`.

### Work Package 2: The Per-Route Body Limit Is Not A Trap

- `kit.WithJSONMaxBodyBytes` is component-wide, and the transport layer already
  supports per-route limits. Today a service that needs one route to accept a larger
  body must register a raw handler through `Handle`, which silently skips the endpoint
  middleware and the recorders that `HandleJSONTyped` installs. Losing observability as
  a side effect of setting a body size is the kind of trap this project otherwise
  refuses to leave lying around.
- The fix is a per-route way to say it that keeps the wiring, and a documented row in
  the customization table next to the middleware scopes.
- `kit.HandleJSONEndpointWithBodyLimit` and `kit.HandleJSONTypedWithBodyLimit` take the
  same registration path as their component-limit counterparts — endpoint middleware,
  recorders, JSON server options, HTTP context — and differ only in the limit. A limit of
  zero or less panics rather than meaning "unbounded", because the component-wide setting
  is the only place a body size is allowed to be absent.

Acceptance:

```bash
go test ./kit/ -run TestPerRouteBodyLimit -count=1
```

Shipped as `kit.per-route-body-limit`.

### Work Package 3: The Error Envelope Says What It Is

- `transport/http/server.ErrorResponse` is a bespoke three-field envelope, declared
  stable. RFC 9457 `application/problem+json` exists and interoperates; the framework
  should either offer it as an encoder a deployment can install, or state why it does
  not. Field-level validation detail is the concrete loss today: `endpoint.ValidationError`
  carries `[]FieldError` and the encoder flattens it to one message.
- Decision: offer it, do not adopt it. `ProblemJSONErrorEncoder` writes the document with
  the same status, redaction, headers and Retry-After as the envelope, keeps `code` as an
  extension member so a client switching on it keeps working, and lists each invalid field
  under `errors`. `ProblemFromError` and `WriteProblemJSON` are exported so a deployment
  with its own kind mapper or extra members composes rather than reimplements.
- No `type` URI is invented. It names a document somebody has to publish and keep
  resolvable, which makes it a deployment's to supply; `nil` writes `about:blank`, the
  value RFC 9457 prescribes for a problem with no type of its own.

Acceptance:

```bash
go test ./transport/http/server/ -run "TestProblem|TestWriteProblem" -count=1
```

Shipped as `http.problem-media-type`, `http.problem-document-shape`,
`http.problem-redaction` and `http.problem-encoder-parity`.

### Work Package 4: The Limit Can Name Who It Limits

- `endpoint.RateLimiter` is `Allow() bool`. The token bucket being the application's is
  correct and stays; what was wrong is that the contract can only ever express a
  process-wide limit, so per-tenant or per-API-key limiting was not absent but
  *unexpressible*, and one noisy caller could reject everybody. A seam that cannot say the
  thing production needs is a liability handed over, which is this milestone's subject.
- `KeyedRateLimiter` — `AllowKey(ctx, key)` / `WaitKey(ctx, key)` — is a second contract,
  not two more parameters on the first: an unkeyed limiter has no per-key state to consult
  and must not be able to claim otherwise by ignoring an argument. `RateLimitKeyFunc`
  supplies the key, because what identifies a caller is the deployment's to decide.
- An empty key is limited under the empty key. Unidentified callers share one bucket rather
  than each receiving a fresh one or bypassing the limit, so a missing header is not a way
  out; a nil key function panics at assembly for the same reason.
- `KeyedRetryAfterReporter` closes the matching gap in the hint: a per-tenant bucket knows
  its own refill, and the unkeyed reporter could not say so.

Acceptance:

```bash
go test ./endpoint/ -run "Keyed" -count=1
```

Shipped as `endpoint.keyed-rate-limit` and
`endpoint.keyed-rate-limit-shares-one-bucket`.

## Milestone 18 (Complete): Read By Somebody Else / 被别人读

Goal: the framework should hold up when it is read by someone who did not write it.
Where a reader looks for an answer, the answer should be there; where two names
suggest two behaviours, they should have two behaviours.

This milestone comes out of reviewing the framework from five users' points of view:
someone arriving for the first time, someone writing business logic, someone
operating it, someone extending it, and someone writing tests against it. Two
findings ordered the work. The operator's view is the best served and the test
author's the worst — and unlike background jobs or limiter implementations, nothing
ever declared test support out of scope. And the documentation gaps are not in the
prose, which is 42 English documents paired one-to-one with 42 Chinese ones and two
indexes with no dead links; they are in the godoc, where the package comment is
missing or one line long in packages a reader reaches first.

A correction to the audit that opened this milestone: the first pass reported
fourteen packages with "no package-level documentation", which was really "no
`doc.go` and no `README.md`". Most of those packages do carry a package comment in
their primary file — `security`, `health`, `sd/selector` and `sd/feedback` carry
substantial ones. The real gap is narrower and is what the work packages below
describe.

### Work Package 2: The Packages Business Code Imports Read Like Documentation

- `apperror`'s package comment is four lines. It is the first package business code
  imports and the one that decides how every failure reaches a client, so the comment
  should answer what a reader arrives with: which kind to pick, what the empty kind
  does, and where the message goes.
- `security` and `health` already carry substantial comments and need no rewrite.
  Naming them as gaps was the audit's error, not theirs.

Acceptance: `go doc ./apperror` answers which kind to pick, what the empty kind
does, where the message goes, and when to use `WrapCause` over `Wrap`.

### Work Package 1: Time Is A Seam

- `time.Now()` is called directly from production code in
  `transport/http/server/recorder.go`, `access_log.go`, `security/http/csrf.go`,
  `sd/retry/retry.go`, `sd/selector/selector.go`, `endpoint/metrics.go`,
  `endpoint/retry.go` and `interaction/mcp/session.go`. A deployment cannot test a
  backoff interval, a CSRF expiry or a slow-start weight without sleeping in real
  time, and neither can this repository — `sd/selector/slow_start_test.go` alone
  makes seven time calls.
- A `Clock` seam with a nil-means-real-time default, so nothing changes for a
  service that does not care and a test can decide what time it is.
- It lives in `endpoint`, not in a new package, because `endpoint` is the stdlib-only
  bottom layer every other layer may import — the same reason `Recorder` and
  `RateLimiter` live there — and `TestEndpointHasOnlyStandardLibraryImports` means an
  interface over `time` is the only shape it could have taken.
- `ManualClock` is exported beside the contract rather than hidden in a test-only
  package. A seam nobody can reach is not a seam: an application accepts the same
  clock this framework's components accept.
- `security/http` declares its clock structurally instead of importing `endpoint`,
  because its dependency gate allows nothing from this module and that is deliberate —
  it is middleware you drop into any `net/http` stack. Any value with a `Now` method
  fits, so `ManualClock` still works there.
- Stated exclusion: the clock does not decide how long real work took.
  `Observation.Duration` and logged request durations stay measured. A clock that could
  shorten them would make a recorder report something untrue about the system, and the
  duration measurement sites — `transport/http/server/recorder.go`, `access_log.go`,
  the `slog`/`zap`/gRPC recorders, `interaction/runtime.go` — keep `time.Since` for
  that reason rather than by omission.
- Deferred with a reason: `sd/retry`'s backoff wait. Its constructors are positional
  (`Retry`, `WithCallback`, `WithClassifier`), so a clock there needs an options shape
  the package does not have yet; adding a fourth positional constructor would be worse
  than the gap. It belongs with a wider `sd/retry` options pass.

Acceptance:

```bash
go test ./endpoint/ -run "Clock|Manual|Retry|Metrics" -count=1
go test ./security/http/ -run TestCSRFRejectsATokenExpired -count=1
```

Shipped as `endpoint.clock-nil-is-wall-clock`,
`endpoint.manual-clock-advance-fires-due-timers`, `endpoint.retry-clock`,
`endpoint.metrics-clock` and `security.csrf-clock`.

### Work Package 2: The Packages Business Code Imports Have Godoc

- `apperror`, `health` and `security` have neither `doc.go` nor `README.md`.
  `apperror` is the first package business code imports and its godoc front page is
  empty.

### Work Package 3: The Extension Points Read Like Documentation

- `sd/endpointer` and `sd/instance` have no package comment at all, and
  `sd/balancer`, `sd/retry` and `sd/client` have one line each. `sd/README.md` covers
  the behaviour, but somebody writing a custom balancer or reading why the instance
  cache exists is looking at godoc.
- `sd/selector` and `sd/feedback` already explain themselves and stay as they are.

Acceptance: `go doc ./sd/endpointer ./sd/instance ./sd/balancer ./sd/retry ./sd/client`
each explains what the package is for and how it differs from its neighbour.

### Work Package 4: A Registration Name Describes What It Registers

- `kit` exports eleven registration entry points whose differences reduce to two
  axes: whether the endpoint middleware chain and recorders run, and which body
  limit applies. Two names mislead. `HandleSSE` (`kit/sse.go:15`) is a one-line
  delegate to `Handle` and carries none of the SSE behaviour its name implies, so a
  reader who follows the name from `HandleSSETyped` silently loses the chain. `JSON`
  and `JSONTyped` return an unregistered handler and differ from `HandleJSON*` by
  one verb.
- The fix is for the API to state the distinction itself rather than rely on the
  reader having found the customization table first.
- `HandleSSE` is deprecated rather than removed. `Handle` already is the escape
  hatch and already says what it skips, so a second name for it was only ever a way
  to arrive there by accident — but `TestAPICompatibilityWithLastRelease` is right
  that a published symbol is a promise, so it stays reachable with a `Deprecated:`
  note that says exactly what it does. That gate is stricter than the pre-freeze
  policy in `RELEASE.md`, and the stricter of the two wins: weakening a gate to let
  one's own change through is the wrong instinct.
- `JSON` and `JSONTyped` are deprecated in favour of `NewJSONHandler` and
  `NewJSONTypedHandler`, matching the `httpserver.NewJSONServer` they wrap, and
  `kit`'s package comment groups all eleven entry points by whether the chain runs.

Acceptance:

```bash
go test ./kit/... -count=1
go vet ./kit/...
```

### Work Package 5: One Index Is Not A Subset Of The Other

- `DOCS_INDEX.md` and `docs/index.md` each omit documents the other lists:
  `tutorial-crud.md` and `examples/README.md` appear only in the book index,
  `ROADMAP.md`, `docs/licenses.md` and `tools/README.md` only in the repository
  index, and `examples/profilesvc/README.md` in neither. Finding a document should
  not depend on which index you opened.

Acceptance: the documentation link and pairing gates in `v2/tools` pass, and every
document listed in one index is listed in the other.

## Milestone 19 (Complete): What The Promise Did Not Cover / promise 没有覆盖到的地方

Goal: where a `// Stable:` marker states a promise, the named test should assert that
promise — not a weaker one, and not a subset of the paths the promise covers.

`tools/protocol_behaviour_test.go:78` already checks that every marker names a test
that exists. It cannot check that the test asserts the promise, and that is where
this milestone's findings come from. Each one is a path nobody walked: the prose was
right, the code was not, and the test agreed with the code.

### Work Package 1: Every Request Means Every Request

- `mcp.method-authorization` said every request reaches the `MethodAuthorizer`.
  `authorizeMethod` had exactly two call sites, both POST. `handleGet` and
  `handleDelete` had none — so a caller holding a session ID could attach to the
  server-initiated SSE stream or terminate a session with the policy never
  consulted. The covering test drove ten stateless POST methods and neither verb.
- `MethodOpenStream` and `MethodDeleteSession` present them under names in
  namespaces no MCP method uses, so a policy decides on them without matching HTTP
  verbs and without a string literal. A refusal is 403: there is no request id to
  answer, the same reason a refused notification is.
- Authorization runs before the session lookup, so a refused caller cannot learn
  which session IDs exist from the difference between 403 and 404.

Acceptance:

```bash
go test ./interaction/mcp/ -run "Authoriz|Transport" -count=1
```

Shipped as `mcp.transport-operation-authorization`.

### Work Package 2: A Stream Cannot Frame Itself

- `http.sse-framing` described one `data:` line per input line. The split was on LF
  alone, while the specification ends a line on CRLF, CR, or LF — so a bare CR stayed
  inside a `data:` line, where a conforming client ends the field and reads the rest
  as a nameless one. The payload arrived truncated. The covering test used `"a\nb"`.
- Data, comment text and event names were unescaped, so each could end its own frame
  and inject an event. Reachable from any handler streaming caller-influenced text.
- Data and comments now stay data and comments. An event name carrying a terminator
  is an error: a field value cannot hold one, so the name is a caller bug, and both
  stripping it and passing it through frame something nobody asked for.

Acceptance:

```bash
go test ./transport/http/server/ -run SSE -count=1
```

Shipped as `http.sse-line-terminators` and `http.sse-no-frame-injection`.

### Work Package 3: A Limit That Runs Before The Cost

- `transport/http/server/multipart.go:71` calls `ParseMultipartForm`, which spills
  parts over `maxMemory` to temp files, and only then compares `fh.Size` against
  `MaxFileBytes`. The effective disk cap per request is `MaxBodyBytes`, not
  `MaxFileBytes`, so a caller can force the larger write and still be answered 413.
- The same marker promises code `request_too_large` for "an over-limit body or file",
  but the file case emits `request_too_large.file`. A client matching the promised
  string misses it.
- Decision: the promise catches up with the behaviour rather than the reverse. Naming
  which limit was hit is more useful than one code for both, so the marker states
  both codes. `request_too_large.file` keeps the prefix, so a client matching by
  prefix already worked.
- Decision on the cost: enforcing per-file limits while streaming needs
  `r.MultipartReader()` and a reimplementation of form assembly, spilling and
  cleanup. That is a large amount of new code guarding a bound the deployment
  already sets — `MaxBodyBytes` caps the temp-file cost, and it is the dial that was
  never described as such. Rejected in favour of stating which field bounds work,
  and declaring what the refusal does clean up.

Acceptance:

```bash
go test ./transport/http/server/ -run Multipart -count=1
```

Shipped as `http.multipart-file-refusal-leaves-nothing-behind`, with
`http.multipart-limits` restated.

### Work Package 4: One Content-Type

- `transport/http/server/error.go` sets `Content-Type` and then `Add`s every header a
  `Headerer` error reports. An error whose `Headers()` includes `Content-Type`
  produces two values and a response no client can interpret. Three call sites.
- The order is the fix: merge what the error asked for, then name the type of the
  body the encoder actually wrote. One helper now backs all three, so they cannot
  drift apart again.

Acceptance:

```bash
go test ./transport/http/server/ -run "ContentType|HeaderOrder" -count=1
```

Shipped as `http.error-single-content-type`.

### Work Package 5: A Bad Cursor Is An Error

- `interaction/mcp/handler.go:566` treats a cursor that is non-numeric, negative or
  past the end as offset 0. The MCP specification requires `-32602`. A client that
  persisted a cursor across a catalogue change is handed page one as though it were
  its page.
- `mcp.error-codes` enumerates the codes the server emits and omits `-32021`, which
  `stateless.go` returns and its own marker promises. Two markers disagree about the
  vocabulary.
- One `listError` now backs all four list methods, so the same bad cursor cannot be
  invalid params on one and an internal error on another.

Acceptance:

```bash
go test ./interaction/mcp/ -run "Cursor|FirstPage" -count=1
```

Shipped as `mcp.invalid-cursor-is-invalid-params`, with `mcp.error-codes` restated.

### Work Package 6: A Marker The Gate Cannot See

- `observability/otel/agreement_test.go:25` declared `otel.metrics-agree-with-the-exposition`,
  and `tools/protocol_behaviour_test.go` skips `_test.go` files — so that promise was
  absent from the reviewed snapshot and outside the freeze gate, while reading in the
  source as though it were covered by both.
- Both halves of the fix, because either alone leaves the hole: the promise moves onto
  `NewMetrics`, where a marker belongs, and `TestBehaviourMarkersLiveInNonTestSource`
  refuses the next one instead of skipping it. Skipping test files is right; skipping
  them silently was not.

Acceptance:

```bash
go test ./... -run TestBehaviourMarkersLiveInNonTestSource -count=1   # in v2/tools
```

### Work Package 7: A Response That Never Arrives

- `interaction/mcp/streamable.go:282` marshalled a tool result with `_` and wrote it
  with `_`. A result that cannot be marshalled — a NaN float, a map with non-string
  keys — sent `data: ` and the client waited for a reply that never came, with
  nothing logged. `writeResponse` had the same shape through `json.NewEncoder`, where
  a failure part-way through had already put bytes on the wire.
- Both now marshal before writing anything, so either the whole response arrives or
  an internal error naming the fault does.

Acceptance:

```bash
go test ./interaction/mcp/ -run TestUnserialisableResult -count=1
```

Shipped as `mcp.response-is-whole-or-an-error`.

## Milestone 20 (Complete): Nothing In A Request Vouches For Itself / 请求里的东西不能为自己作证

Goal: where this framework makes a trust decision, the thing it trusts should not be
something the caller supplied.

### Work Package 1: The Request's Host Is Not A Credential

- `interaction/mcp/streamable.go` allowed an `Origin` equal to `"http://"+r.Host` or
  `"https://"+r.Host`. `Host` comes from the caller, so under DNS rebinding a browser
  sends `Origin: http://evil.example` with `Host: evil.example` and the comparison
  says yes — which is the attack Origin validation exists to stop for a locally bound
  MCP server.
- The audit classified this as "declared, but the consequence was not". The struct
  comment said same-origin requests are always allowed; it said nothing about what
  `Host` is worth. No test exercised the shortcut, which is how it stayed unnoticed —
  making the stricter default cost zero test changes.
- Three options were considered. Tightening unconditionally is safest but breaks
  "start it locally and it works". Documenting only leaves the trap armed. Shipped
  the middle one, because it is the shape this project uses everywhere else: the
  shortcut becomes `TrustRequestHost`, off by default, so trusting `Host` is
  something a deployment signs for. `AllowedOrigins` remains the answer that does not
  depend on a caller-controlled header.
- This is a behaviour change with a migration note in the changelog rather than a
  silent tightening: a deployment that wants the old behaviour sets one field.

Acceptance:

```bash
go test ./interaction/mcp/ -run "Origin|TrustRequestHost" -count=1
```

Shipped as `mcp.request-host-is-not-trusted-by-default`.

## Milestone 21 (Complete): What The Generator Emits Is Part Of The Framework

Goal: the generated service should be as correct as a hand-written one, and a feature
should behave the same when combined as it does alone.

Two audits opened this. The first ever look at `cmd/microgen` — the generator emits a
whole service and had never been reviewed — and a look at feature composition rather
than per-package correctness.

Released as v2.21.0. Four work packages landed: the generated service builds and stops
correctly again, SSE keeps the component's contract and reports its own failures, text
from an IDL is escaped where it becomes code, and the generated document describes the
generated handler. Two gate defects found on the way — a vacuous pass on a tagless
checkout and an out-of-band snapshot refresh — are fixed in the same release.

### Work Package 1: The Generated Service Is Correct Again

- v2.20.0 tightened the MCP Origin check, and no template mentioned `AllowedOrigins`
  or `TrustRequestHost`: zero grep hits across `cmd/microgen`. The generated service
  refused browser clients that used to be served, and offered nothing to point at.
  This one is a regression this repository caused.
- The generated shutdown never called `mcpHandler.Shutdown`, so sessions and their
  goroutines survived it. It now runs before the HTTP shutdown, because a session
  holds an open SSE stream and closing the sessions is what lets `Shutdown` finish.
- The generated success encoder was hand-rolled `json.NewEncoder`. No `Content-Type`
  (so `net/http` answers `text/plain` for JSON, contradicting the OpenAPI document the
  same generator writes), no `StatusCoder`, no `Headerer`, no 204 rule. It now calls
  `server.EncodeJSONResponse`. The unused `encoding/json` import goes with it —
  otherwise the generated code does not compile.
- `--grpc` produced a project that did not build: `go.mod` required neither grpc nor
  protobuf while the generated `main` imports both.
- Deferred with a reason: the GORM driver require. No `gorm.io/driver/*` version
  exists anywhere in this repository, and inventing one turns "missing require" into
  "version does not exist", which is worse. It needs either a pinned driver version
  table or a line in the generator's next-steps output; the latter is preferable and
  belongs with a wider look at what the generator tells the user after it runs.

Acceptance:

```bash
go test ./cmd/microgen/... -count=1
```

### Work Package 2: SSE Keeps The Component's Contract

- `kit.HandleSSETyped` never passed `h.jsonServerOptions` to `NewSSEServerTyped`,
  while `kit/doc.go` — written by this repository in milestone 18 — listed it under
  "the component's JSON server options all apply". A deployment installing
  `ProblemJSONErrorEncoder` component-wide got problem documents on JSON routes and
  plain envelopes on SSE decode failures.
- Two caveats found by the same audit were first written down rather than fixed, and
  are now fixed. A middleware rejection was rendered by a hardcoded
  `JSONErrorEncoder` while the same route's decode failures went through the
  configured one, so one route had two error contracts; the rejection now uses the
  encoder `SSEServer.ErrorEncoder` reports. And the bridge's base endpoint returned
  `nil` unconditionally, so a stream that died mid-way was a success in the metrics;
  `SSEServer.ServeStream` is the seam that was missing — `ServeHTTP` that hands back
  the error that ended the stream — and the bridge returns it to the chain.
- What still cannot follow is the response. Once a stream has answered 200 and
  flushed events, an error can be recorded but not rendered, so the bridge encodes
  only when the stream never started. Telling those two apart is why the base
  endpoint marks a `streamOutcome` in the context instead of guessing from the error.

### Work Package 3: Text From An IDL Is Not Code

- `template_funcs.go` had an `escape` helper that replaced only `"`, not `\` and not
  a newline, and no template used it: the escaping machinery existed, was incomplete,
  and was dead. Nineteen interpolation sites wrote IDL and database text straight
  into Go string literals, struct tags and comments.
- The quote case is the dangerous one because it is *syntactically valid*: the
  generator's `format.Source` check passed `Description: "a "user" record"` through
  to fail in the user's build. A newline aborted generation after earlier files were
  written. A backtick in a column name terminated a struct tag's raw literal.
- Replaced by three helpers that name what they guard: `quote` (`strconv.Quote`,
  which handles quotes, backslashes, newlines and control bytes together — the point
  of using it rather than replacing one character), `comment` (folds every line
  terminator, because `//` ends at the newline and `.proto` output never reaches a
  formatter) and `tag` (removes what a raw string cannot escape).
- The regression test asserts the generated fragment *parses* and that the literal
  still round-trips to the input — valid syntax alone was what let this through.

Acceptance:

```bash
go test ./cmd/microgen/... -count=1
```

### Work Package 4: The Document Describes The Handler

- `additionalProperties` was absent from every message schema while the generated
  decoder sets `DisallowUnknownFields`. Silence means "extra properties are allowed",
  so a client generated from the document could be rejected by the service generated
  beside it. Every message schema now says `false`; `ErrorResponse` deliberately does
  not, because a deployment chooses the error encoder and
  `ProblemJSONErrorEncoder` answers with a wider document.
- Pointer fields lost their nullability three times over: the OpenAPI schema, the
  JSON Schema bundle, and the TypeScript SDK all named the value's type while the Go
  encoder writes `null` for a nil — generated structs carry no `omitempty`. A plain
  type is now a two-element type list, a `$ref` is wrapped in `anyOf` (keywords beside
  a `$ref` are not honoured everywhere), and the TypeScript field is `T | null`.
- `required` is not a defect but a derivation, and it is now written down where a user
  meets it: non-pointer means required, a pointer is how optional is declared, and it
  is a statement about the payload rather than a promise that the server rejects an
  omission. Presence cannot be told from a zero value on a non-pointer field, so a
  missing `count` arrives as `0` — the reason to declare the field as a pointer when
  absence has to be a different answer.

Acceptance:

```bash
go test ./cmd/microgen/... -count=1
go -C ./tools test . -run TestMicrogen -count=1
```

### Still Open From The Same Audits

Recorded with file and line, not yet acted on:

- A live SSE stream burns the HTTP component's whole shutdown share and produces
  `ErrShutdownIncomplete` on any rolling deploy that catches one; `WithTimeout` is
  component-wide with no per-route escape, so hosting a long stream means dropping the
  deadline for the JSON routes beside it. Both are mechanisms the code states; the
  trade-off is not written down.
- The canonical SSE example in `kit/sse.go` selected only on `ctx.Done()` and ignored
  the `Stopping` announcement, contradicting the drain example in `kit/drain.go` — and
  the SSE one is the one an SSE author reads. It now watches both.
- The gate meta-audit found the two vacuous passes now fixed (a tagless checkout, an
  out-of-band snapshot refresh) and one it did not fix in that release: the digest
  storage of `api_surface` and `contract_snapshots/*.sha256`, where nothing forced a
  reviewer to read what changed. Carried into Milestone 22.

## Milestone 22 (Complete): A Gate Whose Failure Is The Review

Goal: when a gate blocks a change, the failure and the stored file together say what
changed. A gate whose diff a person cannot read is a gate whose refresh command is
the only review it will ever get.

No release follows it. Everything here is in the workspace-only `tools` module and in
documentation, so the published module is byte-identical to v2.21.0 and a tag would
say otherwise.

### Work Package 1: The Public API Surface Is A List, Not A Hash

- `api_surface.sha256` held one SHA-256 per package. A failure proved that something
  in some package moved; nothing in it, and nothing in any diff a reviewer read,
  contained the declaration that changed. Refreshing made it green, and refreshing was
  the whole review.
- The reviewed form is now `api_surface.txt`: the declarations themselves, in
  `## <import path>` sections, 2,487 lines for 34 packages — the same shape Go's own
  `api/go1.*.txt` files use, and small enough that `git diff` is the review.
- The failure names the packages whose section differs and quotes the declaration
  lines that appeared and disappeared, capped per package. It deliberately does not
  print the file: two 2,500-line blocks would fail the same way the digest did, from
  the other direction.
- Verified by removing one declaration line from the snapshot and reading the failure:
  it named `apperror` and quoted the one line.

Acceptance:

```bash
go -C ./tools test . -run TestPublicAPISurfaceSnapshot -count=1
```

### Work Package 2: The Generated Contract Is A Golden Tree

- The three `contract_snapshots/*.sha256` files hashed whole generated artefacts —
  manifest, OpenAPI document, JSON Schema bundle, IDL, both SDKs. A failure said that
  one of six documents had changed, and answering "how" meant running the generator
  into a temporary directory by hand. That is how the OpenAPI defects fixed in
  Milestone 21 had to be reviewed: the gate had been catching them for two releases
  without anyone being able to see them.
- The reviewed copy is now the artefacts, under
  `tools/testdata/contract_snapshots/<source>/`, 4,235 lines across the three sources.
  A `files.txt` per source lists what is under review, so an artefact the generator
  stops emitting fails instead of leaving a golden file nobody reads, and a new public
  artefact has to be signed off rather than appearing silently.
- The failure names the artefact and quotes the first differing line with its number,
  then points at the file's own diff. A refresh clears the directory first: a golden
  tree that keeps files the generator no longer writes describes a generator that no
  longer exists.
- Verified by editing one line of the reviewed `openapi.json`: the failure named the
  file, the line number and both sides.

Acceptance:

```bash
go -C ./tools test . -run TestMicrogen -count=1
```

### What The Self-Audit Of This Milestone Found

Two defects in the work above, both found by auditing it rather than by a gate:

- The refresh cleared a source's directory before rebuilding it, and the three
  integration tests that own those directories run in parallel with
  `TestEveryContractSnapshotHasALiveCaller`, which requires each source to have a
  `files.txt`. So `go test ./... -args -update-contract-snapshots` could fail on a
  snapshot it was in the middle of writing correctly. It now writes the index first
  and prunes afterwards, which keeps the stale-file guarantee without the window.
- `AGENTS.md` claimed `make verify` was "exactly what CI runs". It was not: CI also
  runs `releasecheck -check-tags` beside the suites, which the target did not. Rather
  than soften the sentence, the target now runs it, so the claim is true and local
  green cannot be greener than CI.

The audit also checked a real risk and found none: the `race` CI job checks out
without tags, and v2.21.0 made a tagless checkout fail
`TestAPICompatibilityWithLastRelease`. That job runs `go test -race` over the `v2`
module only, never the gate module, so the two do not meet.

## Milestone 23 (Active): The Discovery Subtree Answers For Itself

Goal: `sd` had never been audited. Two audits — the load balancing path
(`balancer`, `selector`, `endpointer`, `instance`) and the resilience path
(`retry`, `feedback`, `health`, `client`) — asked what a deployment loses in each
case, and the answers are being worked through in severity order.

The selection core came out better than the "never audited" framing suggested:
every strategy refuses an empty set with a typed error before any arithmetic, every
published snapshot is built complete and swapped rather than edited in place, and
every goroutine has an owner. The defects are in arithmetic, event ordering and
concurrent read-modify-write.

### Work Package 1: A Retried Call Does Not Lose An Answer It Received

- The loop selected on the call context and on the result channel. With both ready,
  select chooses uniformly, so an attempt that completed as the budget expired had a
  coin-flip chance of being discarded and reported as a deadline error. For a
  non-idempotent request that is unrecoverable: the write happened and the caller
  was told it did not.
- Two changes, because one was not enough. The attempt hands its result to the loop
  *before* the outcome callback runs — deployment code in `Done` was able to widen
  the window arbitrarily — and the loop drains a delivered result before honouring
  the context.
- The budget is also checked before dispatching rather than only after, so a caller
  that has given up, or a backoff timer that returns late, no longer causes one more
  upstream request whose result nothing reads.
- Both fixes carry a `Stable:` marker. The drain deliberately does not: the window it
  covers — an unrelated goroutine cancelling in the same instant as the channel send
  — cannot be produced deterministically by a test, and a promise whose named test
  does not assert its text is the thing this repository spent Milestone 19 removing.

Acceptance:

```bash
go test ./sd/... -count=1
```

### Work Package 2: An Ejection Cap That Holds Under Concurrency

- `Ejector.apply` judged its candidates and checked `MaxEjectionPercent` outside the
  lock, then re-acquired it to apply. The filter sits on the selection path, so it
  runs once per pick and concurrently by construction. Two picks over a four-instance
  pool could each pass a 50% check against a view that excluded the other's decision
  and eject together — 75% of the pool out of service in exactly the situation the cap
  exists to prevent, panic mode never firing, and the caller seeing `ErrNoEndpoints`.
- The same window let two picks eject the *same* instance. Ejections are counted to
  grow the window, so a first offence was recorded as two and the instance stayed out
  for twice its configured base duration, with the doubling ladder ahead of the real
  failure history for the life of the entry.
- Deciding and applying now happen in one critical section. Measurements are read
  before it rather than inside: they come from the `Table`'s own mutex, and
  `Table.Follow` drives `Ejector.Retain`, so reading the table under `e.mu` would
  invert an order the package already relies on. The reset of a returned instance
  still happens before its verdict is read, or the stale measurements that ejected it
  would eject it again on the same call.
- The test needed a barrier, not just goroutines. Its first version used a plain fake
  clock and passed against the defective implementation — four goroutines doing
  microseconds of work do not overlap on demand. Aligning them on the clock call makes
  the double-count fail on the first round.

Acceptance:

```bash
go test ./sd/feedback/ -count=1
```

### Work Package 3: A Flap Is Not A New Instance

- `health.Checker` deleted an absent address's state and recreated it on return with
  `initiallyHealthy` and no recorded failures. A backend it had just taken out of
  service was republished as serving and could not be ejected again for
  `unhealthyThreshold` rounds, so a registry that keeps flapping keeps a dead backend
  in the set for most of its life.
- State now survives one absence and is dropped on the second. The cost is one extra
  generation of stale entries; the alternative — a departure timestamp — needs a clock
  seam the Checker does not have.
- The forgetting half is weaker than it reads, and that is written into the test: a
  source which suppresses an event identical to the one before it, as `instance.Cache`
  does, delivers the second absence only when the set changes again. For a service
  whose set never changes after a departure that leaves one stale map entry per
  departed address. It is memory hygiene rather than correctness, so it is recorded
  here rather than solved with a clock.

Acceptance:

```bash
go test ./sd/health/ -count=1
```

### Work Package 4: A Subscriber Is Never Left Behind The Stored State

- `instance.Cache.Update` stored the new state under its lock and broadcast after
  releasing it. Two concurrent callers could store A then B and deliver B then A, and
  `sendLatest` made it worse: finding the buffer full it drains and rewrites, so the
  older event replaced the newer one. `State` then disagreed with every subscriber
  until the next update, and since an event equal to the stored one is dropped, a
  repeat of the stale list kept the divergence.
- The broadcast happens under the lock now. That is safe rather than a stall risk
  because every channel operation in `sendLatest` has a default case — the property
  that made the unlocked broadcast look necessary in the first place.
- The test it replaced pushed twenty *identical* events and asserted nothing beyond
  "no race / panic". Identical events are dropped by design, so nineteen never reached
  a subscriber and the interleaving the test was named for never happened. The new one
  pushes distinct events and compares what the subscriber holds against `State`; it
  fails at around round 100 of 200 against the unlocked broadcast.
- Worth noting for the next audit: `-race` is silent on this defect. There is no
  unsynchronised access, only a delivery order that does not match the store order. A
  clean race run is not evidence about ordering.

Acceptance:

```bash
go test ./sd/instance/ -count=1
```

### Still Open From The Same Audits

Recorded with file and line, in the order they would be taken:

- Weighted selection reports `ErrNoEndpoints` on integer overflow of the weight total
  (`sd/selector/weighted.go`). Two instances registered with `weight` at `MaxInt`
  wrap the sum negative, the guard fires, and a fully healthy pool becomes
  unroutable — classified as temporary, so callers burn their retry budget first.
- `endpointer.Filter` and `Prefer` accept a nil source and defer the panic to the
  first request (`sd/endpointer/filter.go`), while every sibling constructor
  validates at assembly time.
- `feedback` retains a provider's instance slice without copying
  (`sd/feedback/feedback.go`), at the boundary where `health.accept` copies and says
  why.
- `sd/retry` sleeps and reads the clock directly, in a repository that owns
  `endpoint.Clock` and uses it in `feedback` and in `endpoint.RetryMiddleware`. The
  consequence is not stylistic: nothing asserts the schedule the loop actually
  produces, only `backoff.Next` in isolation.
- `retry.Error.Unwrap` is single-valued, so after a budget expiry the only reachable
  cause is the context error and an upstream `apperror` kind that was known in every
  attempt is invisible to `errors.As`.

## Maintenance Rules / 维护规则

- Update this file only when milestone scope, order, or acceptance criteria
  change.
- Record completed behavior in `CHANGELOG.md`, not as growing status notes here.
- Put concrete usage in `README*` or `MICROGEN.md` and package design in
  `ARCHITECTURE.md`.
- Every active milestone must have focused tests and an end-to-end verification
  path before implementation is considered complete.
