# microgen Index

Purpose:
- Give maintainers and AI agents one short entry point for `microgen` design, compatibility, and ownership docs.

Read this when:
- You are working in `cmd/microgen/`.
- You need to know which `microgen` document is authoritative for a specific question.

See also:
- [MAINTAINER_GUIDE.md](MAINTAINER_GUIDE.md)
- [PROJECT_SNAPSHOT.md](PROJECT_SNAPSHOT.md)
- [MICROGEN_NEXT_PHASE.md](MICROGEN_NEXT_PHASE.md)

## Start Here

Read these first for almost any `microgen` task:

1. [PROJECT_SNAPSHOT.md](PROJECT_SNAPSHOT.md)
   Current state, recent work, next recommended task.
2. [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)
   What generated output currently promises.
3. One of the docs below, depending on the task.

## Pick By Question

### What is the next product direction?

- [MICROGEN_NEXT_PHASE.md](MICROGEN_NEXT_PHASE.md)

Use this when deciding roadmap direction, milestone order, CLI surface direction, or release-level scope.

### How should generated config and remote config work?

- [MICROGEN_CONFIG_DESIGN.md](MICROGEN_CONFIG_DESIGN.md)

Use this when changing generated `config/`, config loading behavior, remote-provider wiring, startup templates, or config-related integration tests.

### How should extend mode work?

- [MICROGEN_EXTEND_DESIGN.md](MICROGEN_EXTEND_DESIGN.md)

Use this when changing project scanning, append flows, extend CLI behavior, or generator-owned aggregation updates.

### Which files are generator-owned versus user-owned?

- [MICROGEN_OWNERSHIP.md](MICROGEN_OWNERSHIP.md)

Use this before changing rerun behavior, extend mode writes, or protected-file handling.

### Which generated behaviors are compatibility-sensitive?

- [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)

Use this before changing generated layout, generated file meaning, rerun expectations, config loading contracts, route behavior, or docs stub handling.

## Recommended Reading Paths

### Config Track

1. [MICROGEN_NEXT_PHASE.md](MICROGEN_NEXT_PHASE.md)
2. [MICROGEN_CONFIG_DESIGN.md](MICROGEN_CONFIG_DESIGN.md)
3. [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)

### Extend Track

1. [MICROGEN_NEXT_PHASE.md](MICROGEN_NEXT_PHASE.md)
2. [MICROGEN_EXTEND_DESIGN.md](MICROGEN_EXTEND_DESIGN.md)
3. [MICROGEN_OWNERSHIP.md](MICROGEN_OWNERSHIP.md)
4. [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)

### General Generator Safety

1. [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)
2. [MICROGEN_OWNERSHIP.md](MICROGEN_OWNERSHIP.md)
3. [PROJECT_WORKFLOW.md](PROJECT_WORKFLOW.md)

## Short Rule

If a `microgen` change affects generated files, startup behavior, extend mode, or rerun expectations, treat it as a product change and read the compatibility and ownership docs before editing templates.
