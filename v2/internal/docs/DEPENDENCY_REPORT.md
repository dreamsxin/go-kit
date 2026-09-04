# Dependency Closure Report
English | [简体中文](DEPENDENCY_REPORT_zh.md)

This report captures the current v2 dependency closure. The executable rules
live in `tools/dependency_boundaries_test.go`; this document records the reviewed
result and a diagnostic comparison baseline.

## Environment

- Current source: v2.8.0 release-candidate working tree
- Comparison baseline: historical v2 measurements (diagnostic only)
- Go: `go1.25.8`
- Platform: `windows/amd64`
- CGO: disabled
- Workspace: enabled for package-closure capture, disabled for the standalone
  HTTP comparison

Package counts include the standard library and platform-specific packages.
Non-standard counts include go-kit packages as well as provider packages.

## Core Entry Points

The root module declares provider dependencies for the optional packages it
ships. Each core entry point still resolves only the standard library and the
listed go-kit packages; provider costs appear only when an optional package is
imported.

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

Optional integrations ship in the same module. Their provider cost appears in a
build only when their packages are imported.

| Package entry point | Compiled packages | Non-standard packages | Provider family |
| --- | ---: | ---: | --- |
| `integrations/consul` | 207 | 18 | HashiCorp Consul API |
| `integrations/etcd` | 353 | 154 | etcd client v3 (pulls gRPC) |
| `integrations/grpc` | 307 | 114 | gRPC and Protobuf |
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

| Metric | Historical baseline | Current candidate | Change |
| --- | ---: | ---: | ---: |
| Compiled packages | 326 | 190 | 41.7% |
| Non-standard packages | 131 | 6 | 95.4% |
| Modules including the probe module | 73 | 2 | 97.3% |
| Forced build time | 9.205 s | 7.013 s | 23.8% |
| Windows binary size | 18,372,096 bytes | 8,898,048 bytes | 51.6% |

The HTTP-only probe resolves the root go-kit module and no optional provider
package. The repository also keeps separate workspace-only modules for examples
and tooling; they are not published or tagged.

## Recheck

Run the maintained gates before reviewing this report after architecture or
module changes:

```bash
go -C ./tools test . -run TestArchitectureDependencyGates -count=1
go -C ./tools test . -run TestKitHTTPAssemblyDoesNotResolveOptionalDependencies -count=1
go -C ./tools test . -run TestPublishableModulesDoNotUseLocalReplacements -count=1
```

Refresh measured values deliberately and record the Go version, platform,
source commit, build flags, and comparison tag. Do not treat cached build time
as a comparable result.
