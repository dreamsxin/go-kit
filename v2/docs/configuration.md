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
  -> command-line flags
  -> Config.Validate
```

Both environment passes are the same full `ApplyEnv`. The first one runs before
the remote source so that `APP_REMOTE_*` can point at it; the second one runs
after, so environment always wins over remote values.

Validation runs after command-line overrides and before the logger, database,
middleware, and servers are created, so a misconfigured deployment fails fast.

Command-line flags use the loaded config as their defaults (`-http.addr`
defaults to `cfg.Server.HTTPAddr`), are applied to the config, and then go
through the same final validation. Flags are convenient local overrides; use
YAML or the environment for deployment configuration.

The generated `main` accepts the flags below, and which ones exist depends on how
the project was generated, the same way the environment keys do. The second
column is what each flag overrides.

```text
-config                          the config file to load
-http.addr                       Server.HTTPAddr
-grpc.addr                       Server.GRPCAddr
-db.dsn                          Database.DSN
-auto-migrate                    Database.AutoMigrate
```

Environment variables use the `APP_` prefix. `ApplyEnv` reads every key below.
Which of them exist depends on how the project was generated: `APP_DB_*` needs a
database, `APP_GRPC_ADDR` needs gRPC, and `APP_REMOTE_*` needs a remote config
mode.

```text
APP_HTTP_ADDR                    Server.HTTPAddr
APP_GRPC_ADDR                    Server.GRPCAddr
APP_READ_TIMEOUT                 Server.ReadTimeout
APP_READ_HEADER_TIMEOUT          Server.ReadHeaderTimeout
APP_WRITE_TIMEOUT                Server.WriteTimeout
APP_GRACEFUL_SHUTDOWN_TIMEOUT    Server.GracefulShutdownTimeout
APP_LOG_LEVEL                    Logging.Level
APP_LOG_FORMAT                   Logging.Format
APP_MIDDLEWARE_TIMEOUT           Middleware.Timeout
APP_DB_DRIVER                    Database.Driver
APP_DB_DSN                       Database.DSN
APP_DB_AUTO_MIGRATE              Database.AutoMigrate
APP_DB_MAX_OPEN_CONNS            Database.MaxOpenConns
APP_DB_MAX_IDLE_CONNS            Database.MaxIdleConns
APP_DB_CONN_MAX_LIFETIME         Database.ConnMaxLifetime
APP_DEBUG_ROUTES_ENABLED         Debug.RoutesEnabled
APP_DEBUG_PRINT_ROUTES           Debug.PrintRoutes
APP_REMOTE_ENABLED               Remote.Enabled
APP_REMOTE_PROVIDER              Remote.Provider
APP_REMOTE_ENDPOINT              Remote.Endpoint
APP_REMOTE_NAMESPACE             Remote.Namespace
APP_REMOTE_GROUP                 Remote.Group
APP_REMOTE_DATA_ID               Remote.DataID
APP_REMOTE_TIMEOUT               Remote.Timeout
APP_REMOTE_FALLBACK_TO_LOCAL     Remote.FallbackToLocal
```

This list is checked against the generated loader, so a key that is renamed or
dropped fails a test instead of going quiet.

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

In `hybrid` mode, an empty remote endpoint or data ID is treated as
local-only startup while fallback is enabled. This lets the generated default
config run locally and lets deployment inject remote coordinates later. The
`remote` mode remains strict and requires complete coordinates.

To change modes, regenerate with a different `-config-mode`.

## Secrets

Never commit credentials. Inject them through the deployment environment or an
application-owned provider.

The generated `main` never logs the config itself — it logs only
`config loaded path=<path>`. In `-db` projects the DSN is logged once through
`redactDSN`, which strips the credentials. Anything else you log about the
config is yours to redact.
