# Configuration

English | [简体中文](configuration_zh.md)

Configuration only exists when a project is generated with `-config`. This
page covers the precedence chain and application-owned custom sections.

## Precedence

```text
defaults
  -> local YAML
  -> environment bootstrap for remote connection settings
  -> optional remote config
  -> final environment overrides
  -> Config.Validate
```

Validation runs before the logger, database, middleware, and servers are
created, so a misconfigured deployment fails fast.

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

Application-specific settings belong in the user-owned `config/custom.go`:

```go
type CustomConfig struct {
	FeatureFlags map[string]bool `yaml:"feature_flags"`
}

func (c *Config) SetDefaults() {
	c.Custom.FeatureFlags = map[string]bool{"new_checkout": false}
}

func (c *Config) ApplyEnv() {
	if v := os.Getenv("APP_FEATURE_NEW_CHECKOUT"); v == "true" {
		c.Custom.FeatureFlags["new_checkout"] = true
	}
}
```

YAML and remote config merge into `custom`; `SetDefaults`, `ApplyEnv`, and
`Validate` provide explicit hooks. Full regeneration never overwrites this
file.

## Generated sections

The generated `Config` carries these sections; the keys are the YAML fields,
and final environment overrides follow the `APP_` prefix shown above:

| Section | Keys | Purpose |
| --- | --- | --- |
| `server` | `http_addr`, `grpc_addr`, `read_timeout`, `read_header_timeout`, `write_timeout`, `graceful_shutdown_timeout` | listeners and timeouts; `write_timeout` stays `0` for streaming |
| `logging` | `level`, `format` | slog level and format (`json` or `console`) |
| `database` | `driver`, `dsn`, `auto_migrate`, `max_open_conns`, `max_idle_conns`, `conn_max_lifetime` | connection and pool tuning |
| `middleware` | `timeout` | generated endpoint middleware |
| `debug` | `routes_enabled`, `print_routes` | route debugging switches |
| `remote` | `enabled`, `provider`, `endpoint`, `namespace`, `group`, `data_id`, `timeout`, `fallback_to_local` | remote configuration source |
| `custom` | application-defined | application-owned section |

For failure symptoms related to these settings, see
[troubleshooting](troubleshooting.md).

## Modes

| Mode | Behavior |
| --- | --- |
| `file` | local file plus environment; remote loading disabled |
| `hybrid` | remote loading enabled with local fallback |
| `remote` | remote loading required; startup fails on remote error |

## Secrets

Never commit credentials. Inject them through the deployment environment or an
application-owned provider; the generated config never logs a full
secret-bearing config, only a redacted summary.
