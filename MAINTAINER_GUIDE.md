# Maintainer Guide

Purpose:
- Give maintainers and AI agents one short, reliable starting point for working on this repository.

Read this when:
- You are about to change code in the repo itself.
- You are resuming an unfinished refactor.
- You need to decide which docs to read before editing.

See also:
- [PROJECT_SNAPSHOT.md](PROJECT_SNAPSHOT.md)
- [PROJECT_WORKFLOW.md](PROJECT_WORKFLOW.md)
- [DOCS_INDEX.md](DOCS_INDEX.md)

## Fast Start

Read these in order:

1. [PROJECT_SNAPSHOT.md](PROJECT_SNAPSHOT.md)
   Current repo state, active refactor line, recent verification, and the next recommended task.
2. [PROJECT_WORKFLOW.md](PROJECT_WORKFLOW.md)
   Which validation lane to use for runtime, generator, docs, or release work.
3. The nearest policy or design doc for your task
   Use the sections below to choose it quickly.

## Pick The Right Doc Set

### Runtime / Framework Change

Read:

1. [FRAMEWORK_BOUNDARIES.md](FRAMEWORK_BOUNDARIES.md)
2. [STABILITY.md](STABILITY.md)
3. [PACKAGE_SURFACES.md](PACKAGE_SURFACES.md)
4. [ANTI_PATTERNS.md](ANTI_PATTERNS.md)
5. [FRAMEWORK_ARCHITECTURE.md](FRAMEWORK_ARCHITECTURE.md)

Use this path when changing:

- `kit/`
- `endpoint/`
- `transport/`
- `sd/`
- `log/`
- `utils/`

### `microgen` Change

Read:

1. [MICROGEN_INDEX.md](MICROGEN_INDEX.md)
2. [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)
3. The relevant design doc:
   [MICROGEN_CONFIG_DESIGN.md](MICROGEN_CONFIG_DESIGN.md) or [MICROGEN_EXTEND_DESIGN.md](MICROGEN_EXTEND_DESIGN.md)

Use this path when changing:

- `cmd/microgen/parser/`
- `cmd/microgen/generator/`
- `cmd/microgen/templates/`
- `cmd/microgen/dbschema/`
- `cmd/microgen/main.go`

### Docs / Examples / Skills Change

Read:

1. [README.md](README.md)
2. [PROJECT_WORKFLOW.md](PROJECT_WORKFLOW.md)
3. [PR_CHECKLIST.md](PR_CHECKLIST.md)

Also inspect:

- `examples/README.md`
- `tools/SKILL.md`

### Release / Review Work

Read:

1. [PROJECT_WORKFLOW.md](PROJECT_WORKFLOW.md)
2. [PR_CHECKLIST.md](PR_CHECKLIST.md)
3. [PROJECT_SNAPSHOT.md](PROJECT_SNAPSHOT.md)

## Quick Rules

- Treat generated output shape as a product contract, not just an internal detail.
- Keep `service -> endpoint -> transport` layering intact.
- Prefer additive changes over broad rewrites when generator ownership matters.
- Read the nearest tests before editing code.
- Update docs in the same change when user-visible behavior moves.
- Update [PROJECT_SNAPSHOT.md](PROJECT_SNAPSHOT.md) after meaningful structural changes.

## If You Only Have 2 Minutes

1. Read [PROJECT_SNAPSHOT.md](PROJECT_SNAPSHOT.md)
2. Run `git status --short`
3. Read the nearest tests
4. Read one design/policy doc from the correct lane
5. Make the smallest change that preserves the current contracts
