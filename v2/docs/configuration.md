# Configuration

English | [简体中文](configuration_zh.md)

Configuration only exists when a project is generated with `-config`. This page
is a reference for precedence, validation, and custom sections.

## Quick Answer

- Use YAML for deployment defaults and `APP_*` variables for deployment-time
  overrides.
- `file`, `hybrid`, and `remote` are generator-time modes, not runtime values.
- `Config.Validate` runs before runtime wiring. Generated command-line flags are
  local overrides and are applied after that validation.
- `config/custom.go` is user-owned and must keep `SetDefaults`, `ApplyEnv() error`,
  and `Validate() error` on `*CustomConfig`.

## Precedence

```text
defaults               (Default)
  -> local YAML        (LoadLocal; a missing file is non-fatal)
  -> environment       (ApplyEnv)
  -> optional remote   (LoadRemote)
  -> environment again (ApplyEnv)
  -> Config.Validate
  -> command-line flags
```

Both environment passes are the same full `ApplyEnv`. The first one runs before
the remote source so that `APP_REMOTE_*` can point at it; the second one runs
after, so environment always wins over remote values.

Validation runs before the logger, database, middleware, and servers are
created, so a misconfigured deployment fails fast.

Command-line flags are applied *after* validation, because `flag` uses the
loaded config as its default values (`-http.addr` defaults to
`cfg.Server.HTTPAddr`). This means a flag value is not validated: pass
`-http.addr=""` and the process fails at listen time, not at load time. Flags
are meant for local overrides; use YAML or the environment for deployments.

The generated `main` accepts `-config`, `-http.addr`, plus `-grpc.addr` when the
project has gRPC and `-db.dsn` / `-auto-migrate` when it has a database.

Environment variables use the `APP_` prefix:

```text
APP_HTTP_ADDR
APP_LOG_LEVEL
APP_LOG_FORMAT
APP_DB_DSN
APP_DB_AUTO_MIGRATE
APP_REMOTE_ENABLED
```

## Custom sections

Application-specific settings belong in the user-owned `config/custom.go`. The
three hooks are methods on `*CustomConfig`, not on `*Config`, and the generated
loader calls them by those exact signatures:

```go
type CustomConfig struct {
	FeatureFlags map[string]bool `yaml:"feature_flags"`
}

func (cfg *CustomConfig) SetDefaults() {
	cfg.FeatureFlags = map[string]bool{"new_checkout": false}
}

func (cfg *CustomConfig) ApplyEnv() error {
	if os.Getenv("APP_FEATURE_NEW_CHECKOUT") == "true" {
		cfg.FeatureFlags["new_checkout"] = true
	}
	return nil
}

func (cfg *CustomConfig) Validate() error { return nil }
```

Keep all three methods and keep their signatures: `SetDefaults()` returns
nothing, `ApplyEnv() error` and `Validate() error` return an error. The
generated `config/config.go` and `config/env.go` call them, so a different
receiver or signature either fails to compile or silently never runs.

YAML and remote config merge into `custom`. Full regeneration never overwrites
this file.

## Generated sections

The generated `Config` carries these sections; the keys are the YAML fields,
and final environment overrides follow the `APP_` prefix shown above:

| Section | Keys | Purpose |
| --- | --- | --- |
| `server` | `http_addr`, `grpc_addr`, `read_timeout`, `read_header_timeout`, `write_timeout`, `graceful_shutdown_timeout` | listeners and timeouts; `write_timeout` stays `0` for streaming. `grpc_addr` is generated only for projects with gRPC |
| `logging` | `level`, `format` | slog level and format (`json` or `console`) |
| `database` | `driver`, `dsn`, `auto_migrate`, `max_open_conns`, `max_idle_conns`, `conn_max_lifetime` | connection and pool tuning; generated only with `-db` |
| `middleware` | `timeout` | generated endpoint middleware |
| `debug` | `routes_enabled`, `print_routes` | route debugging switches |
| `remote` | `enabled`, `provider`, `endpoint`, `namespace`, `group`, `data_id`, `timeout`, `fallback_to_local` | remote configuration source |
| `custom` | application-defined | application-owned section |

Sections are generation-time, not runtime: the `database` and `grpc_addr`
fields do not exist in the struct unless the project was generated with `-db`
and with a gRPC transport. Adding the YAML key to a project without them has no
effect.

For failure symptoms related to these settings, see
[troubleshooting](troubleshooting.md).

## Modes

The mode is a generation-time choice — `microgen -config-mode=<mode>` — that
decides which loader code is emitted, not a runtime setting:

| Mode | Behavior |
| --- | --- |
| `file` | local file plus environment; no remote loader is generated |
| `hybrid` | remote loading enabled with local fallback |
| `remote` | remote loading required; startup fails on remote error |

To change modes, regenerate with a different `-config-mode`.

## Secrets

Never commit credentials. Inject them through the deployment environment or an
application-owned provider.

The generated `main` never logs the config itself — it logs only
`config loaded path=<path>`. In `-db` projects the DSN is logged once through
`redactDSN`, which strips the credentials. Anything else you log about the
config is yours to redact.
