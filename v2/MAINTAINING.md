# Maintainer Guide

This guide defines the normal workflow for changing go-kit v2. Keep durable
rules here; use issues or pull requests for temporary plans and progress notes.

## Before Editing

1. Read [ARCHITECTURE.md](ARCHITECTURE.md) for package ownership.
2. Read the nearest package README and tests.
3. Check `git status` and preserve unrelated worktree changes.
4. Decide whether the change affects runtime APIs, generated output, or both.

Do not edit v1 while implementing a v2-only change.

## Scope Rules

- Prefer existing packages and helpers.
- Keep service, endpoint, transport, and assembly responsibilities separate.
- Add general framework behavior only when multiple applications need it.
- Put provider-specific or deployment-specific behavior in optional integration
  packages.
- Do not add IAM, outbox, job platforms, object storage, secret platforms, or a
  complete transaction framework to core.

## Runtime Change Workflow

1. Add or update a focused behavioral test.
2. Change the package that owns the behavior.
3. Update examples if the recommended assembly changes.
4. Update the package README and top-level docs if the public contract changes.
5. Run package tests, race tests, then the full suite.

Typical commands:

```bash
cd v2
go test ./kit ./endpoint/... ./transport/...
go test -race ./kit ./interaction ./sd/...
go test ./...
```

## microgen Change Workflow

Generated output is a product surface. A template change is incomplete until
the generated project is verified.

1. Change parser/IR/generator/template code at the owning layer.
2. Add unit assertions for the generated contract.
3. Regenerate tracked fixtures through their tests.
4. Run the same generation test twice and verify the second run has no diff.
5. Generate into a temporary directory outside the module.
6. Run `go mod tidy` and `go test ./...` in that project.
7. Update [MICROGEN.md](MICROGEN.md) and the generated README template when
   user workflow changes.

Commands:

```bash
cd v2
go test ./cmd/microgen/...
go test ./tools -count=1
go test ./...
```

Generated Go must pass `go/format`. Generated non-Go text must have deterministic
line endings, trailing whitespace, and final newline behavior.

## Documentation Rules

The maintained top-level set is intentionally small:

- `README.md` and `README_zh.md`: first successful use;
- `MICROGEN.md`: generator behavior and ownership;
- `ARCHITECTURE.md`: package boundaries;
- `PRODUCTION.md`: deployment guidance;
- `MAINTAINING.md`: contributor workflow;
- `MIGRATION.md`: v1 to v2 changes;
- `RELEASE.md` and `CHANGELOG.md`: release policy and history.

Update the authoritative document instead of adding a roadmap, project snapshot,
design draft, or duplicate index. Temporary planning belongs in an issue or pull
request.

Documentation examples must compile against the current v2 API. Links must be
relative and must resolve on a case-sensitive filesystem.

## Review Checklist

### Behavior

- The change solves a general framework problem.
- Error, cancellation, timeout, and shutdown paths are covered.
- Mutable inputs/outputs are copied across concurrent ownership boundaries.
- No library code installs process signals or exits the process.

### API

- The package owning the behavior exposes the API.
- Invalid configuration returns an error where startup can handle it.
- Names describe actual behavior; avoid names that promise retry, streaming, or
  safety that is not implemented.
- Breaking changes are recorded in `CHANGELOG.md` and `MIGRATION.md` when needed.

### Generator

- Go IDL, Protobuf, and database paths affected by the change are tested.
- Generated ownership boundaries remain explicit.
- Repeat generation is deterministic.
- External generated projects build without an invalid local `replace`.
- Source database introspection remains read-only.

### Validation

- Focused tests pass.
- Relevant race tests pass.
- `go test ./...` passes.
- `git diff --check` passes.
- No temporary generated files remain.
- Documentation links resolve.

## Release Preparation

1. Confirm the module path is `github.com/dreamsxin/go-kit/v2`.
2. Review exported API and generated-output diffs.
3. Run the full validation matrix on a clean worktree.
4. Generate and build projects for each supported source mode.
5. Update `CHANGELOG.md`, `MIGRATION.md`, and `RELEASE.md`.
6. Tag v2 releases from the repository commit containing the `v2/go.mod` module.

See [RELEASE.md](RELEASE.md) for the compatibility policy.
