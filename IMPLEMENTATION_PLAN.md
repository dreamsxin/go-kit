# Implementation Plan

This document turns the current repository state into a concrete refactor roadmap.

It is intentionally narrower and more execution-oriented than `FRAMEWORK_BOUNDARIES.md`.
Use that document for product scope decisions, and use this one for sequencing code cleanup.

Important status note:

- the earlier structural cleanup around `kit`, transport safety, and the IR-first `microgen` flow is largely in place
- the next planned phase is no longer just decomposition and cleanup
- the next planned phase is additive `microgen` product expansion in two linked tracks:
  - generated configuration with remote-config support
  - incremental extension of already-generated projects with new services, models, and middleware

## Refactor Goals

The current roadmap focuses on eight outcomes:

1. Reduce hidden coupling across runtime packages.
2. Split oversized modules into clearer responsibilities.
3. Stabilize public-facing APIs before adding more features.
4. Treat generated output as a compatibility contract.
5. Keep examples and docs aligned with the runtime architecture.
6. Strengthen validation so future refactors stay safe.
7. Add a first-class generated configuration layer that can grow from local files to remote configuration.
8. Let `microgen` evolve existing generated projects incrementally instead of forcing all-or-nothing regeneration.

## Current Signals From The Codebase

The repository is already structurally close to the intended product shape, but several refactor signals are visible:

- `kit/kit.go` currently mixes service bootstrap, middleware assembly, health wiring, HTTP lifecycle, and gRPC lifecycle.
- `endpoint/` is the densest runtime package and acts as the architectural spine, so drift here will affect every layer.
- `transport/http/*` and `transport/grpc/*` contain repeated client/server patterns that should be reviewed for shared option and hook contracts.
- `cmd/microgen/generator/generator.go` owns too many responsibilities: directory creation, template execution, routing conventions, config generation, SDK generation, and compatibility-sensitive output decisions.
- Source comments contain encoding corruption in a few places, which lowers maintainability and signals inconsistent file handling.
- There are placeholder or sharp-edge runtime behaviors that should be normalized before expanding APIs, such as panic-based guardrails and no-op middleware stubs.

These signals originally pointed to structural cleanup before feature expansion.

That cleanup has now advanced far enough that the next phase can safely shift toward additive `microgen` capability work, as long as compatibility guardrails remain in place.

## Guardrails

The refactor should preserve these non-negotiables:

- Keep the service -> endpoint -> transport layering model intact.
- Avoid breaking the primary `kit` quickstart flow without an explicit migration note.
- Treat `microgen` output layout and CLI flags as user-facing behavior.
- Prefer additive extension seams over hidden special cases.
- Do not mix repository-internal cleanup with platform-scope expansion.
- Prefer extending generated projects through new files and explicit aggregation points instead of patching user-owned files opportunistically.
- Keep generated projects runnable with local configuration even when remote-config support is enabled as an option.
- Treat append/extend behavior as a compatibility-sensitive product contract, not an internal convenience helper.

## Workstreams

## Workstream 1: Runtime Surface Audit

Objective:

- document which runtime APIs are intentionally public and which are implementation details

Actions:

- audit `kit`, `endpoint`, `transport`, `sd`, `log`, and `utils`
- mark stable entry points versus convenience helpers versus internals
- identify panic-based or placeholder behavior that should be replaced with explicit contracts
- capture any API decisions that require migration notes before code moves

Deliverables:

- a package surface inventory
- a short list of high-risk compatibility points

Success criteria:

- maintainers can tell whether a refactor is internal-only or user-visible before changing code

## Workstream 2: `kit` Runtime Decomposition

Objective:

- reduce responsibility concentration in `kit/kit.go`

Actions:

- separate service construction, HTTP registration, middleware assembly, and lifecycle management into smaller files
- make the request-ID path real and testable instead of leaving it as a pass-through stub
- normalize health endpoint behavior as an explicit runtime concern
- review gRPC startup and shutdown flow for consistency with HTTP lifecycle behavior

Success criteria:

- `kit` remains the fastest on-ramp for users
- lifecycle code is easier to test in isolation
- middleware-related behavior is explicit and no longer hidden inside one large file

## Workstream 3: Endpoint And Transport Contract Tightening

Objective:

- make the runtime boundaries easier to reason about and harder to bypass

Actions:

- split `endpoint/` by concern where it improves discoverability
- standardize option naming and middleware composition patterns
- compare HTTP and gRPC client/server packages for duplicated hook, encode/decode, and finalizer concepts
- extract shared transport conventions only where the abstraction is genuinely stable
- add or tighten tests around error handling, middleware order, and failer semantics

Success criteria:

- common runtime policies remain centered in `endpoint`
- transport packages stay protocol-focused instead of accumulating policy behavior
- tests protect the intended separation more directly

## Workstream 4: `microgen` Generator Modularization

Objective:

- break generator responsibilities into compatibility-aware modules

Actions:

- split `cmd/microgen/generator/generator.go` into smaller units such as layout, template execution, per-artifact generation, and compatibility helpers
- centralize generated path conventions instead of reconstructing them ad hoc in many methods
- isolate database-driver metadata and route-prefix shaping behind named helpers
- document which generated files and directories are intentional product conventions
- review template ownership so output-shape changes are easier to assess during code review

Success criteria:

- generator changes can be scoped to one artifact family at a time
- output layout decisions are explicit and reviewable
- template updates are easier to test and less likely to cause accidental contract drift

## Workstream 5: Documentation And Example Alignment

Objective:

- keep the public story aligned with the actual code after refactors land

Actions:

- update `README.md` only when runtime or generator behavior actually changes
- keep `PROJECT_WORKFLOW.md` mapped to the real package layout and test loops
- use examples to demonstrate the intended layering, not shortcuts that weaken it
- clean up comment encoding issues in touched files as part of normal refactor work

Success criteria:

- docs remain trustworthy during the refactor
- examples continue acting as executable architecture guidance

## Workstream 6: Validation Matrix Hardening

Objective:

- make structural refactors safe to ship incrementally

Actions:

- keep using the top-level targets in `Makefile`
- add focused tests when introducing new seams in `kit`, `endpoint`, `transport`, or `microgen`
- preserve integration coverage for generated output via `tools/...`
- reserve `go test -race ./...` for broad confidence passes and release-level validation

Recommended validation by workstream:

- runtime refactors: `make test-runtime`
- generator refactors: `make test-microgen`
- docs/example adjustments: `make test-docs` and `make test-examples`
- milestone verification: `make verify`

Success criteria:

- each refactor step has a clear minimum test loop
- broader regressions are caught before merge, not after release

## Workstream 7: Generated Configuration And Remote Config

Objective:

- make generated projects ship with a consistent configuration layer that starts simple locally and can grow into remote configuration safely

Actions:

- standardize generated `config/` output around a single project-facing config model
- keep local `config.yaml` as the default runnable path for generated projects
- introduce an internal provider abstraction for config loading so file, env, and remote sources can share one startup contract
- add generator flags for configuration mode and remote provider selection without breaking the current default flow
- wire generated `cmd/main.go` through that config layer instead of ad hoc config bootstrapping
- choose one initial remote provider only after the provider seam exists, so the first integration does not hard-code the whole design prematurely

Deliverables:

- generated `config/` scaffolding with stable local behavior
- a remote-config loading seam with at least one implementation path
- updated README and compatibility docs for any new flags or generated files

Success criteria:

- generated services still run out of the box with local config only
- remote config can be enabled explicitly without changing the basic project structure
- configuration behavior is tested end to end instead of relying only on template inspection

Recommended design constraints:

- support three conceptual loading modes:
  - file
  - env
  - remote
- allow a hybrid path where remote config can override or augment local defaults
- prefer fallback-to-local behavior over hard startup failure when remote config is optional

## Workstream 8: Incremental Extension Of Generated Projects

Objective:

- let `microgen` add new capability to already-generated projects without treating regeneration as the only maintenance path

Actions:

- design an explicit extend/append mode rather than hiding incremental behavior inside the current full-generation flow
- scan an existing generated project before writing files so the generator understands current services, models, middleware, and key aggregation points
- classify generated files into:
  - safe-to-regenerate files
  - compatibility-sensitive aggregation files
  - user-owned files that should not be overwritten casually
- implement append-service first, because it exercises routing, startup wiring, transport generation, and SDK/client evolution together
- follow with append-model after the IR and model/repository boundaries are stable
- add middleware extension through a dedicated generated aggregation file rather than scattering edits across user-touched files

Deliverables:

- an extend-mode CLI design
- project scanning logic
- file-ownership rules for generated versus user-maintained areas
- integration tests for append workflows on real generated projects

Current status:

- the prerequisite generator-owned aggregation files are now emitted for newly generated projects under `cmd/generated_services.go`, `cmd/generated_routes.go`, and `cmd/generated_runtime.go`
- the extend CLI shape is no longer only conceptual; a first explicit path now exists:
  - `microgen extend -idl <file> -out <project> -append-service <name>`
- existing-project scan, ownership classification, artifact planning, and first append-service apply logic are implemented
- append-service now has end-to-end test coverage proving:
  - the new service subtree is generated
  - existing service implementation edits are preserved
  - generated routing and skill output are updated
  - the resulting project still builds and runs
- the current implementation is intentionally conservative:
  - it currently requires a Go IDL source containing the full combined contract
  - it updates generator-owned aggregation files plus `idl.go`, rather than attempting arbitrary partial merges into user-owned files

Success criteria:

- an existing generated project can receive a new service without losing user edits
- new models and middleware can be added through controlled generation seams
- append behavior is predictable enough to document as part of the `microgen` product story

Recommended design constraints:

- prefer generating new files plus updating a small number of explicit registry or aggregation files
- avoid rewriting existing service implementation files when a user may already have edited them
- if a file is user-owned in practice, treat it as protected unless generation is scoped to clearly delimited regions
- make append behavior explicit in the CLI so users understand they are extending, not regenerating

Suggested CLI direction:

- keep the current full-generation mode for new projects
- add an explicit extension path such as:
  - `microgen extend -idl <file> -out <project>`
  - or targeted options like:
    - `-append-service`
    - `-append-model`
    - `-append-middleware`

The exact CLI shape is still open, but the product direction should be “explicit incremental evolution”, not “best-effort hidden merge”.

## Execution Order

Recommended order:

1. Audit runtime public surfaces and identify compatibility-sensitive behavior.
2. Decompose `kit` into smaller runtime responsibilities.
3. Tighten endpoint and transport contracts around middleware, error flow, and hooks.
4. Modularize `microgen` around artifact families and output conventions.
5. Align examples and docs with the refactored runtime shape.
6. Add a unified generated configuration layer with stable local-config behavior.
7. Add remote-config provider seams and one first provider integration.
8. Build existing-project scan and append-service support.
9. Extend incremental generation to models, then middleware.
10. Finish each milestone with a full repository verification pass.

This order keeps the runtime spine stable before reshaping generator output around it, and it keeps the first incremental-generation work focused on the highest-value path.

## Immediate Next Tasks

These are the best next implementation tickets to open:

### Task 1: define generated config contract

- standardize the generated `config/` package shape
- document required fields for HTTP, gRPC, log, database, and remote config
- keep local `config.yaml` the default runnable path

### Task 2: add config provider seam

- introduce a config loading abstraction for file, env, and remote sources
- update generated `cmd/main.go` to load through the shared seam
- choose fallback behavior when remote config is enabled but unavailable

### Task 3: design incremental extension mode

- decide the CLI contract for extend/append behavior
- define which files are safe to regenerate versus protected
- identify aggregation files that can act as controlled update points

### Task 4: implement append-service first

- scan an existing generated project
- generate a new service subtree without rewriting existing business logic
- update startup and routing aggregation in a limited, testable way

### Task 5: add append-model and append-middleware follow-ups

- keep model/repository generation incremental
- route middleware extension through a dedicated generated composition file
- avoid direct edits to arbitrarily user-owned files

### Task 6: expand integration coverage for config and append workflows

- add generated-project tests for local config
- add tests for remote-config fallback or activation behavior
- add tests that generate a project, modify it, append new capability, and confirm existing edits survive

## Non-Goals

This refactor does not aim to:

- redesign the framework into a platform product
- replace the package layout wholesale in one pass
- introduce a plugin system before current seams are stabilized
- add hidden merge heuristics that silently rewrite user-owned generated-project files
- make remote config mandatory for the default quick-start experience

## Definition Of Done

This refactor is in good shape when:

- runtime responsibilities are easier to locate and test
- `kit` and `microgen` no longer concentrate unrelated concerns in single files
- transport and endpoint contracts are clearer and better protected by tests
- generated output conventions are documented as product behavior
- docs and examples still describe the real framework accurately
- generated projects have a stable configuration story from local config to optional remote config
- existing generated projects can be extended intentionally without destructive regeneration
