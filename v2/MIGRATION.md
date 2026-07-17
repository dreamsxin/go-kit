# Migration Guide

Purpose:
- Track compatibility-sensitive changes and provide upgrade guidance as the framework moves toward v1.0.

## Current Status

There is no v1.0 compatibility promise yet. The current release posture is at `v1.6.0 Stable`.

`v1.6.0` stabilizes the documented core runtime and `microgen` generated-output behavior, and now includes `interaction`, `interaction/mcp`, and generated interaction adapters in the stable scope.

For now, treat these as compatibility-sensitive:

- documented runtime package APIs
- documented `microgen` CLI flags
- generated project layout described in README and compatibility docs
- generated config behavior
- generated skill/MCP metadata behavior
- extend-mode behavior for generated projects

## Upgrade Rules Before v1.0

When upgrading between pre-v1 releases:

1. Read `CHANGELOG.md`.
2. Read this file for any manual migration notes.
3. Regenerate a disposable project with the new `microgen` and compare generated layout before applying the change to a maintained project.
4. Run the generated project tests plus the smallest relevant framework validation loop.

## Known Migration Areas

### Generated Interaction Protocols

Generated Proto gRPC streaming is part of the `v1.6.0` generated-output contract for supported Proto stream shapes.

AI interaction adapters are now stable surfaces.

Expected migration risk:

- AI interaction adapters may change before v1.0
- AI interaction session/event envelope may change before v1.0

Migration guidance is documented in CHANGELOG.md.

### Generated Project Ownership

Generated projects distinguish user-owned files from generator-owned files.

Do not hand-edit generator-owned files such as:

- `cmd/generated_*.go`
- `endpoint/<svc>/generated_chain.go`
- `model/generated_*.go`
- `repository/generated_*.go`
- `client/`
- `sdk/`
- `skill/`
- generated `pb/` files

Use `microgen extend -check -out <project>` before applying extend operations to an existing generated project.
