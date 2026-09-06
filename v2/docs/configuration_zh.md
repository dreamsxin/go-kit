# 配置

[English](configuration.md) | 简体中文

只有用 `-config` 生成的项目才存在配置。本页说明优先级、校验和自定义配置段。

## 快速结论

- 用 YAML 提供部署默认值，用 `APP_*` 环境变量提供部署时覆盖。
- `file`、`hybrid`、`remote` 是生成期模式，不是运行时字段。
- `Config.Validate` 在命令行覆盖之后、运行时组装前执行；参数和配置文件走同一套校验。
- `config/custom.go` 归应用所有，必须在 `*CustomConfig` 上保留
  `SetDefaults`、`ApplyEnv() error` 和 `Validate() error`。

## 优先级

```text
默认值               (Default)
  -> 本地 YAML       (LoadLocal；文件缺失不致命)
  -> 环境变量        (ApplyEnv)
  -> 可选远程配置    (LoadRemote)
  -> 再次环境变量    (ApplyEnv)
  -> 命令行 flag
  -> Config.Validate
```

两次环境变量应用是同一个完整的 `ApplyEnv`。第一次在远程配置之前运行，好让
`APP_REMOTE_*` 指向远程源；第二次在其之后运行，因此环境变量始终优先于远程值。

校验在命令行覆盖之后、logger、数据库、中间件与服务器创建之前运行，因此配置错误的部署会快速失败。

命令行 flag 使用已加载配置作为默认值（`-http.addr` 默认取 `cfg.Server.HTTPAddr`），
应用到配置后再走同一套最终校验。flag 适合本地临时覆盖，部署请用 YAML 或环境变量。

生成的 `main` 支持下面这些 flag，其中哪些存在取决于项目是怎么生成的——和环境变量键一样。
第二列是每个 flag 覆盖的目标。

```text
-config                          要加载的配置文件
-http.addr                       Server.HTTPAddr
-grpc.addr                       Server.GRPCAddr
-db.dsn                          Database.DSN
-auto-migrate                    Database.AutoMigrate
```

环境变量使用 `APP_` 前缀。`ApplyEnv` 会读取下面每一个键。其中哪些存在取决于项目是怎么
生成的：`APP_DB_*` 需要数据库，`APP_GRPC_ADDR` 需要 gRPC，`APP_REMOTE_*` 需要远端配置模式。

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

这份清单会与生成的加载器互相核对，因此某个键被改名或删掉会让测试失败，而不是悄无声息。

## 自定义配置段

应用特有的设置放在用户自有的 `config/custom.go` 中。三个钩子是 `*CustomConfig`
的方法，不是 `*Config` 的方法，生成的加载器按这些确切签名调用它们：

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

三个方法都要保留，签名也要保留：`SetDefaults()` 无返回值，`ApplyEnv() error` 与
`Validate() error` 返回 error。生成的 `config/config.go` 与 `config/env.go` 会调用
它们，因此改接收者或改签名要么编译不过，要么被静默地永不调用。

YAML 与远程配置会合并进 `custom`。全量重新生成绝不会覆盖此文件。

## 生成配置段

生成的 `Config` 包含以下配置段；键为 YAML 字段，最终环境变量覆盖遵循上文的 `APP_` 前缀：

| 配置段 | 键 | 用途 |
| --- | --- | --- |
| `server` | `http_addr`、`grpc_addr`、`read_timeout`、`read_header_timeout`、`write_timeout`、`graceful_shutdown_timeout` | 监听与超时；流式场景 `write_timeout` 保持 `0`。`grpc_addr` 仅在带 gRPC 的项目中生成 |
| `logging` | `level`、`format` | slog 级别与格式（`json` 或 `console`） |
| `database` | `driver`、`dsn`、`auto_migrate`、`max_open_conns`、`max_idle_conns`、`conn_max_lifetime` | 连接与连接池调优；仅在 `-db` 时生成 |
| `middleware` | `timeout` | 生成的端点中间件 |
| `debug` | `routes_enabled`、`print_routes` | 路由调试开关 |
| `remote` | `enabled`、`provider`、`endpoint`、`namespace`、`group`、`data_id`、`timeout`、`fallback_to_local` | 远程配置源 |
| `custom` | 应用自定义 | 应用自有配置段 |

配置段是生成期的，不是运行期的：项目若不是用 `-db` 且带 gRPC 传输生成的，
`database` 与 `grpc_addr` 字段根本不存在于结构体中。在没有它们的项目里加上对应
YAML 键不会有任何效果。

这些设置相关的故障症状见[排障指南](troubleshooting_zh.md)。

## 模式

模式是生成期选项——`microgen -config-mode=<mode>`——它决定生成哪份加载器代码，
不是运行期设置：

| 模式 | 行为 |
| --- | --- |
| `file` | 本地文件加环境变量；不生成远程加载器 |
| `hybrid` | 启用远程加载，本地作为降级兜底 |
| `remote` | 必须远程加载；远程出错则启动失败 |

在 `hybrid` 模式下，只要开启本地回退，远程 endpoint 或 data ID 为空就按仅本地启动处理。
这样生成的默认配置可以直接本地运行，部署时再注入远程坐标；`remote` 模式仍然严格要求完整配置。

要切换模式，请用不同的 `-config-mode` 重新生成。

## 机密

绝不要提交凭据。通过部署环境或应用自有的提供方注入。

生成的 `main` 从不记录配置本身——它只记录 `config loaded path=<path>`。在 `-db`
项目中，DSN 会经 `redactDSN` 脱敏后记录一次。除此之外你自己记录的配置内容，需要
你自己负责脱敏。
