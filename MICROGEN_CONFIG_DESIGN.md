# microgen Config And Remote Config Design

Purpose:
- Define the implementation-level design for generated config, env overrides, and remote config loading in `microgen`.

Read this when:
- You are changing generated `config/`, remote-provider behavior, config loading order, or config-related tests.

See also:
- [MICROGEN_INDEX.md](MICROGEN_INDEX.md)
- [MICROGEN_DESIGN.md](MICROGEN_DESIGN.md)
- [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)

This document defines the implementation-level design for generated configuration in `microgen`.

It is a deeper companion to:

- [MICROGEN_DESIGN.md](MICROGEN_DESIGN.md)
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
- generates `config/config.go`, `config/local.go`, `config/env.go`, `config/remote.go`, and `config/loader.go`
- generated `cmd/main.go` can load config from `config/config.yaml`
- missing config files fall back to generated defaults
- generated config now exposes explicit `LoadLocal(...)`, `ApplyEnv(...)`, and `LoadRemote(...)` helpers behind the stable `Load(path string)` entry point
- generated `config/config.yaml` now also includes a `remote:` section with conservative defaults
- environment variable overrides are now a first shipped layer for scalar config fields under the `APP_` prefix
- remote loading now has a first real provider-backed implementation for `provider: consul`
- CLI generation now also supports `-config-mode file|hybrid|remote` and `-remote-provider consul`

Current limitations:

- the config schema still uses the current `server` / `logging` / `database` shape instead of the longer-term normalized shape proposed below
- only one remote provider is currently implemented
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

## Generated Layout

Current generated shape:

```text
config/
├─ config.go
├─ config.yaml
├─ loader.go
├─ local.go
├─ env.go
└─ remote.go
```

Implementation note:

- the generated config package already follows this split-file layout while preserving the same startup-facing contract:
  - `config.go` for types and defaults
  - `local.go` for YAML loading
  - `env.go` for environment overrides
  - `remote.go` for provider-backed remote loading
  - `loader.go` for the shared `Load(...)` entry point

## Public Generated API

The generated config package currently centers on a small startup-facing surface:

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

Implementation rule:

- generated startup code should depend on the generated config package contract rather than template-local helper behavior

## Config Structure

### Top-Level Sections

Current logical sections:

- `service`
- `http`
- `grpc`
- `log`
- `db`
- `middleware`
- `debug`
- `remote`

Representative shape:

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

### Schema Compatibility Guidance

The current `server` and `logging` shape in generated config can evolve, but if changed:

- the change must be documented in release notes
- template and integration tests must be updated together
- migration guidance should be provided if generated examples or docs are affected

Current implementation constraint:

- schema cleanup should happen through controlled, documented changes rather than silent template churn

## Loading Pipeline

Current generated loading pipeline:

1. create defaults through `Default()`
2. load local file if present
3. apply env overrides
4. optionally load remote config
5. validate final config

Implementation shape:

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

Env support is intentionally explicit and predictable.

Current naming direction:

- use the generated `APP_` prefix
- use upper snake case

Examples:

- `APP_HTTP_ADDR`
- `APP_GRPC_ADDR`
- `APP_LOG_LEVEL`
- `APP_DB_DSN`
- `APP_REMOTE_ENABLED`

Current implementation constraints:

- env overrides are simple scalar overrides
- no complex nested list parsing
- env applies after local file load

## Remote Config

### Required Properties

Remote config is currently expected to be:

- optional
- provider-driven
- layered on top of local defaults
- able to fall back to local config by default

### RemoteConfig Shape

Current generated struct shape:

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

Implementation rule:

- keep the config struct provider-neutral unless a concrete provider requirement justifies widening it

## Loader Abstraction

Current internal abstraction direction:

```go
type RemoteLoader interface {
	Load(ctx context.Context, base *Config) (*Config, error)
}
```

Supporting helpers:

- `LoadLocal(path string) (*Config, error)`
- `ApplyEnv(cfg *Config) error`
- `LoadRemote(ctx context.Context, cfg *Config) (*Config, error)`

Implementation rule:

- keep startup logic readable and testable by routing file, env, and remote loading through explicit helpers

## CLI Surface

Current additive flags:

- `-config-mode file|hybrid|remote`
- `-remote-provider <name>`

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
  remote intended as primary source, with failure behavior controlled by generated config defaults

## Implementation Touchpoints

### `cmd/microgen/main.go`

Current responsibility:

- parse and validate `ConfigMode`
- parse and validate `RemoteProvider`
- map those values into generator options

### `cmd/microgen/generator/project_files.go`

Current responsibility:

- render config package files
- render config YAML defaults
- keep config artifact creation explicit and phased

### `cmd/microgen/templates/config*.tmpl`

Current responsibility:

- render the generated config schema and defaults
- render split config package files
- keep local startup friendly while exposing env and remote seams

### `cmd/microgen/templates/main.tmpl`

Current responsibility:

- depend on config package loading rather than inline config assumptions
- use loaded config consistently for HTTP, gRPC, DB, logging, debug routes, and swagger host behavior

## Generator Options

Current option shape:

```go
type Options struct {
	WithConfig     bool
	ConfigMode     string
	RemoteProvider string
}
```

Current validation rules:

- unknown `ConfigMode` should fail fast
- unknown `RemoteProvider` should fail fast when remote config is requested
- remote-provider flags should be ignored or rejected clearly when config generation is disabled

## Testing Guidance

### Unit Tests

- config option validation in CLI parsing
- generator option validation for config mode and remote provider
- config template data wiring tests

### Integration Tests

The important integration checks are:

- generated local-config project still builds and runs
- env overrides affect generated runtime config
- hybrid config project builds and runs without remote infra when fallback is enabled
- provider-enabled generation creates expected config artifacts

### Regression Checks

- current `-config=false` path must still work
- current generated local config path must remain runnable
- config changes must not break proto, from-db, or minimal-runtime generation

## Remaining Implementation Gaps

The main remaining gaps in this area are now implementation cleanup items rather than product-shape questions:

- tighten validation around unsupported provider/mode combinations
- keep strict remote-failure behavior clearly covered by integration tests
- decide whether schema normalization is worth a compatibility-managed change
- avoid letting config templates drift back toward one large monolith

## Definition Of Done

This design is implemented well when:

- generated config has one stable entry point
- local config remains easy and default
- env overrides are predictable
- remote config is additive and optional
- startup templates are simpler, not more tangled
- integration tests prove the generated project behavior end to end
