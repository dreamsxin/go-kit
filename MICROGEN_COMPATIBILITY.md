# microgen Compatibility Guide

This document defines the user-visible compatibility contract for `microgen`.

The goal is simple:

- treat `microgen` as a product, not just an internal code generator
- make generated output expectations explicit
- reduce upgrade surprises for users adopting generated projects

## What Counts As Public Contract

For `microgen`, the public contract is made of three things:

1. the CLI surface
2. the documented generated project layout
3. the documented behavior of major generation modes

Anything outside those areas should be assumed internal unless documented here or elsewhere in public docs.

## Stable User-Facing Surface

The following should be treated as compatibility-sensitive:

- the `microgen` CLI entry point
- documented flags such as:
  - `-idl`
  - `-from-db`
  - `-dsn`
  - `-dbname`
  - `-out`
  - `-import`
  - `-protocols`
  - `-config`
  - `-docs`
  - `-tests`
  - `-model`
  - `-db`
  - `-driver`
  - `-swag`
  - `-skill`
  - `-service`
  - `-prefix`
  - `extend`
  - `-check`
  - `-append-service`
  - `-append-model`
  - `-append-middleware`
- the ability to generate from:
  - Go IDL
  - Proto
  - DB schema
- the documented three-layer generated structure
- generation of service, endpoint, transport, and startup scaffolding
- generation of skill output when `-skill` is enabled
- generation of gRPC output when `grpc` is included in `-protocols`

## Internal Surface

The following are implementation details and not public compatibility promises:

- package structure under `cmd/microgen/generator`
- package structure under `cmd/microgen/parser`
- package structure under `cmd/microgen/dbschema`
- individual template file names and their internal composition
- the exact order files are written during generation
- internal helper functions or intermediate parse structures

Users should not treat template internals as a supported extension API.

## Generated Project Contract

The generated project layout is part of the public story.

Expected generated structure may include:

- `cmd/`
- `service/`
- `endpoint/`
- `transport/`
- `client/`
- `config/`
- `docs/`
- `model/`
- `pb/`
- `repository/`
- `sdk/`
- `skill/`
- copied IDL or generated proto-related assets when relevant

Not every directory appears in every mode, but the meaning of these directories should remain consistent:

- `service/` contains business logic layer code
- `endpoint/` contains middleware-oriented service wrapping
- `transport/` contains protocol adaptation
- `cmd/` contains service startup and wiring
- `client/` contains runnable client/demo scaffolding for generated services
- `docs/` contains generator-created Swagger scaffolding when enabled
- `pb/` contains generated proto-related service contract assets when gRPC is enabled
- `sdk/` contains generated client usage surface
- `skill/` contains AI-facing capability definitions

Ownership guidance that should be treated as compatibility-sensitive:

- generator-owned files and user-owned files should remain meaningfully separated
- users should not be expected to modify files the generator must keep rewriting
- future extend or append behavior should prefer new generated files and explicit generator-owned aggregation files over rewriting user business logic files
- generated model and repository scaffolding should continue moving toward explicit generator-owned files such as `model/generated_<name>.go`, `repository/generated_<name>_repository.go`, and `repository/generated_base.go`, with separate user-owned customization seams where needed
- generated endpoint middleware wiring should continue using explicit generator-owned seams such as `endpoint/<service>/generated_chain.go` with separate user-owned customization seams such as `endpoint/<service>/custom_chain.go`

Additional current conventions that should be treated as compatibility-sensitive:

- `cmd/main.go` remains the generated startup entry point
- `cmd/generated_services.go`, `cmd/generated_routes.go`, and `cmd/generated_runtime.go` are the generator-owned aggregation files used as stable extend-mode mutation points
- `endpoint/<service>/generated_chain.go` is the generator-owned middleware aggregation seam, while `endpoint/<service>/custom_chain.go` remains user-owned
- `client/<service>/demo.go` remains the generated runnable client/demo entry for a service
- `README.md` may be generated when docs output is enabled
- `docs/docs.go` may be generated as a Swagger stub when swag output is enabled
- `idl.go` is reserved for copied Go IDL input, not Proto input
- `pb/<service>/<service>.proto` remains the generated proto asset location when gRPC output is enabled
- `go.mod` is generated or updated as part of project initialization
- route prefix configuration should affect generated HTTP-facing artifacts consistently

## Compatibility Expectations By Output Area

### Stable expectations

These expectations should be preserved unless a deliberate breaking change is announced:

- generated projects keep the service -> endpoint -> transport layering model
- generated startup code remains recognizable and documented
- HTTP-only generation does not unexpectedly produce gRPC runtime requirements
- enabling gRPC produces corresponding transport and startup wiring
- enabling `-skill` produces machine-readable skill exposure support
- model/repository output remains aligned with documented generation flags
- Go IDL input may be copied into generated output as `idl.go`
- Proto input should not incorrectly create `idl.go`
- route prefix behavior should remain consistent across generated transport wiring and generated startup wiring
- generated `go.mod` should preserve documented module update behavior instead of being rewritten arbitrarily

### Semi-stable expectations

These areas are user-visible, but maintainers may refine them as long as behavior remains understandable and documented:

- formatting details inside generated files
- comments and docstrings in generated files
- helper function naming inside generated output when not documented as a stable API
- optional config and docs stubs
- the exact contents of docs stubs, as long as their purpose and overwrite behavior remain consistent

### Internal expectations

These should not be treated as compatibility requirements:

- exact placement of helper code inside a generated file
- exact template decomposition across `.tmpl` files
- exact internal codegen pipeline steps

## What Counts As A Breaking Change

For `microgen`, the following should be treated as breaking or near-breaking changes:

- removing or renaming a documented CLI flag
- changing the meaning of a documented CLI flag
- changing the meaning of a generated top-level directory
- changing generated layering so business logic no longer lands in `service/`
- changing the skill generation contract in a user-visible way
- making an HTTP-only generated project require gRPC or other previously optional runtime pieces
- changing generated output in a way that invalidates documented examples or upgrade assumptions
- changing file ownership expectations in a way that makes previously user-safe files generator-managed without clear migration guidance

These changes require:

1. documentation updates
2. test updates
3. migration guidance if users are likely to feel the break

## Compatibility Notes For Current Generation Flow

The current generator behavior includes several conventions that should be treated as externally meaningful.

### `go.mod` behavior

Current expectations:

- if `go.mod` does not exist, `microgen` creates one
- if `go.mod` exists and already matches the requested module path, it should not be rewritten unnecessarily
- if `go.mod` exists but the module line does not match the requested import path, the module line may be updated while preserving the rest of the file
- local replace-path behavior used by generated testdata and examples should remain intentional and documented

### IDL copy behavior

Current expectations:

- Go IDL input may be copied into generated output as `idl.go`
- Proto input should not be copied into `idl.go`
- users may rely on `idl.go` being present only for Go-IDL-driven project generation
- in current `append-service` extend mode, `idl.go` is treated as a generator-managed Go-IDL snapshot and may be refreshed from the full combined source contract

### Docs stub behavior

Current expectations:

- enabling docs or swag-related output may create a docs stub
- docs stubs are scaffolding, not the final source of truth for generated Swagger output
- once a real docs file exists, generator behavior should avoid unnecessarily overwriting it

### Route prefix behavior

Current expectations:

- route prefix configuration should affect generated HTTP transport code
- route prefix configuration should stay aligned with generated startup wiring
- maintainers should treat prefix drift between transport and startup output as a compatibility regression

### Extend mode behavior

Current expectations:

- `microgen extend -check -out <project>` is a supported read-only compatibility check for existing generated projects
- `microgen extend -idl <file> -out <project> -append-service <Name>` is the first supported incremental extend path
- `microgen extend -idl <file> -out <project> -append-model <Name>` is now also supported for generated projects that already have model output enabled
- `microgen extend -idl <file> -out <project> -append-middleware <Name[,Name...]>` is now supported for generator-owned endpoint middleware seams
- `extend -check` should not require an IDL file and should not mutate project files
- `extend -check` should exit with `0` when compatibility is ready and `2` when the scan succeeds but required compatibility seams are still missing
- current `append-service` support is limited to Go IDL inputs, not Proto inputs
- current `append-model` support is also limited to Go IDL inputs, not Proto inputs
- current `append-middleware` support is also limited to Go IDL inputs, not Proto inputs
- current `append-service` expects the provided Go IDL file to contain the full combined contract for both existing services and the new service being appended
- current `append-model` expects the provided Go IDL file to contain the full combined contract for both existing services/models and the new model being appended
- current `append-middleware` expects the provided Go IDL file to contain the full combined contract for existing services so endpoint middleware seams can be regenerated safely
- extend mode should scan the target project and fail clearly if required generator-owned aggregation files are missing
- extend check mode should report generator-owned compatibility seams and current append readiness clearly enough that users can decide whether regeneration is needed before append
- extend mode should create new generated files for the appended service while preserving existing user-owned implementation files such as `service/<svc>/service.go`
- extend mode should create new generated model/repository files for the appended model while preserving existing user-owned customization seams such as `model/<name>.go`
- extend mode should update only generator-owned endpoint middleware seams such as `endpoint/<svc>/generated_chain.go` when appending middleware and should preserve user-owned files such as `endpoint/<svc>/custom_chain.go`
- extend mode may update generator-owned aggregation files and generator-managed snapshots such as `cmd/generated_services.go`, `cmd/generated_routes.go`, `cmd/generated_runtime.go`, `skill/skill.go`, and `idl.go` when those outputs are part of the generated project shape

## What Does Not Automatically Count As Breaking

The following are usually safe if public behavior remains the same:

- refactoring internal templates
- reorganizing parser internals
- changing internal generator helper functions
- improving comments or formatting in generated code
- adding new optional flags that do not change existing default behavior

## Rules For Adding New Flags

When adding a new `microgen` flag:

1. choose a name that reflects output behavior, not implementation detail
2. preserve existing defaults unless a deliberate compatibility decision is made
3. document the flag in README or generator docs
4. add or update integration tests when the flag changes generated structure

## Rules For Template Changes

When changing templates:

1. ask whether the change is visible to users
2. if visible, decide whether it affects generated project conventions
3. if yes, treat it as a product change rather than an internal refactor
4. verify it with `TestMicrogenIntegration` and related generator tests

## Rules For Generated Layout Changes

When changing generated directories or major files:

1. preserve the current top-level meaning unless there is a strong reason not to
2. avoid moving user-expected code between `service`, `endpoint`, and `transport`
3. update docs immediately if the generated layout changes
4. assume users may have automation or onboarding docs built around the current layout

## Rules For Ownership Boundaries

When changing which files are generator-owned versus user-owned:

1. prefer introducing new generator-owned aggregation files over reclaiming user-edited files
2. do not make append or rerun behavior depend on overwriting business logic files
3. document which files extend mode may update
4. treat ownership-boundary changes as compatibility-sensitive behavior

## Validation Requirements

Changes touching `microgen` should normally be validated with:

```bash
make test-microgen
go test -race ./...
```

When generated examples are affected, also run:

```bash
make test-examples
```

## Current Compatibility Safety Nets

The repository already includes strong validation for `microgen`:

- generator package tests
- parser package tests
- dbschema tests
- `TestMicrogenIntegration`
- example smoke tests for generated-service behavior
- orchestration-focused generator tests for `idl.go`, `go.mod`, docs stub, and route prefix behavior

These tests should be treated as protection for public behavior, not just implementation correctness.

## Recommended Upgrade Policy

For future releases:

- prefer additive changes to generation behavior
- preserve default output shape whenever possible
- announce user-visible layout or flag changes clearly
- keep generated examples aligned with current output conventions

## Relationship To Other Docs

Use this guide together with:

- [FRAMEWORK_BOUNDARIES.md](FRAMEWORK_BOUNDARIES.md)
- [STABILITY.md](STABILITY.md)
- [PACKAGE_SURFACES.md](PACKAGE_SURFACES.md)
- [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md)
- [MICROGEN_OWNERSHIP.md](MICROGEN_OWNERSHIP.md)

Together they define framework scope, package stability, allowed usage, and generator compatibility expectations.
