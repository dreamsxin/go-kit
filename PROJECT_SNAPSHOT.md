# Project Snapshot

Purpose:
- Give maintainers and AI agents a fast re-entry summary of the current repo state, recent changes, and next recommended work.

Read this when:
- You are resuming work, taking over a refactor thread, or deciding what to do next.

See also:
- [MAINTAINER_GUIDE.md](MAINTAINER_GUIDE.md)
- [PROJECT_WORKFLOW.md](PROJECT_WORKFLOW.md)
- [DOCS_INDEX.md](DOCS_INDEX.md)

This file is the fastest re-entry point for a new maintainer or AI coding session.

Read this first when you need to answer:

- what this repository is
- what changed recently
- what is currently being refactored
- what should happen next
- which commands give the fastest confidence loop

## Product Summary

`go-kit` is a Go microservice framework with three linked responsibilities:

1. Runtime framework packages for transport, endpoint middleware, service discovery, logging, and utilities.
2. `microgen`, a definition-driven generator for service scaffolding from Go IDL, Proto, or database schema.
3. AI-facing skill exposure so generated services can publish callable tool definitions.

The architectural spine is:

`service -> endpoint -> transport`

That layering should remain intact during refactors.

## Current Refactor Status

The repository is in an active structural-cleanup phase.

Current priority summary:

1. keep the new remote-config provider contract stable and decide whether to add explicit CLI/provider surface next
2. keep extend-mode guarantees documented and compatibility-safe
3. continue favoring generator-owned seams and integration-tested user-visible behavior

Recently completed:

- `kit/kit.go` was split into smaller files for service setup, options, HTTP registration, lifecycle, gRPC access, JSON helpers, and request ID handling.
- `FRAMEWORK_ARCHITECTURE.md` was added as the recommended target-architecture baseline covering:
  - repository and generated-project directory structure
  - `service -> endpoint -> transport` responsibility boundaries
  - a `microgen` IR-centered evolution path
  - source mapping strategy for Go IDL / Proto / DB schema
  - AI Tool / MCP generation direction
  - shared design guidance for errors, metadata, context, observability, testing, and plugin strategy
- `cmd/microgen/ir/` was introduced as the first concrete step of that IR-centered evolution:
  - `ir.Project`, `ir.Service`, `ir.Method`, `ir.Message`, and `ir.Field` now define a source-agnostic contract model
  - `ir.FromParseResult(...)` converts both Go-IDL and Proto parser output into that shared contract model
  - generated `skill/skill.go` now consumes IR instead of ranging directly over parser models and services
  - generated `.proto` contract assembly now also consumes IR message/method data, so RPC signatures follow the real input/output contract instead of reconstructing request/response names from method names
  - generated `README.md` content now also consumes IR service/method data, including multi-service endpoint listings and proto quick-start guidance
  - generated `service/` and `endpoint/` skeletons now also consume IR method/input/output data while still reusing parser-backed model metadata where repository/model scaffolding needs it
  - generated `transport/http` and `transport/grpc` skeletons now also consume IR method/input/output data, including HTTP route wiring, gRPC client/server glue, and route-prefix-sensitive swagger annotations
  - generated `cmd/main.go` now also consumes IR service metadata for core startup wiring, imports, banner text, health output, and gRPC registration while still reusing parser-backed route grouping helpers where convenient
  - the remaining `SvcRoutes` helper used by `main.tmpl` route/debug output now also consumes IR services instead of parser services
  - `cmd/microgen/ir` now also has a first direct `DB schema -> IR` path via `FromTableSchemas(...)`, so the DB input path no longer needs to conceptually stop at `ParseResult` before entering the unified contract model
  - `ir.Message` and `ir.Field` now also retain model-oriented metadata such as table names, gorm tags, and key/nullability flags so model/repository scaffolding can rebuild from IR
  - generator model/repository views now prefer IR metadata, which means IR-only generation is now directly covered for DB, Go-IDL, and Proto inputs instead of requiring a compatibility `ParseResult`
  - generator source-mode branching now also prefers IR `project.Source` strings internally, reducing its remaining dependence on parser-specific source enums during artifact generation
  - `generator.GenerateIR(...)` now exists as the explicit IR-first entry point, and the `microgen` CLI now parses inputs into IR before calling that path rather than threading parser results through generation
  - the old generator compatibility entry points and compatibility-context bridge have now been removed, so generation flows only through IR-backed APIs instead of threading parser-era entry points through the generator package
  - `cmd/microgen/main.go` now also returns explicit errors from the IDL parse path and DB introspection / `idl.go` write path, instead of mixing helper-local `log.Fatal(...)` calls with silently ignored DB-path errors
  - generator tests now consistently exercise the IR-first path for normal generation behavior, and the old compatibility-entry tests/helpers have been removed along with the obsolete entry points
  - while doing that cleanup, historical comment-encoding damage in `generator_test.go` was repaired enough to keep the package compiling and the migrated test suite stable
  - focused `cmd/microgen/ir` tests now protect the Go-IDL and Proto conversion paths
- `kit.WithRequestID()` now performs real request ID propagation instead of acting as a stub.
- `cmd/microgen/generator/generator.go` was decomposed so responsibilities are now split across:
  - `layout.go`
  - `project_files.go`
  - `runtime_artifacts.go`
  - `model_artifacts.go`
  - `drivers.go`
  - `template_funcs.go`
- `GenerateFull` was split into explicit phase helpers:
  - `prepareProject`
  - `generateModelArtifacts`
  - `generateServiceArtifacts`
  - `generateFinalProjectArtifacts`
- generator orchestration protection tests were added to lock in key flow behavior:
  - Go IDL inputs are copied to `idl.go`
  - Proto inputs do not create `idl.go`
  - `go.mod` replace paths stay correct for `testdata` outputs
  - route prefix behavior stays aligned across generated artifacts
- generator structure-protection tests were extended to lock in current output conventions for:
  - `docs/docs.go` as the Swagger stub path when swag output is enabled
  - `client/<service>/demo.go` as the generated demo-client path
  - `pb/<service>/<service>.proto` as the generated proto asset path for gRPC output
- generator phase coverage was also widened with a full-feature orchestration test that verifies one combined generation run can produce:
  - project setup artifacts such as `go.mod` and `idl.go`
  - model and repository artifacts
  - service, endpoint, HTTP transport, gRPC transport, client, SDK, and test artifacts
  - final project artifacts such as `cmd/main.go`, config files, `README.md`, `docs/docs.go`, and `skill/skill.go`
  - route prefix consistency across generated HTTP transport and startup wiring even in the combined feature set
- `tools` integration coverage was strengthened so end-to-end `microgen` runs now also verify:
  - `go.mod` generation for both IDL and Proto paths
  - `client/` and `sdk/` artifact generation in integration outputs
  - `idl.go` is present for IDL generation and absent for Proto generation
- `tools` integration coverage now also exercises more of the CLI-visible feature matrix:
  - Proto integration runs with `-protocols http,grpc` and `-swag`
  - integration assertions now include `docs/docs.go`
  - Proto integration assertions now include gRPC transport output and `pb/<service>/<service>.proto`
  - IDL and Proto integration assertions now also verify route prefix propagation across generated HTTP transport and startup wiring
  - Proto integration assertions now also verify that generated README quick-start text explains the required `protoc` step before trying to run the service
  - Proto integration assertions now also verify that generated proto assets contain concrete message fields derived from the current contract and that generated README guidance tells users to review the contract before running `protoc`
  - Proto integration coverage now also has an environment-aware component-flow check that will compile generated proto stubs plus generated `service/endpoint/transport/client/sdk/skill` packages when `protoc` and its Go plugins are available, and otherwise skips explicitly
  - that proto component-flow check now reports exactly which protobuf tool is missing, and supports environment-variable overrides for tool locations instead of relying only on `PATH`
  - with the protobuf toolchain now available, that proto component-flow check passes, exercises a real `service + endpoint + transport + log` assembly probe, and protects compatibility with modern `protoc-gen-go-grpc` output by requiring generated gRPC servers to embed `Unimplemented...Server`
- `tools` integration coverage now also protects `microgen` usability and rerun reliability:
  - default IDL CLI generation is checked for expected out-of-the-box artifacts without requiring extra flags
  - a minimal runnable IDL generation path is checked by building the generated `./cmd` binary and starting it successfully
  - a minimal runnable IDL generation path with `config/docs/model/db/skill` disabled is checked by building the generated `./cmd` binary, assembling generated `service/endpoint/transport` plus framework logging in a component probe, starting it successfully, and confirming `/skill` stays disabled
  - a prefixed minimal runnable IDL generation path is checked by building the generated `./cmd` binary, starting it successfully, confirming the prefixed business route works, and confirming the old unprefixed route does not
  - a fuller runnable IDL generation path is checked by compiling generated `cmd/`, `service/`, `endpoint/`, `transport/`, `client/`, `sdk/`, and `skill/` packages, assembling `service + endpoint + transport + log` in a component probe, starting the generated service, running the generated demo client, and verifying that a small SDK caller can reach the scaffolded API and surface the expected structured error
  - runnable generated IDL startup checks now also verify more real HTTP surface behavior such as `/debug/routes`, `/skill?format=mcp`, prefixed business routes, and disabled skill endpoints when `-skill=false`
  - runnable generated Proto projects are now also checked end-to-end over real gRPC by generating stubs with `protoc`, building the generated `./cmd` binary, starting the service with both HTTP and gRPC listeners, and exercising the generated gRPC client against the live server
  - `-from-db` generation is now also checked end-to-end against a real SQLite database created during the test run, including schema introspection, `idl.go` emission, model/service/endpoint/transport generation, project build, startup, and CRUD-style HTTP route exposure
  - rerunning IDL generation preserves customized `go.mod` content when the module path already matches
  - rerunning IDL generation preserves a real `docs/docs.go` instead of overwriting it with a stub
- `tools` integration coverage now also protects `microgen` CLI failure behavior:
  - running without `-idl` or `-from-db` fails with a clear validation error
  - missing IDL paths fail clearly instead of generating partial output
  - unsupported driver values fail clearly during generator setup
- `cmd/microgen/main.go` now maps the user-facing `-driver sqlite` option to the actual `database/sql` driver name `sqlite3` during live DB introspection, so `-from-db` SQLite generation works through the CLI instead of failing with an unknown-driver error
- phase-related helper tests now also protect:
  - `shouldCopyIDLSource()`
  - `rootRelativePath()`
  - `serviceRoutes()`
- `MICROGEN_COMPATIBILITY.md` was updated to document the current generation-flow contract for:
  - `idl.go` copy behavior
  - `go.mod` update behavior
  - docs stub overwrite expectations
  - route prefix consistency
- `microgen` now also has the first concrete extend-mode implementation path for existing generated projects:
  - new project generation now emits generator-owned aggregation files under `cmd/`:
    - `generated_services.go`
    - `generated_routes.go`
    - `generated_runtime.go`
  - `cmd/main.go` is now thinner, and service wiring plus route registration were moved toward generator-owned aggregation files instead of remaining mixed into one startup file
  - `cmd/microgen/generator/extend_scan.go` now scans existing generated projects for module path, services, models, aggregation points, ownership classification, and current generated-feature signals
  - `cmd/microgen/generator/extend_plan.go` now builds a structured append-service artifact plan before any controlled writes happen
  - `cmd/microgen/generator/extend_apply.go` now performs the first controlled append-service apply flow
  - `cmd/microgen/main.go` now exposes an explicit `microgen extend ... -append-service ...` entry point instead of hiding incremental mutation inside the normal generation flow
  - the first shipped extend constraint is intentionally conservative:
    - append-service currently expects a Go IDL input containing the full combined contract for both existing and new services
    - extend mode updates only new generated files plus generator-owned aggregation files and the generator-managed `idl.go` snapshot
    - extend mode does not rewrite existing `service/<svc>/service.go` files
- generated HTTP route aggregation was also tightened while doing the extend work:
  - generated service routes are now registered explicitly onto `gorilla/mux` routers instead of relying on broad `PathPrefix(...).Handler(...)` attachment for every service
  - this removes the empty-prefix multi-service routing conflict that would otherwise block safe append-service behavior when multiple services share one generated project
- generator and integration coverage now also protects the extend/aggregation path:
  - package-level tests now cover existing-project scan, ownership classification, append-service planning, and controlled append apply behavior
  - end-to-end integration now verifies that `microgen extend -append-service`:
    - creates the new service subtree
    - preserves edits in pre-existing service implementation files
    - updates generated routing and skill output
    - leaves the resulting generated project buildable and runnable
- extend-mode usability was then tightened so the current first append-service path fails more clearly:
  - CLI extend validation now rejects `-from-db` and `.proto` input up front for `-append-service`
  - missing `-idl` and missing `-append-service` now report extend-specific guidance
  - append-service planning now reports available service names when the requested append target is not found in the supplied contract
  - append-service apply now reports missing existing service definitions as a full-combined-contract requirement instead of a more generic mismatch
  - focused tests now lock in those clearer failure messages, while the end-to-end append-service integration path still passes
- generated configuration was also advanced toward the next-phase loading model:
  - generated `config/config.go` now exposes `Default()`, `LoadLocal(path string)`, `ApplyEnv(cfg *Config)`, `LoadRemote(cfg *Config)`, and `Load(path string)`
  - generated `Load(path string)` now follows the first shared seam: defaults, local YAML, environment overrides, and a remote-loading seam
  - generated config now includes a `RemoteConfig` section plus `remote:` values in `config/config.yaml`
  - the first real remote-loading implementation now exists behind that seam:
    - `LoadRemote(...)` uses Viper remote loading for `provider: consul`
    - remote config is read from Consul KV via `remote.data_id`
    - remote values are layered onto the already-resolved local+env config
    - when `remote.fallback_to_local: true`, provider read failures fall back to local config instead of aborting startup
  - generated config structs now also include `mapstructure` tags so Viper remote decoding follows the same field names as YAML loading
  - generated `go.mod` now includes the Viper dependency when config generation is enabled
  - focused generator tests plus default-flags and remote-config integration coverage now protect that generated config contract
- `README.md` was updated so the generated project layout description matches current generator behavior more closely:
  - `client/` is called out explicitly
  - `pb/` is described as proto-related gRPC output
  - `docs/docs.go` and `idl.go` are now documented as optional generated artifacts with specific meanings
- generated project README behavior was tightened to better match actual usability:
  - Go-IDL-driven projects still advertise a direct `go run ./cmd/main.go` quick start
  - Proto-driven projects now advertise a `protoc --go_out=. --go-grpc_out=.` step before `go run`, instead of implying they are immediately runnable without protobuf stub generation
  - Proto-driven projects now describe the generated `pb/<service>/<service>.proto` file as a derived contract that should be reviewed before stub generation, with `TODO` fallback only for shapes that still cannot be derived automatically
- generated proto output was tightened so `.proto` files are no longer limited to request/response placeholders:
  - message fields are now derived from parsed Go IDL structs and parsed proto message definitions when those shapes are available
  - nested model references such as `User` are emitted as concrete proto messages
  - placeholder `TODO` message bodies are now reserved for unsupported or unknown shapes rather than being the default path
  - common composite and wire-level shapes such as `[]string`, `[]byte`, `map[string]string`, and `time.Time` now map to concrete proto fields instead of falling back to generic placeholders
  - pointer-backed scalar fields such as `*string` and `*int32` now map to `proto3 optional` fields so presence semantics survive generation more faithfully
  - `time.Duration` now maps to `google.protobuf.Duration`, continuing the move toward explicit well-known-type handling instead of string fallbacks
- `transport/README.md` now documents the cross-protocol hook contract more explicitly:
  - `Before`
  - `After`
  - `Finalizer`
  with shared semantics across HTTP/gRPC and client/server transports
- `transport/grpc/server` was tightened to match HTTP server safety expectations:
  - default error handler is now initialized
  - nil essential constructor parameters now panic consistently
  - tests now cover decode and endpoint error paths without explicit error handler setup
- transport runtime option handling was further tightened so explicit nil overrides do not leave delayed request-time panics behind:
  - `transport/grpc/server.ServerErrorHandler(nil)` now falls back to the default log-based error handler instead of panicking when a request fails
  - `transport/http/server.ServerErrorEncoder(nil)` now falls back to `transport.DefaultErrorEncoder` instead of crashing on the first encoded error path
  - `transport/http/client.SetClient(nil)` now falls back to `http.DefaultClient` instead of leaving a nil client that would panic on the first outbound request
  - HTTP/gRPC client/server `Before` / `After` / `Finalizer` option helpers now ignore nil hooks instead of storing them and failing later during live request handling
  - focused tests now protect those nil-option and nil-hook paths across HTTP and gRPC transports
- transport clients were also tightened to fail fast on invalid construction:
  - `transport/http/client.NewExplicitClient` now panics on nil essential parameters
  - `transport/http/client.NewClient` now panics on nil target or nil request encoder
  - `transport/grpc/client.NewClient` now panics on nil essential parameters
  - client constructor tests were added for both HTTP and gRPC
- gRPC client transport metadata handling was tightened to better match HTTP client observability expectations:
  - gRPC response headers and trailers are now stored in context for downstream decode/finalizer inspection
  - a focused `bufconn` test now protects that metadata propagation path
- `endpoint` typed and composition contracts were tightened to fail earlier and more consistently:
  - `TypedEndpoint.Wrap()` now returns a structured type-assertion error on request mismatch instead of panicking
  - `NewBuilder(nil)`, `Builder.Use(nil)`, and `Chain(...nil...)` now fail fast instead of deferring misuse to request time
  - endpoint docs now describe typed assertion behavior and builder composition constraints explicitly
- logging-related runtime entry points were made more forgiving where the framework can safely recover:
  - `endpoint.LoggingMiddleware(nil, ...)` now degrades to a nop logger
  - `kit.WithLogging(nil)` now preserves a safe logger instead of installing a nil one
  - focused tests now protect nil-logger composition paths in both `endpoint` and `kit`
- `kit` option validation was tightened for obviously invalid configuration:
  - `WithRateLimit(<=0)` now fails fast
  - `WithTimeout(<=0)` now fails fast
  - `WithCircuitBreaker(0)` now fails fast
  - `WithGRPC(\"\")` now fails fast
  - focused tests now protect these invalid option paths
- the README-level `kit.WithGRPC(...)` usage path now also has a live runtime test:
  - a real proto-generated service is registered through `svc.GRPCServer()`
  - `kit.Service.Start()` launches both listeners
  - a real gRPC client dials the configured address and completes a unary RPC successfully

This means the generator is now closer to an orchestration layer rather than a single monolithic file.

## Planned Next Phase

The next `microgen` roadmap is centered on two linked capabilities.

### 1. Generated configuration with remote-config support

Intent:

- every generated project should have a consistent `config/` layer
- local config should remain the default runnable path
- remote config should be additive, not mandatory

Planned direction:

- standardize generated config files and runtime loading flow
- introduce a provider seam for file, env, and remote loading
- update generated startup code to consume the shared config layer
- add at least one concrete remote-config provider after the seam exists

Important constraint:

- remote config must not make the default quick-start path harder for users who only want a local generated service

### 2. Incremental extension of existing generated projects

Intent:

- allow `microgen` to add new capability to an existing generated project instead of requiring full regeneration

Planned direction:

- design an explicit extend/append mode
- scan existing generated projects before writing changes
- classify files into safe-to-regenerate, aggregation, and user-owned zones
- implement append-service first
- then extend to append-model and append-middleware

Current status:

- the generator-owned aggregation-file migration is now in place for newly generated projects
- extend mode is now explicit in the CLI via `microgen extend`
- scanner, ownership classification, artifact planning, and a first append-service apply flow are implemented
- append-service now works end-to-end for the conservative first contract:
  - target project must already be in the supported generated layout
  - generator-owned `cmd/generated_*.go` files act as the mutation points
  - input must currently be a full combined Go IDL contract, not a partial delta-only contract
  - CLI and generator error messages are now tighter around unsupported `.proto` input, missing `-append-service`, and incomplete combined Go IDL contracts
- the next work in this track is no longer “design append-service from scratch”; it is:
  - documenting the current extend contract more explicitly
  - improving failure reporting and compatibility guidance
  - extending the same ownership model toward append-model and append-middleware

Important constraint:

Update 2026-04-20:

- the extend track has now moved past "append-service only"
- controlled extend flows are implemented for:
  - `-append-service`
  - `-append-model`
  - `-append-middleware`
  - `-check`
- append-model now works through the same ownership model:
  - generated model/repository output is split into finer-grained generator-owned files such as `model/generated_<name>.go`, `repository/generated_<name>_repository.go`, and `repository/generated_base.go`
  - user model customization remains in `model/<name>.go`
  - generated repository wiring now flows through `service/<svc>/generated_repos.go` and generated runtime migration wiring
- append-middleware now works through explicit generated/custom middleware seams:
  - generator-owned endpoint middleware composition lives in `endpoint/<svc>/generated_chain.go`
  - user-owned middleware customization lives in `endpoint/<svc>/custom_chain.go`
  - generated route vs custom route ownership is also explicit via `cmd/generated_routes.go` and `cmd/custom_routes.go`
- `microgen extend -check -out <project>` now provides a read-only compatibility scan:
  - prints summary, compatibility seams, append-path readiness, and warnings
  - reports missing seams directly on each append path
  - exits `0` when all supported append paths are ready and `2` when compatibility seams are still missing
- the remaining major gap in this track is no longer append capability itself; it is keeping the extend contract documented and stable while shifting focus to the config track's real remote-provider integration

- prefer generating new files plus updating a small number of controlled aggregation files
- avoid overwriting user-owned service implementation files opportunistically

## Working Tree Summary

Current local state is a work-in-progress refactor, not a clean snapshot.

Notable uncommitted areas:

- `kit/`
  `kit/kit.go` was deleted and replaced by smaller files:
  - `doc.go`
  - `service.go`
  - `options.go`
  - `http.go`
  - `grpc.go`
  - `lifecycle.go`
  - `json.go`
  - `requestid.go`
- `cmd/microgen/generator/`
  `generator.go` was slimmed down and new helper files were added:
  - `drivers.go`
  - `layout.go`
  - `layout_test.go`
  - `model_artifacts.go`
  - `orchestration_test.go`
  - `phases.go`
  - `phases_internal_test.go`
  - `project_files.go`
  - `runtime_artifacts.go`
  - `template_funcs.go`
- docs and planning files updated:
  - `README.md`
  - `PROJECT_WORKFLOW.md`
  - `IMPLEMENTATION_PLAN.md`
  - `MICROGEN_COMPATIBILITY.md`
  - `PROJECT_SNAPSHOT.md`
- endpoint documentation updated:
  - `endpoint/README.md`
- root documentation updated:
  - `README.md`
- transport documentation updated:
  - `transport/README.md`
- transport runtime updated:
  - `transport/grpc/server/server.go`
  - `transport/grpc/server/server_test.go`
  - `transport/grpc/server/options.go`
  - `transport/grpc/context.go`
  - `transport/http/client/client.go`
  - `transport/http/client/client_test.go`
  - `transport/http/client/options.go`
  - `transport/grpc/client/client.go`
  - `transport/grpc/client/client_test.go`
  - `transport/grpc/client/options.go`
  - `transport/http/server/server.go`
  - `transport/http/server/server_test.go`
  - `transport/http/server/options.go`
- generated-project integration coverage updated:
  - `tools/integration_test.go`

Interpretation:

- the repo is mid-refactor but in a coherent state
- the active branch has not yet been normalized into a final commit
- future sessions should read the new files rather than looking for old monoliths like `kit/kit.go`
- future `microgen` work should treat config generation and remote-provider integration as the highest-value next ticket
- append-service is no longer only a roadmap item, and append-model / append-middleware / extend-check are no longer design-only work either; the immediate next `microgen` task is to turn the current config remote seam into one real provider-backed implementation

## Recent Verification

The following checks passed during the current refactor thread:

- `go test ./kit`
- `go test ./kit ./endpoint ./transport/... ./sd/... ./log ./utils`
- `go test ./cmd/microgen/generator`
- `go test ./cmd/microgen/...`
- `go test ./cmd/microgen/generator ./cmd/microgen/...`
- `go test ./transport/...`
- `go test ./transport/grpc/server ./transport/...`
- `go test ./transport/http/client ./transport/grpc/client ./transport/...`
- `go test ./transport/grpc/client`
- `go test ./transport/http/client ./transport/grpc/client ./transport/...` after gRPC client metadata propagation alignment
- `go test ./transport/http/server ./transport/grpc/server ./transport/http/client ./transport/grpc/client ./transport/...` after nil-hook filtering and nil override fallback tightening
- `go test ./endpoint`
- `go test ./kit ./endpoint ./transport/... ./sd/... ./log ./utils` after endpoint contract tightening
- `go test ./endpoint ./kit`
- `go test ./tools/... -run TestMicrogenIntegration -v`
- `go test ./kit -run "Test(Readme_WithGRPC_LiveRPC|Service_WithGRPC_.*)" -v`
- `go test ./kit ./endpoint ./transport/... ./sd/... ./log ./utils` after nil-logger contract tightening
- `go test ./kit`
- `go test ./kit ./endpoint ./transport/... ./sd/... ./log ./utils` after kit option validation tightening
- `go test ./cmd/microgen/generator` after adding layout/orchestration compatibility tests
- `go test ./cmd/microgen/...` after aligning generator docs and structure-protection tests
- `go test ./cmd/microgen/generator` after adding a full-feature phase/orchestration test
- `go test ./cmd/microgen/...` after widening generator phase coverage
- `go test ./tools/... -run TestMicrogenIntegration -v` after extending end-to-end generator artifact checks
- `go test ./tools/... -run TestMicrogenIntegration -v` after promoting `docs/` and `pb/` guarantees into integration coverage
- `go test ./tools/... -run TestMicrogenIntegration -v` after promoting route-prefix guarantees into CLI-level integration coverage
- `go test ./tools/... -run TestMicrogenIntegration -v` after adding default-flags and rerun-reliability checks
- `go test ./tools/... -run TestMicrogenIntegration -v` after adding CLI failure-path reliability checks
- `go test ./tools/... -run TestMicrogenIntegration -v` after adding generated-project build-and-run validation
- `go test ./tools/... -run TestMicrogenIntegration -v` after adding minimal-feature-off generated-project build-and-run validation
- `go test ./tools/... -run TestMicrogenIntegration -v` after adding prefixed generated-project runtime validation
- `go test ./cmd/microgen/...` after adding explicit extend mode, existing-project scan, artifact planning, and first append-service apply support
- `go test ./tools/... -run TestMicrogenIntegration -v` after adding CLI-level append-service integration coverage and moving generated route aggregation onto explicit mux registration
- `go test ./tools/... -run TestMicrogenIntegration/IDL_GeneratedProject_BuildsAndRuns -v` after adding `/debug/routes` and `/skill?format=mcp` startup validation
- `go test ./tools/... -run TestMicrogenIntegration/IDL_MinimalProject_BuildsAndRunsWithoutOptionalFeatures -v` after checking both OpenAI-tool and MCP `/skill` endpoints stay disabled when `-skill=false`
- `go test ./tools/... -run "TestMicrogenIntegration/(IDL_GeneratedProject_BuildsAndRuns|IDL_MinimalProject_BuildsAndRunsWithoutOptionalFeatures|IDL_PrefixedProject_BuildsAndServesPrefixedBusinessRoute)" -v` after widening runnable generated-project route assertions
- `go test ./tools/... -run TestMicrogenIntegration/IDL_FullGeneratedComponents_AreUsable -v` after adding end-to-end component usability coverage for generated `cmd/`, `client/`, `sdk/`, and `skill/`
- `go test ./tools/... -run TestMicrogenIntegration/IDL_FullGeneratedComponents_AreUsable -v` after extending that component-usage coverage to explicitly assemble generated `service`, `endpoint`, `transport`, and framework `log` into a working request path
- `go test ./tools/... -run "TestMicrogenIntegration/(IDL_MinimalProject_BuildsAndRunsWithoutOptionalFeatures|IDL_FullGeneratedComponents_AreUsable)" -v` after extracting a reusable IDL component probe and using it in both minimal and fuller generated-project paths
- `go test ./tools/... -run TestMicrogenIntegration/Proto_ComponentFlow_WhenProtocAvailable -v` after adding a `protoc`-gated proto component-flow check that skips cleanly when the local protobuf toolchain is unavailable
- `go test ./tools/... -run TestMicrogenIntegration/Proto_ComponentFlow_WhenProtocAvailable -v` after improving protobuf tool detection so the skip reason now reports the exact missing binary (currently `protoc-gen-go` in this environment)
- `go test ./tools/... -run TestMicrogenIntegration/Proto_ComponentFlow_WhenProtocAvailable -count=1 -v` after updating generated gRPC server transport code to embed `Unimplemented...Server` so the proto component flow passes with the installed protobuf toolchain
- `go test ./tools/... -run TestMicrogenIntegration/Proto_ComponentFlow_WhenProtocAvailable -count=1 -v` after extending the proto component-flow test from compilation-only coverage to a real `service/endpoint/transport/log` assembly probe
- `go test ./cmd/microgen/generator ./tools/...` after tightening proto README quick-start guidance and locking it with package plus integration tests
- `go test ./cmd/microgen/generator ./tools/...` after tightening proto scaffold guidance so README explains both the required `TODO` field completion and the required `protoc` step
- `go test ./cmd/microgen/generator ./tools/...` after deriving concrete proto message fields from parsed contracts and updating README guidance to match
- `go test ./cmd/microgen/generator` after tightening proto complex-type mappings for bytes, repeated fields, maps, nested messages, and `google.protobuf.Timestamp`
- `go test ./tools/...` after verifying end-to-end generator coverage still passes with the richer proto field derivation
- `go test ./cmd/microgen/generator` after adding `proto3 optional` mapping for pointer-backed scalar fields
- `go test ./tools/...` after verifying integration coverage still passes with `proto3 optional` field generation
- `go test ./cmd/microgen/generator` after mapping `time.Duration` to `google.protobuf.Duration`
- `go test ./tools/...` after verifying integration coverage still passes with `Duration` well-known-type imports
- `go test ./cmd/microgen/generator -run "TestGenerateProject_(FromDBIR_GeneratesModelArtifactsWithoutCompatParseResult|FromGoIR_GeneratesArtifactsWithoutCompatParseResult|FromProtoIR_GeneratesProtoArtifactsWithoutCompatParseResult)" -count=1` after locking in IR-only generation coverage for DB, Go-IDL, and Proto inputs
- `go test ./cmd/microgen/generator ./tools/... -count=1` after moving model/repository generation off the last required `ParseResult` compatibility path
- `go test ./cmd/microgen/generator ./cmd/microgen/... ./tools/... -count=1` after replacing generator-internal `parser.SourceType` branching with IR-backed source strings
- `go test ./cmd/microgen/generator -run "TestGenerate(IR|Project_)" -count=1`
- `go test ./cmd/microgen ./tools/... -count=1` after switching the CLI orchestration path over to `GenerateIR(...)`
- `go test ./cmd/microgen/... -count=1` after making the CLI IDL/DB helper paths return explicit errors and locking unsupported-driver plus `idl.go` write failures with focused tests
- `go test ./cmd/microgen/generator -count=1`
- `go test ./cmd/microgen/... -count=1` after moving common proto/skill/sdk/route-prefix generator tests onto `GenerateIR(...)`
- `go test ./cmd/microgen/generator -count=1`
- `go test ./cmd/microgen/... -count=1` after moving a broader set of generator contract tests (`go.mod`, config, docs, README, multi-service layout/content) onto `GenerateIR(...)`
- `go test ./cmd/microgen/generator -count=1`
- `go test ./cmd/microgen/... -count=1` after finishing the migration of the remaining general `generator_test.go` cases onto `GenerateIR(...)`
- `go test ./cmd/microgen/generator -count=1`
- `go test ./cmd/microgen/... -count=1` after moving orchestration tests onto `GenerateIR(...)` and repairing comment-encoding fallout in `generator_test.go` so the package remains buildable
- `go test ./cmd/microgen/generator -count=1`
- `go test ./cmd/microgen/... -count=1` after removing the remaining generator compatibility entry points and compatibility-context bridge so the package is now IR-only
- `go test ./cmd/microgen/generator -count=1` after splitting model/repository output into generator-owned files and introducing generated service repository seams
- `go test ./cmd/microgen/... -count=1` after adding `-append-model`, `-append-middleware`, and `extend -check`
- `go test ./tools/... -run "TestMicrogenIntegration/(IDL_Extend_AppendService_PreservesExistingFilesAndServesNewRoute|IDL_Extend_AppendModel_PreservesExistingHooksAndBuilds|IDL_Extend_AppendMiddleware_PreservesCustomChainAndServesWrappedErrors|IDL_Extend_Check_ReportsCompatibility|IDL_Extend_Check_ReturnsExitCodes)" -count=1`
- `go test ./tools/... -run "TestMicrogenIntegration/(IDL_CustomRoutes_ArePreservedAndServed|IDL_DefaultFlags)" -count=1`
- `go test ./cmd/microgen/generator -count=1` after adding the first Viper-backed Consul remote-config provider
- `go test ./cmd/microgen/... -count=1` after wiring the first real remote-config provider through generated config output
- `go test ./tools/... -run "TestMicrogenIntegration/(IDL_DefaultFlags|IDL_Config_RemoteConsul_UsesRemoteAndFallsBackToLocal)" -count=1 -v`

These results mean the recent runtime split and generator decomposition are at least passing their focused validation loops.

## Most Relevant Files

Read these first when resuming work:

- `README.md`
  User-facing product overview.
- `PROJECT_WORKFLOW.md`
  Repository development workflow and validation strategy.
- `PROJECT_SNAPSHOT.md`
  Current repo status, active refactor thread, and next steps.
- `IMPLEMENTATION_PLAN.md`
  Refactor roadmap and sequencing.
- `FRAMEWORK_BOUNDARIES.md`
  Scope and ownership rules for what belongs in the framework.
- `ANTI_PATTERNS.md`
  Things to avoid while changing runtime, generator, or examples.

## Current Code Hotspots

Highest-value active areas:

- `kit/`
  Runtime convenience layer. Recently decomposed and still a likely place for cleanup or contract tightening; recent work has focused on making option misuse fail safely, degrade predictably, or fail fast when configuration is clearly invalid.
- `cmd/microgen/generator/`
  Main refactor zone. Structure is much improved, generator output conventions are more explicitly documented and tested, the IR-only generation path is explicitly protected for DB, Go-IDL, and Proto inputs, and extend support now includes append-service, append-model, append-middleware, and a read-only compatibility check. The config track now has a first real Viper-backed Consul provider behind the generated `LoadRemote(...)` seam, so the next likely work here is CLI/provider-surface tightening rather than the initial provider implementation itself.
- `tools/`
  Integration coverage now checks more of the user-visible generated output shape, including docs, proto/gRPC artifacts, route-prefix propagation, default CLI usability, rerun reliability, clear failure behavior for invalid CLI usage, whether a generated project can actually compile and start, whether a minimal feature-off project still remains runnable, whether prefixed runtime routes actually behave correctly after startup, whether generated `client/` and `sdk/` components remain usable against the generated service, whether generated `service/endpoint/transport` packages can actually be assembled with framework logging into a working request path, and whether proto-generated README plus `.proto` assets accurately reflect the current contract instead of a blanket scaffold-only story. On machines with a protobuf toolchain, it now also reaches one step deeper into proto component compilation, runtime-style component assembly, and modern gRPC server-interface compatibility.
- `endpoint/`
  Architectural spine for middleware and runtime policy composition. Changes here have repo-wide effect, and recent work has focused on making typed endpoint behavior and builder composition fail earlier and more predictably.
- `transport/http/*` and `transport/grpc/*`
  Shared hook semantics are now documented, constructor/error-path safety has been tightened across server and client transports, and gRPC client response metadata now has a clearer parity story with HTTP client finalizer/decoder inspection.

## Immediate Next Steps

If continuing the current refactor line, prefer this order:

1. Decide whether the next config milestone should add explicit CLI surface such as `-config-mode` or `-remote-provider`.
2. Add any missing config integration coverage, especially strict remote-failure behavior or provider validation.
3. Split generated config helpers into smaller files only if it improves clarity without changing the public contract.
4. Revisit `endpoint` and `transport` shared patterns only after generator/config momentum settles.

Keep these constraints in mind:

- local-config startup should remain the default happy path
- extend mode should remain a documented product surface, not a best-effort merge path
- generated user-facing outputs such as config, routes, `client/`, and `sdk/` should stay protected by integration tests
- generator entry points should remain IR-first unless a deliberate migration decision changes that

## Recommended Next Session Start

If a new AI session resumes this work, the best low-friction start is:

1. Read this file.
2. Run `git status --short`.
3. Re-run the smallest relevant test loop:
   - runtime thread: `go test ./kit ./endpoint ./transport/... ./sd/... ./log ./utils`
   - generator thread: `go test ./cmd/microgen/...`
4. Pick one concrete next task before editing code.

Recommended first task right now:

- decide whether to ship the current Viper-backed Consul provider as the first stable contract or follow immediately with CLI-level provider selection

Specifically:

- read `MICROGEN_NEXT_PHASE.md`, `MICROGEN_CONFIG_DESIGN.md`, and the generated config templates first
- then decide whether the next slice is CLI surface, stricter validation, or config package file-splitting
- add or update tests before broadening the provider surface
- only revisit transport/runtime cleanup after the config thread lands

## Validation Shortcuts

Use the smallest sufficient loop first.

For recent refactor areas:

- runtime changes:
  `go test ./kit ./endpoint ./transport/... ./sd/... ./log ./utils`
- generator changes:
  `go test ./cmd/microgen/...`
- targeted extend integration:
  `go test ./tools/... -run "TestMicrogenIntegration/IDL_Extend_AppendService_PreservesExistingFilesAndServesNewRoute" -v`
- targeted extend check integration:
  `go test ./tools/... -run "TestMicrogenIntegration/(IDL_Extend_Check_ReportsCompatibility|IDL_Extend_Check_ReturnsExitCodes)" -v`
- targeted default-config integration:
  `go test ./tools/... -run "TestMicrogenIntegration/IDL_DefaultFlags" -v`
- broader verification:
  `make verify`

## Session Handoff Notes

When ending a work session after meaningful structural changes:

1. Update this file with:
   - what changed
   - what remains risky
   - what should happen next
   - which focused tests were actually run
2. Update `IMPLEMENTATION_PLAN.md` only if the roadmap or sequencing changed.
3. Update `README.md` only if user-visible behavior changed.
4. Prefer leaving the next session a concrete first task, not a vague status note.

## Working Assumptions

Assume these unless new evidence says otherwise:

- generated output shape is externally meaningful
- docs/examples should follow real package behavior
- framework growth should happen through clear extension points, not special cases
- refactors should prefer smaller files and explicit contracts over new abstraction layers
