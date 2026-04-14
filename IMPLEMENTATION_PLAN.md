# Implementation Plan

This document turns the current repository state into a concrete refactor roadmap.

It is intentionally narrower and more execution-oriented than `FRAMEWORK_BOUNDARIES.md`.
Use that document for product scope decisions, and use this one for sequencing code cleanup.

## Refactor Goals

This refactor focuses on six outcomes:

1. Reduce hidden coupling across runtime packages.
2. Split oversized modules into clearer responsibilities.
3. Stabilize public-facing APIs before adding more features.
4. Treat generated output as a compatibility contract.
5. Keep examples and docs aligned with the runtime architecture.
6. Strengthen validation so future refactors stay safe.

## Current Signals From The Codebase

The repository is already structurally close to the intended product shape, but several refactor signals are visible:

- `kit/kit.go` currently mixes service bootstrap, middleware assembly, health wiring, HTTP lifecycle, and gRPC lifecycle.
- `endpoint/` is the densest runtime package and acts as the architectural spine, so drift here will affect every layer.
- `transport/http/*` and `transport/grpc/*` contain repeated client/server patterns that should be reviewed for shared option and hook contracts.
- `cmd/microgen/generator/generator.go` owns too many responsibilities: directory creation, template execution, routing conventions, config generation, SDK generation, and compatibility-sensitive output decisions.
- Source comments contain encoding corruption in a few places, which lowers maintainability and signals inconsistent file handling.
- There are placeholder or sharp-edge runtime behaviors that should be normalized before expanding APIs, such as panic-based guardrails and no-op middleware stubs.

These signals suggest that the next phase should be structural cleanup, not feature expansion.

## Guardrails

The refactor should preserve these non-negotiables:

- Keep the service -> endpoint -> transport layering model intact.
- Avoid breaking the primary `kit` quickstart flow without an explicit migration note.
- Treat `microgen` output layout and CLI flags as user-facing behavior.
- Prefer additive extension seams over hidden special cases.
- Do not mix repository-internal cleanup with platform-scope expansion.

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

## Execution Order

Recommended order:

1. Audit runtime public surfaces and identify compatibility-sensitive behavior.
2. Decompose `kit` into smaller runtime responsibilities.
3. Tighten endpoint and transport contracts around middleware, error flow, and hooks.
4. Modularize `microgen` around artifact families and output conventions.
5. Align examples and docs with the refactored runtime shape.
6. Finish with a full repository verification pass.

This order keeps the runtime spine stable before reshaping generator output around it.

## Immediate Next Tasks

These are the best first implementation tickets to open:

### Task 1: split `kit/kit.go`

- extract service lifecycle code
- extract option wiring and middleware registration
- add direct tests for request-ID behavior and graceful shutdown

### Task 2: create a generator layout helper

- move output path and directory rules out of `generator.go`
- make generated directory conventions explicit and reusable

### Task 3: audit duplicated transport option patterns

- compare HTTP and gRPC client/server option types
- identify which concepts are shared by contract and which are only coincidentally similar

### Task 4: clean encoding-corrupted comments in touched files

- normalize source comments as files are refactored
- avoid mixing broad formatting churn into unrelated commits

## Non-Goals

This refactor does not aim to:

- redesign the framework into a platform product
- replace the package layout wholesale in one pass
- introduce a plugin system before current seams are stabilized
- add major new runtime features before core contracts are clearer

## Definition Of Done

This refactor is in good shape when:

- runtime responsibilities are easier to locate and test
- `kit` and `microgen` no longer concentrate unrelated concerns in single files
- transport and endpoint contracts are clearer and better protected by tests
- generated output conventions are documented as product behavior
- docs and examples still describe the real framework accurately
