# Release Policy

Purpose:
- Define how this framework moves from beta-quality internal adoption toward an industrial v1.0 release.

## Current Release Position

`go-kit` should currently be treated as:

```text
v0.8 Beta
```

This means:

- core runtime layering and `microgen` generation are usable for internal projects, prototypes, and controlled pilots
- generated output and documented CLI flags are compatibility-sensitive
- advanced interaction protocols, security policy, observability policy, and release governance are still being hardened before v1.0

Do not describe the project as an industrial v1.0 framework until the v1.0 checklist below is complete.

## Version Targets

### v0.8 Beta

Scope:

- unary HTTP and unary gRPC runtime support
- `microgen` generation from Go IDL, Proto, and DB schema
- generated README, SDK, client, config, and AI skill metadata
- extend mode for generated projects
- integration coverage for generated project build/run paths

Release posture:

- suitable for internal production trials where the owning team accepts framework evolution
- not yet a long-term compatibility promise

### v0.9 AI Interaction Preview

Scope:

- IR support for interaction method kinds
- gRPC server-stream, client-stream, and bidirectional-stream generation, currently previewed through generated server adapters, transport client helpers, SDK streaming clients, and success-path integration tests
- optional WebSocket transport preview after gRPC streaming stabilizes
- AI interaction runtime preview for sessions, events, tool calls, cancellation, and audit hooks
- generated examples and integration tests for streaming and WebSocket flows

Release posture:

- preview APIs may change
- must be clearly marked in docs and generated README output

### v1.0 Industrial

Scope:

- stable API and generated-output compatibility contract
- release notes and migration notes for every compatibility-affecting change
- production security hooks for authn/authz, request limits, and generated project hardening
- OpenTelemetry tracing and metrics guidance
- streaming, WebSocket, and AI interaction lifecycle tests
- CI matrix covering supported Go versions and required toolchains

Release posture:

- stable public framework release
- breaking changes require a documented migration path

## v1.0 Checklist

- [ ] Public API freeze for stable packages in `STABILITY.md`
- [ ] Generated output compatibility freeze for documented `microgen` defaults
- [ ] `CHANGELOG.md` maintained for user-visible changes
- [ ] `MIGRATION.md` documents breaking or compatibility-sensitive moves
- [ ] gRPC streaming support documented and integration-tested for success, errors, cancellation, and slow-consumer behavior
- [ ] WebSocket transport documented and integration-tested if enabled as a supported preview surface
- [ ] AI interaction runtime documented and integration-tested
- [ ] Auth, limits, and audit hooks documented for generated services
- [ ] OpenTelemetry tracing/metrics guidance documented
- [ ] Release validation command set documented and repeatable

## Release Validation

Minimum release candidate loop:

```bash
go test ./cmd/microgen/... -count=1
go test ./tools/... -run "Test(Microgen|ReadmeQuickStartSmoke)" -count=1 -v
go test ./kit ./endpoint ./transport/... ./sd/... ./log ./utils -count=1
go test ./tools/... -run TestSKILL -count=1 -v
git diff --check
```

Before v1.0, add streaming, WebSocket, and AI interaction integration suites to this loop.

Current open release gaps:

- `v1.5.0-preview.1` was released on 2026-05-17 for the gRPC streaming preview and initial AI interaction runtime preview. It should not be described as an industrial stable surface.
- AI interaction runtime production adapters, generated-project orientation, auth/audit examples, and hardening remain open.
- WebSocket remains optional and should not block v1.0 unless it becomes an accepted supported preview surface.
- Security hardening, OpenTelemetry guidance, and compatibility-freeze docs still need final release work.

Latest validation result for `v1.5.0-preview.1`:

- `go test ./cmd/microgen/... -count=1`: passed
- `go test ./tools/... -run "Test(Microgen|ReadmeQuickStartSmoke)" -count=1 -v`: passed
- `go test ./kit ./endpoint ./transport/... ./sd/... ./log ./utils -count=1`: passed
- `go test ./tools/... -run TestSKILL -count=1 -v`: passed
- `go test ./interaction/... -count=1`: passed
- `git diff --check`: passed
