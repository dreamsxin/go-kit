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
- `tools` integration coverage now also protects `microgen` usability and rerun reliability:
  - default IDL CLI generation is checked for expected out-of-the-box artifacts without requiring extra flags
  - a minimal runnable IDL generation path is checked by building the generated `./cmd` binary and starting it successfully
  - a minimal runnable IDL generation path with `config/docs/model/db/skill` disabled is checked by building the generated `./cmd` binary, starting it successfully, and confirming `/skill` stays disabled
  - rerunning IDL generation preserves customized `go.mod` content when the module path already matches
  - rerunning IDL generation preserves a real `docs/docs.go` instead of overwriting it with a stub
- `tools` integration coverage now also protects `microgen` CLI failure behavior:
  - running without `-idl` or `-from-db` fails with a clear validation error
  - missing IDL paths fail clearly instead of generating partial output
  - unsupported driver values fail clearly during generator setup
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
- `transport/README.md` now documents the cross-protocol hook contract more explicitly:
  - `Before`
  - `After`
  - `Finalizer`
  with shared semantics across HTTP/gRPC and client/server transports
- `transport/grpc/server` was tightened to match HTTP server safety expectations:
  - default error handler is now initialized
  - nil essential constructor parameters now panic consistently
  - tests now cover decode and endpoint error paths without explicit error handler setup
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
  - `transport/grpc/context.go`
  - `transport/http/client/client.go`
  - `transport/http/client/client_test.go`
  - `transport/grpc/client/client.go`
  - `transport/grpc/client/client_test.go`

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
- `go test ./endpoint`
- `go test ./kit ./endpoint ./transport/... ./sd/... ./log ./utils` after endpoint contract tightening
- `go test ./endpoint ./kit`
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
  Main refactor zone. Structure is much improved, generator output conventions are more explicitly documented and tested, and there is now a broader combined-feature orchestration test guarding phase interaction.
- `tools/`
  Integration coverage now checks more of the user-visible generated output shape, including docs, proto/gRPC artifacts, route-prefix propagation, default CLI usability, rerun reliability, clear failure behavior for invalid CLI usage, whether a generated project can actually compile and start, and whether a minimal feature-off project still remains runnable.
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
