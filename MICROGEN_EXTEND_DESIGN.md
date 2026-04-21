# microgen Extend Mode Design

Purpose:
- Define the implementation-level design for safe extension of already-generated projects.

Read this when:
- You are changing extend mode, append flows, project scanning, or generator-owned aggregation updates.

See also:
- [MICROGEN_INDEX.md](MICROGEN_INDEX.md)
- [MICROGEN_DESIGN.md](MICROGEN_DESIGN.md)
- [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)

This document defines the implementation-level design for incremental extension of already-generated projects.

It is a deeper companion to:

- [MICROGEN_DESIGN.md](MICROGEN_DESIGN.md)
- [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)

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

The current implementation goal is to let users intentionally extend a generated project without turning the generator into a fragile merge engine.

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

Current user-facing concept:

- project creation and project extension are different workflows
- extension works by adding generated slices, not by rewriting user-owned files

Current shipped extend surface:

- `microgen extend -check -out <project>` provides a read-only compatibility scan
- `microgen extend -idl <full-combined.go> -out <project> -append-service <Name>` is implemented
- `microgen extend -idl <full-combined.go> -out <project> -append-model <Name>` is implemented
- `microgen extend -idl <full-combined.go> -out <project> -append-middleware <Name[,Name...]>` is implemented
- extend mode scans the existing project, validates ownership and aggregation points, generates artifacts into a temporary tree, and copies only planned new files plus allowed generator-owned updates
- existing user-owned files such as `service/<svc>/service.go` are preserved
- generated aggregation files under `cmd/` and `endpoint/` act as the stable mutation points

Current CLI shape:

```bash
microgen extend -idl <file> -out <project>
```

Current targeted flags:

- `-append-service <name>`
- `-append-model <name>`
- `-append-middleware <name[,name...]>`

Current status summary:

- explicit extend mode is implemented
- append-service is implemented
- append-model is implemented
- append-middleware is implemented
- `extend -check` is implemented

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

Current scanner output shape:

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

For the consolidated product-level policy, see [MICROGEN_DESIGN.md](MICROGEN_DESIGN.md).

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

## Aggregation Seams

Current extend behavior depends on keeping startup and routing mutation points in explicit generator-owned files.

Current important file set:

```text
cmd/
├─ main.go
├─ generated_services.go
├─ generated_routes.go
└─ generated_runtime.go
```

Implementation rule:

- keep these generated aggregation files as the preferred patch points so extend mode does not need to rewrite user-owned startup files

## Append-Service

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

### Apply Flow

1. scan existing project
2. validate append target does not already exist
3. validate the provided Go IDL source covers both existing and newly appended services
4. generate the full source contract to a temporary artifact set
5. write only planned new files
6. copy only allowed generator-owned aggregation and snapshot updates
7. run validation checks

## Append-Model

Current implementation status:

- implemented for supported generated projects with model/repository output
- relies on the same scan, planning, temporary-generation, and controlled-apply pattern as append-service

### Outputs

- new `model/<entity>.go`
- new repository files where needed
- optional updates to generated repository aggregation

### Rules

- avoid rewriting existing model files
- avoid broad rewrites to repository logic users may have customized
- prefer one-file-per-model or similarly isolated outputs

### Important Constraint

Repository code is very likely to be user-modified, so append-model stays conservative by default.

## Append-Middleware

Current implementation status:

- implemented for supported generated projects with generator-owned middleware seams
- intended to update only generated middleware composition files rather than handwritten endpoint code

### Apply Strategy

- introduce a generator-owned middleware aggregation file
- keep generated middleware registration there
- let users keep custom middleware in separate, user-owned code

### Current seam shape

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

Current semantics:

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

## Internal Generator Architecture

Current internal types:

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

Current generator phases for extend mode:

1. scan existing project
2. validate extend request
3. build artifact plan
4. generate new artifacts
5. apply controlled updates
6. validate result

## Artifact Plan

Before writing files, extend mode builds a structured plan.

Example:

```go
type ArtifactPlan struct {
	NewFiles        []PlannedFile
	UpdatedFiles    []PlannedUpdate
	ProtectedSkips  []string
	Warnings        []string
}
```

Implementation benefits:

- easier testing
- clearer failure reporting
- lower chance of partial accidental mutation

## Testing Guidance

### Unit Tests

- scanner detects supported generated project layouts
- ownership classification behaves as expected
- append validation rejects duplicates cleanly
- artifact planning is deterministic

### Integration Tests

The important integration checks are:

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

## Remaining Implementation Gaps

The main remaining gaps in this area are now implementation and compatibility-hardening work:

- keep failure reporting specific when compatibility seams are missing
- decide how much compatibility help to offer older generated projects that predate current aggregation seams
- consider whether extend metadata would materially improve scan clarity without becoming another fragile contract
- keep extend coverage focused on protected-file preservation and clear exit behavior

## Definition Of Done

This design is implemented well when:

- extend mode is explicit and documented
- append-service works safely on supported generated projects
- users do not lose handwritten business logic
- generator-owned mutation points are narrow and predictable
- tests prove both success paths and protection behavior
