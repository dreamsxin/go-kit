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
- `config/`
- `docs/`
- `model/`
- `repository/`
- `sdk/`
- `skill/`
- copied IDL or generated proto-related assets when relevant

Not every directory appears in every mode, but the meaning of these directories should remain consistent:

- `service/` contains business logic layer code
- `endpoint/` contains middleware-oriented service wrapping
- `transport/` contains protocol adaptation
- `cmd/` contains service startup and wiring
- `sdk/` contains generated client usage surface
- `skill/` contains AI-facing capability definitions

## Compatibility Expectations By Output Area

### Stable expectations

These expectations should be preserved unless a deliberate breaking change is announced:

- generated projects keep the service -> endpoint -> transport layering model
- generated startup code remains recognizable and documented
- HTTP-only generation does not unexpectedly produce gRPC runtime requirements
- enabling gRPC produces corresponding transport and startup wiring
- enabling `-skill` produces machine-readable skill exposure support
- model/repository output remains aligned with documented generation flags

### Semi-stable expectations

These areas are user-visible, but maintainers may refine them as long as behavior remains understandable and documented:

- formatting details inside generated files
- comments and docstrings in generated files
- helper function naming inside generated output when not documented as a stable API
- optional config and docs stubs

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

These changes require:

1. documentation updates
2. test updates
3. migration guidance if users are likely to feel the break

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

Together they define framework scope, package stability, allowed usage, and generator compatibility expectations.
