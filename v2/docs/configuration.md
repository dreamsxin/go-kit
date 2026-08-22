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
