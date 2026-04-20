# microgen Extend Mode Design

Purpose:
- Define the implementation-level design for safe extension of already-generated projects.

Read this when:
- You are changing extend mode, append flows, project scanning, or generator-owned aggregation updates.

See also:
- [MICROGEN_INDEX.md](MICROGEN_INDEX.md)
- [MICROGEN_OWNERSHIP.md](MICROGEN_OWNERSHIP.md)
- [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)

This document defines the implementation-level design for incremental extension of already-generated projects.

It is a deeper companion to:

- [MICROGEN_NEXT_PHASE.md](MICROGEN_NEXT_PHASE.md)
- [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)
- [MICROGEN_OWNERSHIP.md](MICROGEN_OWNERSHIP.md)

Use this document when implementing:

- append or extend CLI behavior
- existing-project scanning
- generated file ownership rules
- append-service, append-model, and append-middleware flows
- integration tests for safe extension

## Problem Statement

Today `microgen` is strongest at creating a new generated project from:

- Go IDL
- Proto
- DB schema

But once that project exists, the user experience weakens:

- rerun is not the same thing as safe extension
- users may already have edited service or repository files
- naive regeneration risks overwriting valuable handwritten code

The next phase should let users intentionally extend a generated project without turning the generator into a fragile merge engine.

## Goals

- support explicit extension of existing generated projects
- avoid destructive rewrites of user-owned files
- make extension behavior understandable and testable
- start with the highest-value extend path: append-service
- ensure generated code and user-modified code remain separated structurally

## Non-Goals

- arbitrary AST merging of handwritten code
- patching unsupported project layouts heuristically
- supporting every possible handwritten directory structure
- hot-plugging services into unknown application architectures

## Product Shape

Recommended user-facing concept:

- project creation and project extension are different workflows
- extension works by adding generated slices, not by rewriting user-owned files

Current shipped first step:

- `microgen extend -idl <full-combined.go> -out <project> -append-service <Name>` is now implemented as the first real extend flow
- it scans the existing project, validates ownership and aggregation points, generates artifacts into a temporary tree, and copies only planned new files plus allowed generator-owned updates
- existing `service/<svc>/service.go` files are preserved
- generated aggregation files under `cmd/` act as the stable mutation points

Recommended CLI direction:

```bash
microgen extend -idl <file> -out <project>
```

Optional targeted flags:

- `-append-service <name>`
- `-append-model <name>`
- `-append-middleware <name[,name...]>`

Recommended rollout:

1. explicit extend mode
2. append-service
3. append-model
4. append-middleware

## Required Preconditions

Extend mode should only proceed when the target project is recognized as a supported generated project shape.

Minimum expected signals:

- `go.mod`
- `cmd/`
- at least one of `service/`, `endpoint/`, `transport/`
- recognizable generated structure or metadata

If the target is unsupported, extend mode should fail clearly instead of guessing.

## Existing Project Scan

Extend mode needs a dedicated scan pass before writing files.

Recommended scanner output:

```go
type ExistingProject struct {
	Root              string
	ModulePath        string
	Services          []ExistingService
	Models            []ExistingModel
	Transports        []ExistingTransport
	AggregationPoints AggregationPoints
	Ownership         OwnershipMap
	Warnings          []string
}
```

### Scanner Responsibilities

- detect existing services by package layout
- detect existing models and repositories
- detect generated aggregation files if present
- detect whether `cmd/main.go` appears generator-owned or user-heavily-modified
- classify files by ownership level

### Scanner Non-Goals

- understanding arbitrary business logic internals
- reconstructing every handwritten semantic change

## File Ownership Model

This is the most important rule set in extend mode.

For the full policy, see [MICROGEN_OWNERSHIP.md](MICROGEN_OWNERSHIP.md).

### Tier 1: Safe Generator-Owned Files

These are safe to regenerate or replace.

Examples:

- generated SDK files
- generated client demo files
- generated skill files
- future explicit generated registry files

### Tier 2: Generator-Owned Aggregation Files

These are writable, but only through strict rules.

Examples:

- `cmd/generated_services.go`
- `cmd/generated_routes.go`
- `endpoint/generated_chain.go`

These should become the preferred mutation points for extend mode.

### Tier 3: Protected User-Likely-Owned Files

These should not be overwritten casually.

Examples:

- `service/<svc>/service.go`
- custom repository logic
- manually modified middleware composition files
- hand-edited startup files if they no longer match supported generator-owned shape

Hard rule:

- if an append operation would require rewriting one of these files, the generator should fail instead of attempting a best-effort merge

## Migration Toward Better Aggregation

Current generated projects center a lot of startup logic in `cmd/main.go`.

That is acceptable for project creation, but not ideal for future extension.

Recommended future direction:

- keep `cmd/main.go` small
- move generated service wiring into generator-owned files
- move generated route registration into generator-owned files
- move generated middleware assembly into generator-owned files
- keep user customization outside those generated files

Suggested future file set:

```text
cmd/
├─ main.go
├─ generated_services.go
├─ generated_routes.go
└─ generated_runtime.go
```

This gives extend mode stable patch points and reduces the need to rewrite `cmd/main.go`.

## Append-Service Design

Append-service should be the first supported extend operation.

Current implementation status:

- first shipped and validated through package-level tests plus end-to-end integration coverage
- CLI entry point is exposed through `microgen extend`
- supported only for Go IDL input at this stage
- requires a full combined contract that includes both existing services and the new service being appended
- regenerates aggregation files deterministically from that full contract instead of attempting partial handwritten-code merges

### Inputs

- source contract containing the new service
- target existing project
- service name to append, if not inferable uniquely

### Outputs

- new `service/<svc>/...`
- new `endpoint/<svc>/...`
- new `transport/<svc>/...`
- new `client/<svc>/...` if enabled
- new `sdk/<svc>sdk/...` if enabled
- updated service registration through aggregation files
- updated skill output if skill generation is enabled

### Rules

- do not overwrite existing service implementation files
- fail if target service already exists unless a future explicit replace mode is added
- update only supported aggregation points
- fail if required generator-owned aggregation files are missing
- fail if the provided source contract does not include already-existing services needed to rebuild aggregation files safely
- treat `.proto` input as unsupported for current append-service behavior

### Preferred Implementation Strategy

1. scan existing project
2. validate append target does not already exist
3. validate the provided Go IDL source covers both existing and newly appended services
4. generate the full source contract to a temporary artifact set
5. write only planned new files
6. copy only allowed generator-owned aggregation and snapshot updates
7. run validation checks

### Why It Comes First

- highest product value
- strongest test of scan and ownership rules
- exercises startup, routing, transport, and SDK/client behavior together

## Append-Model Design

Append-model should come after append-service.

### Outputs

- new `model/<entity>.go`
- new repository files where needed
- optional updates to generated repository aggregation

### Rules

- avoid rewriting existing model files
- avoid broad rewrites to repository logic users may have customized
- prefer one-file-per-model or similarly isolated outputs

### Important Constraint

Repository code is very likely to be user-modified, so append-model must be conservative by default.

## Append-Middleware Design

Append-middleware is valuable but riskier.

### Product Intent

Users should be able to add generated middleware wiring without forcing the generator to edit arbitrary service endpoint code.

### Recommended Strategy

- introduce a generator-owned middleware aggregation file
- keep generated middleware registration there
- let users keep custom middleware in separate, user-owned code

### Example Direction

```text
endpoint/
├─ generated_chain.go
└─ custom_chain.go
```

Extend mode should only update the generated chain file.

## Extend Mode Failure Behavior

Extend mode should fail clearly when:

- the target project layout is unsupported
- the target service or model already exists
- a required aggregation point is missing and cannot be created safely
- a protected file would need to be overwritten to complete the operation
- the target project has mixed generated and handwritten logic in a file that extend mode is not allowed to rewrite

The generator should not silently degrade into partial hidden edits.

## CLI Semantics

Recommended semantics:

### `microgen extend`

- required for existing-project mutation
- fails if `-out` does not contain a recognized target project

### `-append-service`

- append exactly one service
- fail if not found in source contract

### `-append-model`

- append exactly one model

### `-append-middleware`

- append named generated middleware to supported generated middleware chain

If these are added as flags without a subcommand, the CLI still needs to make the mode explicit in help text and behavior.

## Internal Generator Architecture

Recommended internal additions:

```go
type ExtendOptions struct {
	AppendService    string
	AppendModel      string
	AppendMiddleware []string
}

type ExistingProjectScanner interface {
	Scan(root string) (*ExistingProject, error)
}
```

Recommended generator phases for extend mode:

1. scan existing project
2. validate extend request
3. build artifact plan
4. generate new artifacts
5. apply controlled updates
6. validate result

## Artifact Plan

Before writing files, extend mode should build a structured plan.

Example:

```go
type ArtifactPlan struct {
	NewFiles        []PlannedFile
	UpdatedFiles    []PlannedUpdate
	ProtectedSkips  []string
	Warnings        []string
}
```

Benefits:

- easier testing
- clearer failure reporting
- lower chance of partial accidental mutation

## Testing Plan

### Unit Tests

- scanner detects supported generated project layouts
- ownership classification behaves as expected
- append validation rejects duplicates cleanly
- artifact planning is deterministic

### Integration Tests

Add `tools` cases for:

- generate base project, append a new service, build and run
- append-service updates routes and startup wiring correctly
- append-model adds model artifacts without damaging existing ones
- append-middleware updates only generated chain files

### Protection Tests

- user edits in protected files survive extend operations
- unsupported target layouts fail with clear errors
- duplicate append attempts fail cleanly

### End-To-End Checks

For append-service, verify:

- project still builds
- service starts
- new route exists
- old routes still work
- generated skill output reflects the new service when enabled

## Compatibility Guidance

Extend mode is compatibility-sensitive.

That means:

- CLI behavior must be documented
- supported target layout must be documented
- file ownership assumptions must be documented
- destructive overwrite behavior must not be introduced casually

If extend mode cannot safely update a project, it should say so directly.

## Suggested Milestones

### Milestone 1

- existing-project scan
- ownership model
- no-op validation mode

### Milestone 2

- append-service support
- generator-owned aggregation files introduced where necessary

### Milestone 3

- append-model support

### Milestone 4

- append-middleware support

## Open Decisions

- subcommand versus flag-based extend UX
- which generated aggregation files to introduce first
- how much migration support to offer for older generated projects
- whether extend mode should write metadata to help future scans

## Definition Of Done

This design is implemented well when:

- extend mode is explicit and documented
- append-service works safely on supported generated projects
- users do not lose handwritten business logic
- generator-owned mutation points are narrow and predictable
- tests prove both success paths and protection behavior
