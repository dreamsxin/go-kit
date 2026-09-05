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

## Milestone 9 (Active): A Freeze Worth Declaring / 值得宣布的冻结

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

- The contract snapshots pin four fixed paths, two globs, and an optional
  `idl.go`. `main.go`, `service/`, `transport/`, `config/custom.go`, the generated
  `Makefile` and README are user-owned and unpinned: relocating any of them fails
  nothing. The snapshot covers every generated path the contract promises, or the
  contract narrows to what is covered.
- The snapshots are cut from fixture projects, so they describe the layout for
  those flag combinations only. Which combinations are covered is stated.

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

## Maintenance Rules / 维护规则

- Update this file only when milestone scope, order, or acceptance criteria
  change.
- Record completed behavior in `CHANGELOG.md`, not as growing status notes here.
- Put concrete usage in `README*` or `MICROGEN.md` and package design in
  `ARCHITECTURE.md`.
- Every active milestone must have focused tests and an end-to-end verification
  path before implementation is considered complete.
