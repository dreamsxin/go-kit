# Dependency Closure Report

This report captures the dependency closure after the direct v2 architecture
refactor. The executable rules live in `tools/dependency_boundaries_test.go`;
this document records the reviewed result and the `v2.0.0` comparison required
by `ROADMAP.md`.

## Environment

- Current source: `be05d33370a2ac9f7cfbc266033a65df641ea90f`
- Published baseline: `v2.0.0` (`788c0538db872aab10dc910b9ed14e8a6a4745b0`)
- Go: `go1.25.8`
- Platform: `windows/amd64`
- CGO: disabled
- Workspace: enabled for package-closure capture, disabled for the standalone
  HTTP comparison

Package counts include the standard library and platform-specific packages.
Non-standard counts include go-kit packages as well as provider packages.

## Core Entry Points

The root runtime module has no third-party module requirements. Each entry point
therefore resolves only the standard library and the listed go-kit packages.

| Entry point | Compiled packages | Go-kit packages | Allowed go-kit dependency area |
| --- | ---: | ---: | --- |
| `endpoint` | 65 | 1 | `endpoint` |
| `transport/http/...` | 187 | 5 | `endpoint`, `transport`, `transport/http` |
| `sd/...` | 83 | 8 | `endpoint`, `sd` |
| `kit` | 187 | 5 | `endpoint`, `transport`, `transport/http`, `kit` |
| `interaction` | 65 | 1 | `interaction` |
| `interaction/mcp` | 184 | 2 | `interaction`, `interaction/mcp` |
| `security/http` | 181 | 1 | `security/http` |
| `observability/slog` | 77 | 3 | `endpoint`, protocol-neutral `transport`, `observability/slog` |

`go -C ./tools test . -run TestArchitectureDependencyGates` rejects imports
outside these areas, including provider SDKs and protocol crossings.

## Optional Entry Points

Optional integrations are separate modules. Their provider cost is visible only
when the corresponding module is selected.

| Module entry point | Compiled packages | Non-standard packages | Provider family |
| --- | ---: | ---: | --- |
| `integrations/circuitbreaker` | 67 | 3 | Sony Gobreaker |
| `integrations/consul` | 207 | 18 | HashiCorp Consul API |
| `integrations/grpc` | 307 | 114 | gRPC and Protobuf |
| `integrations/ratelimit` | 66 | 2 | `x/time/rate` adapter contract |
| `integrations/zap` | 196 | 13 | Uber Zap |
| `kit/grpc` | 303 | 110 | gRPC and Protobuf |
| `observability/otel` | 209 | 24 | OpenTelemetry |

The counts come from `go list -deps` under the repository workspace, so local
core contracts are used. Publishable modules are also tested with
`GOWORK=off`, and the release boundary test rejects local `replace` directives.

## Minimal HTTP Comparison

Both measurements build the same functional service from the published kit
example source. The current measurement replaces only the go-kit module with
the refactored local root. Each binary was built twice with:

```bash
GOWORK=off CGO_ENABLED=0 go build -a -trimpath
```

The table reports the shorter forced-build duration. Timing is diagnostic, not
an absolute release threshold; dependency and binary measurements are stable
closure signals.

| Metric | `v2.0.0` | Current refactor | Reduction |
| --- | ---: | ---: | ---: |
| Compiled packages | 326 | 190 | 41.7% |
| Non-standard packages | 131 | 6 | 95.4% |
| Modules including the probe module | 73 | 2 | 97.3% |
| Forced build time | 9.205 s | 7.013 s | 23.8% |
| Windows binary size | 18,372,096 bytes | 8,898,048 bytes | 51.6% |

The current two-module graph is the standalone probe plus the root go-kit
module. Optional integrations do not resolve for this HTTP-only service.

## Recheck

Run the maintained gates before reviewing this report after architecture or
module changes:

```bash
go -C ./tools test . -run TestArchitectureDependencyGates -count=1
go -C ./tools test . -run TestCoreModuleHasNoThirdPartyRequirements -count=1
go -C ./tools test . -run TestPublishableModulesDoNotUseLocalReplacements -count=1
```

Refresh measured values deliberately and record the Go version, platform,
source commit, build flags, and comparison tag. Do not treat cached build time
as a comparable result.
