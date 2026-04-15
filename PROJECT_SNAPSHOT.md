# Project Snapshot

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
  Main refactor zone. Structure is much improved, generator output conventions are more explicitly documented and tested, there is now a broader combined-feature orchestration test guarding phase interaction, generated README behavior distinguishes between immediately runnable IDL output and proto output that still needs protobuf stub generation, proto asset generation now emits more concrete message schemas instead of defaulting to placeholder bodies, including richer handling for common composite types, well-known time types, and pointer-backed optional scalars, and the IR-only generation path is now explicitly protected for DB, Go-IDL, and Proto inputs.
- `tools/`
  Integration coverage now checks more of the user-visible generated output shape, including docs, proto/gRPC artifacts, route-prefix propagation, default CLI usability, rerun reliability, clear failure behavior for invalid CLI usage, whether a generated project can actually compile and start, whether a minimal feature-off project still remains runnable, whether prefixed runtime routes actually behave correctly after startup, whether generated `client/` and `sdk/` components remain usable against the generated service, whether generated `service/endpoint/transport` packages can actually be assembled with framework logging into a working request path, and whether proto-generated README plus `.proto` assets accurately reflect the current contract instead of a blanket scaffold-only story. On machines with a protobuf toolchain, it now also reaches one step deeper into proto component compilation, runtime-style component assembly, and modern gRPC server-interface compatibility.
- `endpoint/`
  Architectural spine for middleware and runtime policy composition. Changes here have repo-wide effect, and recent work has focused on making typed endpoint behavior and builder composition fail earlier and more predictably.
- `transport/http/*` and `transport/grpc/*`
  Shared hook semantics are now documented, constructor/error-path safety has been tightened across server and client transports, and gRPC client response metadata now has a clearer parity story with HTTP client finalizer/decoder inspection.

## Immediate Next Steps

If continuing the current refactor line, prefer this order:

1. Decide whether current helper plus artifact-level generator coverage is sufficient or whether one broader sequencing test would add real value.
2. Audit whether any remaining generator guarantees still need to move up into `tools` integration tests, or whether the current split is now sufficient.
3. Revisit `endpoint` and `transport` shared patterns only after generator refactor momentum settles.
4. Update docs when user-facing behavior changes, not before.

Good next tickets:

- decide whether more orchestration rules should be encoded as tests or documentation now that the main output paths are documented
- continue treating `microgen` as a product by testing default-flag behavior and rerun safety, not just feature-on paths
- continue treating `microgen` as a product by testing clear failure behavior as well as successful generation paths
- keep at least one generated-project smoke test that proves the output is not only structurally correct but also runnable
- keep at least one generated-project smoke test for the feature-minimal path so optional layers do not become hidden runtime dependencies
- keep route-prefix guarantees protected at runtime, not only as generated-file string assertions
- keep generated README and quick-start guidance aligned with what each generation mode can actually do, especially for proto workflows that still require protobuf codegen
- continue tightening proto generation so fewer contracts fall back to placeholder `TODO` message bodies
- decide whether timestamp mapping should stay on `google.protobuf.Timestamp` or whether additional well-known types should join it
- decide how far pointer semantics should go beyond scalar `optional`, for example whether slices or maps need extra presence conventions in generated contracts
- decide whether the next well-known-type step should be `structpb`, wrappers, or more duration/timestamp-adjacent conventions rather than adding isolated one-off mappings
- keep testing generated user-facing components as products, not just generated source trees, especially `client/` and `sdk/` entry points
- keep generated gRPC transport templates aligned with the current `protoc-gen-go-grpc` server interface contract, not only older registration patterns
- keep generator entry points IR-only unless a new, deliberate migration policy says otherwise
- check whether any remaining phase-order assumptions only live in package tests and truly need end-to-end coverage
- check whether any remaining endpoint sharp edges still panic at request time and should instead fail earlier with explicit contracts
- check whether any remaining `kit` options still accept obviously invalid values that should fail fast at construction time
- decide whether transport duplication should stay as documented parallel structure or whether a small shared abstraction would now pay for itself
- check whether any remaining HTTP/gRPC transport mismatches exist beyond the gRPC server error-handler and client metadata-parity fixes
- unless a new concrete mismatch is found, transport parity work can pause and focus can return to broader runtime architecture or generator compatibility

## Recommended Next Session Start

If a new AI session resumes this work, the best low-friction start is:

1. Read this file.
2. Run `git status --short`.
3. Re-run the smallest relevant test loop:
   - runtime thread: `go test ./kit ./endpoint ./transport/... ./sd/... ./log ./utils`
   - generator thread: `go test ./cmd/microgen/...`
4. Pick one concrete next task before editing code.

Recommended first task right now:

- check whether any remaining endpoint or transport sharp edges still rely on delayed panics instead of explicit construction-time or type-level contracts

Specifically:

- review whether typed endpoint, builder, and middleware composition boundaries are now consistent with the framework's stated runtime model
- check whether any user-visible generator guarantees are still missing from either documentation or tests
- decide whether any remaining runtime sharp edges belong in endpoint, transport, or kit-level contract tightening
- then revisit transport duplication or broader generator work once runtime contract cleanup slows down

## Validation Shortcuts

Use the smallest sufficient loop first.

For recent refactor areas:

- runtime changes:
  `go test ./kit ./endpoint ./transport/... ./sd/... ./log ./utils`
- generator changes:
  `go test ./cmd/microgen/...`
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
