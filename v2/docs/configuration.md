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
| `server` | `http_addr`, `grpc_addr`, `read_timeout`, `read_header_timeout`, `write_timeout`, `graceful_shutdown_timeout`, `drain_delay`, `metrics_path` | listeners and timeouts; `write_timeout` stays `0` for streaming. `drain_delay` holds the process open after readiness starts failing, so set it above the interval at which your platform re-reads readiness. `metrics_path` serves the Prometheus exposition of per-route numbers and is empty (off) by default, because it publishes route names and traffic shape. `grpc_addr` is generated only for projects with gRPC |
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

## Serving TLS

Plaintext is the default. It assumes something in front of the process terminates
TLS — a sidecar, an ingress, a load balancer — which is a reasonable assumption in
most deployments and a bad one to leave unwritten.

To terminate in the process instead:

```go
component, err := kit.NewHTTP(":8443", kit.WithTLS(cfg.TLSCertFile, cfg.TLSKeyFile))
```

The pair is read by `NewHTTP`, not at the first handshake: a wrong path, an
unreadable file, or a key that does not match its certificate stops startup with the
path in the error, instead of becoming a client's TLS error to report.

`WithTLS` sets nothing else. Cipher suites, client certificates, SNI, and rotation
without a restart (a `GetCertificate` callback rather than a file read) are policy
with a compliance requirement behind them, so they belong to the deployment:

```go
component, err := kit.NewHTTP(":8443", kit.WithTLSConfig(&tls.Config{
	GetCertificate: reloader.GetCertificate,
	ClientAuth:     tls.RequireAndVerifyClientCert,
	ClientCAs:      pool,
}))
```

The config is used as given and cloned, with one exception: a zero `MinVersion`
becomes TLS 1.2, because Go's zero value there means TLS 1.0 for a server and no
deployment means to ask for that. `kit.DefaultTLSMinVersion` is that value; set
`MinVersion` explicitly to choose otherwise, including to accept older clients.

### Rotating the certificate

Files are read when you ask for them to be read, not on every handshake, and never
by a watcher this framework installed:

```go
certificates, err := kit.NewCertificateFiles(cfg.Server.TLSCertFile, cfg.Server.TLSKeyFile)
if err != nil {
    return err // a bad path still stops startup, with the path in the error
}
component, err := kit.NewHTTP(":8443", kit.WithTLSCertificateSource(certificates))

// Whatever your deployment renews on — a signal, a timer, an inotify library:
if err := certificates.Reload(); err != nil {
    logger.Error("certificate reload failed; still serving the previous one", "err", err)
}
```

`WithTLSCertificateSource` asks the source on every handshake, so the next client gets
whatever the last successful `Reload` loaded — no restart. A failed reload returns the
error and keeps serving the pair already loaded, because a half-written secret should
cost a log line rather than the listener.

A generated service does this for you: with `server.tls_cert_file` set, it serves the
certificate through the same per-handshake seam and re-reads both files on `SIGHUP`,
logging the paths on success and keeping the previous certificate on failure. Renewing
a certificate is `kill -HUP`, not a deploy.

When the source should be consulted is deliberately yours. A filesystem watch, a
poll interval, or a `SIGHUP` handler would each be a policy some deployment has to work
around, so the framework ships the seam and not the trigger. `kit.CertificateSource` is
that seam: implement it against a secret manager, an ACME client, or a per-name map for
SNI, and remember it runs on the handshake path, so cache rather than fetch.

### What is read again, and what is read once

Only the certificate is re-read. Everything else about the listener is fixed when it
starts, and knowing which is which is the difference between a renewal and a restart:

- read again, on the trigger you choose: the certificate and key, through
  `CertificateSource`. In a generated service, on `SIGHUP`.
- read once, at construction: the rest of the `tls.Config` — minimum version, cipher
  suites, `ClientAuth`, `ClientCAs`, `NextProtos`. `WithTLSConfig` clones what you pass,
  so mutating your copy afterwards changes nothing.
- read once, at `Start`: the listener address, the server timeouts
  (`read_timeout`, `read_header_timeout`, `write_timeout`, idle), `MaxHeaderBytes`, and
  every route and probe path that was registered.
- read once, at process start: the configuration file and environment. Nothing in this
  framework re-reads them, so a changed `server.metrics_path` or `server.drain_delay`
  takes effect on the next start.

Changing anything in the last three groups means a new listener, which means a restart —
and a rolling restart is what the drain sequence in
[Lifecycle](lifecycle.md) exists to make uneventful.

### What TLS also changes

Go offers HTTP/2 through ALPN as soon as the server has a TLS config, so a component
that gains a certificate gains h2 with it. Two consequences are worth knowing before
a client finds them:

- Streaming still works. An `http.Flusher` response — SSE, the streaming MCP
  transport — is framed by the h2 stream layer instead of chunked transfer encoding,
  and flushing still reaches the client.
- Upgrades do not. h2 has no `101 Switching Protocols`, so a handler that hijacks the
  connection only works on the plaintext listener. See
  [Upgraded connections are not drained](lifecycle.md#upgraded-connections-are-not-drained).

Cleartext HTTP/2 (h2c) is not offered. Negotiating it needs either a
prior-knowledge client or an upgrade exchange — both of which mean the caller already
knows what it is talking to — and where it is actually wanted, for a proxy speaking
h2 to the backend, the proxy's own configuration owns that decision. Serving it would
put a protocol nobody asked for on the port every plaintext client already uses.
