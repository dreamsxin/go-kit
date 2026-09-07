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
APP_DRAIN_DELAY                  Server.DrainDelay
APP_METRICS_PATH                 Server.MetricsPath
APP_TLS_CERT_FILE                Server.TLSCertFile
APP_TLS_KEY_FILE                 Server.TLSKeyFile
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
| `server` | `http_addr`、`grpc_addr`、`read_timeout`、`read_header_timeout`、`write_timeout`、`graceful_shutdown_timeout`、`drain_delay`、`metrics_path` | 监听与超时；流式场景 `write_timeout` 保持 `0`。`drain_delay` 在 readiness 开始失败之后把进程多留一会儿，应设为大于平台重新读取 readiness 的间隔。`metrics_path` 提供逐路由数字的 Prometheus exposition，默认为空（关闭），因为它会公开路由名与流量形状。`grpc_addr` 仅在带 gRPC 的项目中生成 |
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

## 提供 TLS

默认是明文。它假定进程前面有别的东西在终止 TLS——sidecar、ingress、负载均衡——这在多数
部署里是个合理假设，但把它留着不写下来就不合理。

要改成在进程内终止：

```go
component, err := kit.NewHTTP(":8443", kit.WithTLS(cfg.TLSCertFile, cfg.TLSKeyFile))
```

证书对由 `NewHTTP` 读取，而不是等到第一次握手：路径写错、文件读不了、私钥和证书不匹配，
都会带着路径让启动失败，而不是变成某个客户端的 TLS 错误去汇报。

`WithTLS` 不设置别的任何东西。加密套件、客户端证书、SNI、以及不重启的轮换（那意味着
`GetCertificate` 回调而不是读文件），背后都有合规要求，属于部署方的决定：

```go
component, err := kit.NewHTTP(":8443", kit.WithTLSConfig(&tls.Config{
	GetCertificate: reloader.GetCertificate,
	ClientAuth:     tls.RequireAndVerifyClientCert,
	ClientCAs:      pool,
}))
```

传入的 config 会被克隆并原样使用，只有一个例外：`MinVersion` 为零时改为 TLS 1.2，因为
Go 在这里的零值对服务端意味着 TLS 1.0，没有哪个部署真想要它。这个值就是
`kit.DefaultTLSMinVersion`；想要别的（包括为了兼容老客户端）请显式设置 `MinVersion`。

### 轮换证书

文件在你要求它被读的时候才读，不是每次握手都读，也不会由这个框架装的监听器去读：

```go
certificates, err := kit.NewCertificateFiles(cfg.Server.TLSCertFile, cfg.Server.TLSKeyFile)
if err != nil {
    return err // 路径写错仍然让启动失败，并带上路径
}
component, err := kit.NewHTTP(":8443", kit.WithTLSCertificateSource(certificates))

// 你的部署靠什么触发续期，就用什么触发它——信号、定时器、inotify 库：
if err := certificates.Reload(); err != nil {
    logger.Error("certificate reload failed; still serving the previous one", "err", err)
}
```

`WithTLSCertificateSource` 每次握手都问 source，所以下一个客户端拿到的就是最后一次成功
`Reload` 加载的东西——不需要重启。失败的 reload 返回错误并继续服务已加载的那一对，因为写了
一半的 secret 应该只值一条日志，而不是整个监听器。

什么时候去问 source，刻意留给你。文件监听、轮询间隔、`SIGHUP` 处理器，每一个都是某些部署得
绕开的策略，所以框架只给接缝、不给触发器。`kit.CertificateSource` 就是那个接缝：你可以对着
密钥管理服务、ACME 客户端、或给 SNI 用的按名字映射去实现它——记住它运行在握手路径上，要缓存，
不要现取。

### 开了 TLS 还会变什么

只要服务端有 TLS config，Go 就会通过 ALPN 提供 HTTP/2——所以组件拿到证书的同时也拿到了
h2。有两个后果最好在客户端发现之前先知道：

- 流式仍然可用。带 `http.Flusher` 的响应——SSE、流式 MCP 传输——由 h2 的流层分帧，而不是
  chunked transfer encoding，flush 依然能到达客户端。
- 协议升级不可用。h2 没有 `101 Switching Protocols`，所以 hijack 连接的 handler 只在明文
  监听器上可用。见[被 hijack 的连接不会被 drain](lifecycle_zh.md#被-hijack-的连接不会被-drain)。

明文 HTTP/2（h2c）不提供。协商它要么需要客户端"预先知道"，要么需要一次升级交换——两者都
意味着调用方已经知道对面是什么；而真正想要它的场景（代理用 h2 连后端），这个决定属于代理
自己的配置。提供 h2c 等于在每个明文客户端都在用的端口上，放一个没人要求的协议。
