# microgen Config And Remote Config Design

Purpose:
- Define the implementation-level design for generated config, env overrides, and remote config loading in `microgen`.

Read this when:
- You are changing generated `config/`, remote-provider behavior, config loading order, or config-related tests.

See also:
- [MICROGEN_INDEX.md](MICROGEN_INDEX.md)
- [MICROGEN_NEXT_PHASE.md](MICROGEN_NEXT_PHASE.md)
- [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)

This document defines the implementation-level design for generated configuration in `microgen`.

It is a deeper companion to:

- [MICROGEN_NEXT_PHASE.md](MICROGEN_NEXT_PHASE.md)
- [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)

Use this document when implementing:

- new config-related CLI flags
- generated `config/` package changes
- startup template changes in `cmd/main.go`
- remote-config loading support
- config-related integration tests

## Current State

Today the generator already produces a local config path when `-config=true`.

Current behavior:

- generates `config/config.yaml`
- generates `config/config.go`
- generated `cmd/main.go` can load config from `config/config.yaml`
- missing config files fall back to generated defaults
- generated `config/config.go` now also includes explicit `LoadLocal(...)`, `ApplyEnv(...)`, and `LoadRemote(...)` helpers behind the stable `Load(path string)` entry point
- generated `config/config.yaml` now also includes a `remote:` section with conservative defaults
- environment variable overrides are now a first shipped layer for scalar config fields under the `APP_` prefix
- remote loading is currently exposed as a no-op seam so generated projects remain runnable with local config only while provider integration is still pending

Current limitations:

- generated config still lives in one `config.go` file rather than separate `loader.go` / `env.go` / `remote.go` files
- the config schema still uses the current `server` / `logging` / `database` shape instead of the longer-term normalized shape proposed below
- remote loading does not yet call a concrete provider
- config structure is still tightly coupled to the current template set

## Goals

- keep local config as the default generated experience
- introduce env and remote config as additive layers
- avoid breaking existing `-config` behavior
- standardize the generated `config/` package around one public loading contract

## Non-Goals

- runtime hot reload in the first milestone
- dynamic schema negotiation with remote config centers
- supporting multiple remote providers in the first implementation
- exposing config internals as a stable framework package outside generated projects

## Proposed Generated Layout

Recommended target shape:

```text
config/
├─ config.go
├─ config.yaml
├─ loader.go
├─ local.go
├─ env.go
└─ remote.go
```

Recommended rollout by milestone:

### Milestone 1

- `config/config.go`
- `config/config.yaml`
- optional `config/loader.go`

### Milestone 2

- add `config/env.go`
- add `config/remote.go`

Not every file must exist immediately, but the generated package should move toward this shape rather than growing `config.go` indefinitely.

Current implementation note:

- the repo has now partially entered this rollout without splitting files yet:
  - env overrides are implemented inside generated `config.go`
  - remote loading has a generated seam inside `config.go`
  - a future refactor can still split those helpers into dedicated files without changing the startup-facing contract

## Public Generated API

The generated config package should converge on a small public surface:

```go
package config

type Config struct {
	Service ServiceConfig `yaml:"service"`
	HTTP    HTTPConfig    `yaml:"http"`
	GRPC    GRPCConfig    `yaml:"grpc"`
	Log     LogConfig     `yaml:"log"`
	DB      DBConfig      `yaml:"db"`
	Debug   DebugConfig   `yaml:"debug"`
	Remote  RemoteConfig  `yaml:"remote"`
}

func Default() *Config
func Load(path string, opts ...Option) (*Config, error)
```

This API may still evolve, but the generated startup code should stop depending on template-local helper behavior and instead depend on the generated config package contract.

## Proposed Config Structure

### Top-Level Sections

Recommended sections:

- `service`
- `http`
- `grpc`
- `log`
- `db`
- `middleware`
- `debug`
- `remote`

Suggested shape:

```yaml
service:
  name: "userservice"
  env: "dev"

http:
  addr: ":8080"
  read_timeout: "15s"
  write_timeout: "15s"
  graceful_shutdown_timeout: "30s"

grpc:
  enabled: true
  addr: ":8081"

log:
  level: "info"
  format: "json"

db:
  driver: "mysql"
  dsn: ""
  max_open_conns: 20
  max_idle_conns: 5
  conn_max_lifetime: "1h"

middleware:
  circuit_breaker:
    enabled: true
    failure_threshold: 5
    timeout: "60s"
  rate_limit:
    enabled: true
    requests_per_second: 100

debug:
  routes_enabled: true
  print_routes: true

remote:
  enabled: false
  provider: ""
  endpoint: ""
  namespace: ""
  group: ""
  data_id: ""
  timeout: "5s"
  fallback_to_local: true
```

### Compatibility Guidance

The current `server` and `logging` shape in generated config can evolve, but if changed:

- the change must be documented in release notes
- template and integration tests must be updated together
- migration guidance should be provided if generated examples or docs are affected

Recommended migration direction:

- normalize names gradually
- prefer additive aliases or controlled regeneration over abrupt schema churn

## Loading Pipeline

Recommended generated loading pipeline:

1. create defaults through `Default()`
2. load local file if present
3. apply env overrides
4. optionally load remote config
5. validate final config

Suggested internal flow:

```go
cfg := Default()
cfg = mergeFile(cfg, path)
cfg = applyEnv(cfg)
cfg = applyRemote(ctx, cfg)
validate(cfg)
```

## Local File Loading

Local file loading should preserve current behavior:

- missing file is not fatal
- malformed file is fatal
- unspecified values inherit defaults

This keeps generated services easy to run immediately after generation.

## Environment Variable Overrides

Env support should be explicit and predictable.

Recommended naming:

- prefix by service or generic generated prefix
- use upper snake case

Examples:

- `APP_HTTP_ADDR`
- `APP_GRPC_ADDR`
- `APP_LOG_LEVEL`
- `APP_DB_DSN`
- `APP_REMOTE_ENABLED`

Recommended first milestone behavior:

- env overrides are simple scalar overrides
- no complex nested list parsing
- env applies after local file load

## Remote Config Design

### Required Properties

Remote config should be:

- optional
- provider-driven
- layered on top of local defaults
- able to fall back to local config by default

### RemoteConfig Shape

Suggested generated struct:

```go
type RemoteConfig struct {
	Enabled         bool          `yaml:"enabled"`
	Provider        string        `yaml:"provider"`
	Endpoint        string        `yaml:"endpoint"`
	Namespace       string        `yaml:"namespace"`
	Group           string        `yaml:"group"`
	DataID          string        `yaml:"data_id"`
	Timeout         time.Duration `yaml:"timeout"`
	FallbackToLocal bool          `yaml:"fallback_to_local"`
}
```

This is intentionally provider-neutral.

Provider-specific fields can be added later if the first provider needs them.

## Loader Abstraction

Recommended internal interface:

```go
type RemoteLoader interface {
	Load(ctx context.Context, base *Config) (*Config, error)
}
```

Supporting helpers:

- `LoadLocal(path string) (*Config, error)`
- `ApplyEnv(cfg *Config) error`
- `LoadRemote(ctx context.Context, cfg *Config) (*Config, error)`

This keeps startup logic readable and testable.

## CLI Additions

Recommended additive flags:

- `-config-mode file|hybrid|remote`
- `-remote-provider <name>`

Optional alternative:

- `-remote-config`
- `-remote-provider <name>`

Recommended choice:

- keep `-config` as-is
- add `-config-mode` only when remote config is implemented
- reserve `hybrid` to mean local + env + remote layering

### CLI Semantics

- `-config=false`
  no generated config package or config file
- `-config=true`
  generate local config support
- `-config-mode=file`
  local file plus env
- `-config-mode=hybrid`
  local file plus env plus optional remote
- `-config-mode=remote`
  remote intended as primary source, but exact strictness should still be opt-in if added later

## Template Changes

### `cmd/microgen/main.go`

Add parsing and plumbing for:

- `ConfigMode`
- `RemoteProvider`

These should become generator options, not template-only ad hoc variables.

### `cmd/microgen/generator/project_files.go`

Update config generation helpers to:

- render the new config schema
- render any additional config source files
- keep file creation phased and explicit

### `cmd/microgen/templates/config.tmpl`

Needs to:

- move toward the new config schema
- include `remote` settings only when config support is enabled
- keep defaults friendly for immediate local startup

### `cmd/microgen/templates/config_code.tmpl`

Needs to:

- split or prepare for split into smaller config package files
- expose a stable generated API
- support env and remote loading without turning one file into another monolith

### `cmd/microgen/templates/main.tmpl`

Needs to:

- depend on config package loading rather than inline config assumptions
- use loaded config consistently for HTTP, gRPC, DB, logging, debug routes, and swagger host behavior

## Generator Options

Recommended additions to generator options:

```go
type Options struct {
	WithConfig     bool
	ConfigMode     string
	RemoteProvider string
}
```

Recommended validation:

- unknown `ConfigMode` should fail fast
- unknown `RemoteProvider` should fail fast when remote config is requested
- remote-provider flags should be ignored or rejected clearly when config generation is disabled

## Testing Plan

### Unit Tests

- config option validation in CLI parsing
- generator option validation for config mode and remote provider
- config template data wiring tests

### Integration Tests

Add or extend `TestMicrogenIntegration` cases for:

- generated local-config project still builds and runs
- env overrides affect generated runtime config
- hybrid config project builds and runs without remote infra when fallback is enabled
- provider-enabled generation creates expected config artifacts

### Regression Checks

- current `-config=false` path must still work
- current generated local config path must remain runnable
- config changes must not break proto, from-db, or minimal-runtime generation

## Open Decisions

- which config schema names to preserve versus normalize
- which remote provider should be first
- whether remote config should merge deeply or replace sections wholesale
- whether generated config loading should expose options or keep a fixed generated flow

## Definition Of Done

This design is implemented well when:

- generated config has one stable entry point
- local config remains easy and default
- env overrides are predictable
- remote config is additive and optional
- startup templates are simpler, not more tangled
- integration tests prove the generated project behavior end to end
