# 配置

[English](configuration.md) | 简体中文

只有用 `-config` 生成的项目才存在配置。本页覆盖优先级链与应用自有的自定义配置段。

## 优先级

```text
defaults
  -> local YAML
  -> environment bootstrap for remote connection settings
  -> optional remote config
  -> final environment overrides
  -> Config.Validate
```

校验在 logger、数据库、中间件与服务器创建之前运行，因此配置错误的部署会快速
失败。

环境变量使用 `APP_` 前缀：

```text
APP_HTTP_ADDR
APP_LOG_LEVEL
APP_LOG_FORMAT
APP_DB_DSN
APP_DB_AUTO_MIGRATE
APP_REMOTE_ENABLED
```

## 自定义配置段

应用特有的设置放在用户自有的 `config/custom.go` 中：

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

YAML 与远程配置会合并进 `custom`；`SetDefaults`、`ApplyEnv` 与 `Validate` 提供
显式钩子。全量重新生成绝不会覆盖此文件。

## 模式

| 模式 | 行为 |
| --- | --- |
| `file` | 本地文件加环境变量；禁用远程加载 |
| `hybrid` | 启用远程加载，本地作为降级兜底 |
| `remote` | 必须远程加载；远程出错则启动失败 |

## 机密

绝不要提交凭据。通过部署环境或应用自有的提供方注入；生成的配置绝不会记录完整
的携带机密的配置，只记录脱敏摘要。
