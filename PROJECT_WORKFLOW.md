# Project Workflow

Purpose:
- Define the recommended development and validation workflow for work inside this repository.

Read this when:
- You are about to change runtime packages, `microgen`, docs, examples, or prepare a release.

See also:
- [MAINTAINER_GUIDE.md](MAINTAINER_GUIDE.md)
- [PROJECT_SNAPSHOT.md](PROJECT_SNAPSHOT.md)
- [PR_CHECKLIST.md](PR_CHECKLIST.md)

This document describes the recommended development workflow for the `go-kit` repository itself.

Important distinction:

- `README.md` mainly explains how to use `go-kit` and what a generated service looks like.
- This file explains how to work on the framework, the `microgen` generator, examples, and validation tooling inside this repo.

For fast session re-entry, read [PROJECT_SNAPSHOT.md](PROJECT_SNAPSHOT.md) before diving into package code.

## Repository Structure

The repository is organized around four development areas:

- `kit/`, `endpoint/`, `transport/`, `sd/`, `log/`, `utils/`
  Runtime framework packages used by services.
- `cmd/microgen/`
  Code generator for IDL, Proto, and database-driven service scaffolding.
- `examples/`
  Runnable examples and learning path from minimal usage to production-style services.
- `tools/`
  Integration tests, example smoke tests, and `SKILL.md` verification.

## Choose The Right Workflow

Before changing code, classify the task into one primary lane:

1. Runtime/framework change
   Examples: middleware, endpoint behavior, HTTP/gRPC transport, service discovery, logging helpers.
2. Generator change
   Examples: parser, templates, DB schema introspection, generated project layout.
3. Docs/examples/skill change
   Examples: `README.md`, `examples/README.md`, `tools/SKILL.md`, tutorial snippets.
4. Release/regression verification
   Examples: broad validation before merge or release.

Keeping one lane as the primary focus helps avoid expensive full-repo validation too early.

## Workflow 1: Runtime Framework Changes

Use this when modifying:

- `kit/`
- `endpoint/`
- `transport/`
- `sd/`
- `log/`
- `utils/`

Recommended sequence:

1. Read the target package tests first.
2. Make the smallest change that preserves the transport-endpoint-service separation.
3. Run focused package tests.
4. Run the most relevant examples.
5. Finish with a wider repository test pass.

Special checks for runtime work:

- prefer construction-time or composition-time contract failures over delayed request-time panics when misuse can be detected earlier
- keep typed endpoint behavior symmetric where possible, especially around request/response type assertion errors
- when tightening HTTP/gRPC parity, document the intended shared semantic contract as part of the same change

Suggested commands:

```bash
go test ./kit ./endpoint ./transport/... ./sd/... ./log ./utils
go test ./examples/basic/... ./examples/transport/...
go test -race ./...
```

Use examples as behavioral checks:

- `examples/quickstart` for minimal HTTP behavior
- `examples/best_practice` for production middleware composition
- `examples/middleware` for endpoint middleware interactions
- `examples/sd` for service discovery wiring

## Workflow 2: microgen Changes

Use this when modifying:

- `cmd/microgen/parser/`
- `cmd/microgen/generator/`
- `cmd/microgen/dbschema/`
- `cmd/microgen/templates/`
- `cmd/microgen/main.go`

Recommended sequence:

1. Run focused tests for the subpackage you touched.
2. Run generator-level tests.
3. Run `tools` integration tests to verify end-to-end generation.
4. Inspect or run `examples/microgen_skill` if the change affects generated output shape.

Suggested commands:

```bash
go test ./cmd/microgen/...
go test ./tools/... -run TestMicrogenIntegration -v
go test ./tools/... -run TestAllExamples -v
```

Special checks for generator work:

- Parser changes must preserve support for Go IDL and `.proto` inputs.
- Template changes should be treated as public API changes for generated projects.
- DB schema changes should be validated against at least one realistic DSN path when possible.

## Workflow 3: Docs, Examples, And Skill Changes

Use this when modifying:

- `README.md`
- `examples/README.md`
- `tools/SKILL.md`
- example programs and tutorial snippets

Recommended sequence:

1. Keep docs aligned with the actual package API and examples.
2. Verify code snippets compile or are covered by existing tests.
3. Prefer examples that are already exercised by tests.

Suggested commands:

```bash
go test ./tools/... -run TestSKILL -v
go test ./examples/...
```

Notes:

- `tools/skill_test.go` is the highest-value safety net for `SKILL.md`.
- If a README snippet is not covered by tests, prefer adapting it from an existing example.

## Workflow 4: Release And Regression Pass

Use this before merging broad changes or preparing a release.

Recommended sequence:

1. Build the repository.
2. Run full tests.
3. Run integration-heavy tool tests.
4. Run lint if available in the environment.
5. Review docs and examples for drift.

Suggested commands:

```bash
go build ./...
go test -race ./...
go test ./tools/... -v
golangci-lint run
```

## Practical Validation Matrix

Use this matrix to avoid over-testing too early while still catching regressions.

| Change type | Minimum validation | Strong validation |
|-------------|--------------------|-------------------|
| Runtime package | Targeted package tests | Targeted tests + examples + `go test -race ./...` |
| `microgen` parser/template | `go test ./cmd/microgen/...` | Add `./tools/...` integration tests |
| Example only | Example package test/run | Example tests + related tool smoke tests |
| README / `SKILL.md` | Manual review | `go test ./tools/... -run TestSKILL -v` |

## Team Conventions

- Keep service business logic framework-agnostic when changing examples or generated output.
- Treat generated layout changes as user-facing behavior changes.
- Update docs and examples in the same change when public APIs move.
- Prefer focused tests first, then broaden outward.
- If `README.md`, `examples/README.md`, and `tools/SKILL.md` disagree, align all three before finishing.
- After meaningful refactor progress, update `PROJECT_SNAPSHOT.md` so the next coding session can resume quickly.

## Recommended Daily Loop

For most tasks, this is the best default loop:

1. Identify the primary lane.
2. Read `PROJECT_SNAPSHOT.md`, then the nearest tests and example.
3. Implement the smallest viable change.
4. Run focused validation.
5. Run one broader regression pass.
6. Update docs only where the user-facing behavior changed.

## AI Session Workflow

When resuming with an AI coding agent, use this sequence:

1. Read `PROJECT_SNAPSHOT.md` for current status and next steps.
2. Read `README.md` for the product story.
3. Read `PROJECT_WORKFLOW.md` for the correct validation lane.
4. Read the nearest package tests before editing code.
5. Update `PROJECT_SNAPSHOT.md` before ending the session if the project state materially changed.

## Useful Existing Entry Points

The repo already includes some helpful commands in `Makefile`:

- `make build`
- `make test`
- `make coverage`
- `make lint`
- `make test-runtime`
- `make test-microgen`
- `make test-docs`
- `make test-examples`
- `make verify`
- `make gen`
- `make gen-http`
- `make gen-grpc`
- `make gen-full`

These are helpful top-level shortcuts, but package-level `go test` commands are still the best choice during active iteration.
