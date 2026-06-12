# Release Policy

Purpose:
- Define how this framework moves from beta-quality internal adoption toward an industrial v1.0 release.

## Current Release Position

`go-kit` is preparing for:

```text
v1.5.0 Stable
```

Stable in this release means:

- the core `service -> endpoint -> transport` runtime layering is the supported product contract
- documented `kit`, `endpoint`, HTTP transport, service discovery, logging, and `microgen` CLI behavior are compatibility-sensitive
- generated unary HTTP/gRPC projects, config loading, extend mode, AI skill metadata, and Proto gRPC streaming generation are covered by release validation

All surfaces including interaction, interaction/mcp, WebSocket transport, and generated interaction adapters are now part of the stable scope.

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

### v0.9 AI Interaction

Scope:

- IR support for interaction method kinds
- gRPC server-stream, client-stream, and bidirectional-stream generation, implemented through generated server adapters, transport client helpers, SDK streaming clients, and success-path integration tests
- optional WebSocket transport after gRPC streaming stabilizes
- AI interaction runtime for sessions, events, tool calls, cancellation, and audit hooks
- generated examples and integration tests for streaming and WebSocket flows

Release posture:

- APIs are stable
- documented in standard release docs and generated README output

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

### v1.5.0 Stable

Scope:

- stable core runtime and documented `microgen` generation behavior
- generated Proto gRPC streaming support promoted from preview candidate to stable generated-output behavior for supported Proto stream shapes
- AI interaction runtime, WebSocket transport, and generated interaction adapters are promoted to stable scope

Release posture:

- suitable for stable adoption of the documented core framework and generator surfaces
- all packages follow standard changelog and migration practices

## v1.5.0 Stable Checklist

- [x] Stable scope includes interaction, interaction/mcp, and WebSocket surfaces
- [x] Stable package surfaces are documented in [STABILITY.md](STABILITY.md)
- [x] Generated output compatibility expectations are documented in [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)
- [x] `CHANGELOG.md` distinguishes stable and preview changes
- [x] gRPC streaming support is documented and integration-tested for success, errors, cancellation, backpressure, and slow-consumer behavior
- [x] AI interaction runtime has package tests, MCP endpoint tests, policy hook tests, and an example
- [x] Final release validation passes on the release commit
- [x] `CHANGELOG.md` has a `v1.5.0` section with date and stable/preview split
- [ ] Annotated `v1.5.0` tag points at the release commit

## v1.0 Checklist

- [ ] Public API freeze for stable packages in `STABILITY.md`
- [ ] Generated output compatibility freeze for documented `microgen` defaults
- [ ] `CHANGELOG.md` maintained for user-visible changes
- [ ] `MIGRATION.md` documents breaking or compatibility-sensitive moves
- [ ] gRPC streaming support documented and integration-tested for success, errors, cancellation, and slow-consumer behavior
- [ ] WebSocket transport documented and integration-tested as a supported stable surface
- [ ] AI interaction runtime documented and integration-tested
- [x] Auth, limits, and audit hooks documented for generated services
- [x] OpenTelemetry tracing/metrics guidance documented
- [ ] Release validation command set documented and repeatable

## Release Validation

Minimum release candidate loop:

```bash
go test ./cmd/microgen/... -count=1
go test ./tools/... -run "Test(Microgen|ReadmeQuickStartSmoke)" -count=1 -v
go test ./kit ./endpoint ./transport/... ./sd/... ./log ./utils -count=1
go test ./tools/... -run TestSKILL -count=1 -v
go test ./interaction/... ./examples/interaction_policy/... -count=1
git diff --check
```

For `v1.5.0`, this loop is the required release validation. WebSocket is not required because it is not in the stable release scope.

Current open release gaps before `v1.5.0`:

- Create an annotated `v1.5.0` tag.
- `interaction`, `interaction/mcp`, and WebSocket are now part of the stable scope.

Latest validation result for `v1.5.0`:

- `go test ./cmd/microgen/... -count=1`: passed
- `go test ./tools/... -run "Test(Microgen|ReadmeQuickStartSmoke)" -count=1 -v`: passed
- `go test ./kit ./endpoint ./transport/... ./sd/... ./log ./utils -count=1`: passed
- `go test ./tools/... -run TestSKILL -count=1 -v`: passed
- `go test ./interaction/... ./examples/interaction_policy/... -count=1`: passed
- `git diff --check`: passed
