# Troubleshooting

English | [简体中文](troubleshooting_zh.md)

A symptom-based guide: what a failure means in this framework, and where to
look. Every section assumes the correlation setup from the first section is
in place.

## First step: correlate one request end to end

Every tool below works per request. Wire correlation once:

```go
svc, err := kit.NewHTTP(":8080",
    kit.WithRequestID(),                       // X-Request-ID in + out
    kit.WithEndpointMiddleware(
        endpoint.TracingMiddleware(),          // W3C trace context
        slogadapter.LoggingMiddleware(logger, "myop"), // business log line
    ),
    kit.WithJSONServerOptions(
        server.ServerErrorEncoder(server.JSONErrorEncoder),
        server.ServerErrorHandler(slogadapter.NewErrorHandler(logger)),
    ),
)
host, _ := kit.NewHost(kit.WithLifecycle(svc))
// transport-layer access lines:
// host wiring with kit.WithHTTPMiddleware(server.AccessLogMiddleware(logger))
```

Then a failing request is traceable by the `X-Request-ID` response header and
the `request_id` / `trace_id` fields in the business and access log lines.
Note: the default `ServerErrorHandler` is a no-op — transport and endpoint
errors are only *mapped*, not *logged*, until a handler is installed.

## The service does not start

Startup failures are synchronous and name their cause:

| Symptom | Meaning | Action |
| --- | --- | --- |
| `http listen: ...` | port already bound | check the port; another instance still running |
| config validation error | generated `Config.Validate` failed before anything started | the message names the offending key; fix config/env |
| `connect database failed` (generated services) | DSN unreachable at startup | verify `database.dsn` / `APP_DB_DSN`, driver, network |
| `start lifecycle component <name>` | a `kit.Lifecycle` component failed its `Start` | the component name is included when it implements `kit.NamedLifecycle` |

Configuration validation runs before the logger, database, middleware, and
servers are created, so config problems never hide behind runtime logs.

## Health checks fail

- `/readyz` unavailable: the response body names the failing check
  (`{"checks":[{"name":"db","status":"error"}]}`). Causes: a registered
  readiness check fails (dependency down), or a lifecycle component
  implementing `kit.ReadinessProvider` has not finished warming up (check name
  is `lifecycle:<name>`), or a check exceeded `WithHealthCheckTimeout`
  (`"check timed out"`).
- `/livez` failing while `/readyz` also fails: liveness checks are a separate
  set; a failing liveness check means the process itself should restart.
- `/health` shows both sets plus the request count when `kit.WithMetrics` is
  set — a stuck request count indicates traffic stopped reaching the chain.

## Requests fail by status code

| Status | Typical cause | Where to look |
| --- | --- | --- |
| 400 `bad_request.validation` | `Validatable` request rejected | the error body lists the invalid fields |
| 400 (decode) | strict JSON decode rejected the body | unknown fields, duplicate keys, or trailing data — strict by design |
| 401 | authentication rejected (`apperror.KindUnauthenticated`) | credential extraction at the protocol boundary |
| 403 | authorization rejected (`KindPermissionDenied`) | `security.RequireRole` / application policy |
| 404 | route or method mismatch | Go mux patterns are method-aware (`"POST /x"` ≠ `"GET /x"`); check `/debug/routes` |
| 409 / 412 | `KindConflict` / `KindFailedPrecondition` | business state conflict |
| 429 | rate limiting rejected the caller | check the limiter budget; `Retry-After` reports the wait when the limiter knows it |
| 499 | the client disconnected or cancelled | `KindCanceled` or an unclassified `context.Canceled` — not a server fault |
| 500 | unclassified error | internals are redacted from the response — inspect logs via the error handler |
| 501 | `KindUnimplemented` | the route exists but the operation is not built |
| 503 | `KindUnavailable`, or one of three load-shedding protections | see the next section |
| 504 | `KindDeadlineExceeded` or `TimeoutMiddleware` | downstream dependency too slow |

## Which protection rejected the request?

Rate limiting answers 429 (the caller exceeded its quota). The other three
protections answer 503, because the service is shedding load. Distinguish them
by what is configured and what the metrics show:

| Source | Middleware | Status | Signal |
| --- | --- | --- | --- |
| Rate limiting | `RateLimitMiddleware` | 429 | steady over-limit traffic; check the limiter budget |
| Circuit breaker | `CircuitBreaker` | 503 | opens after consecutive failures — the real problem is the failing dependency; breaker `State()` shows open/half-open, and `Retry-After` reports the open window |
| Bulkhead | `BulkheadMiddleware` | 503 | one key saturates its concurrency slot |
| Backpressure | `BackpressureMiddleware` | 503 | global in-flight cap; the whole service is saturated |

`endpoint.Metrics.SnapshotFor(pattern)` counts errors per route,
`Snapshot()` the total, and `endpoint.Builder.Describe()` prints the chain order
at startup so you know which protections are armed for a route.

## Database problems (generated services)

- Connection settings: `database.driver`, `database.dsn`
  (`APP_DB_DSN` overrides), `database.auto_migrate`
  (`APP_DB_AUTO_MIGRATE`).
- Pool tuning: `database.max_open_conns`, `database.max_idle_conns`,
  `database.conn_max_lifetime`. Exhausted pools make requests block and then
  fail with deadline errors — raise `max_open_conns` or fix slow queries
  before raising timeouts.
- Migrations: `AutoMigrate` is off by default; it runs at startup only when
  explicitly enabled. Production schema changes should use a dedicated
  migration process.
- Surface database health through a readiness check so `/readyz` drains
  traffic before failures become user-visible.

## Logging and observability

- Generated services build the logger from `logging.level` and
  `logging.format` (`json` or `console`); `APP_LOG_LEVEL` / `APP_LOG_FORMAT`
  override at deploy time. Levels are standard `log/slog` levels.
- Log output goes to **stdout by design** — there is no log-path setting.
  For file storage build your own `slog.Handler` (see
  [Production: log destinations](../PRODUCTION.md#log-destinations-and-file-storage));
  rotation is application owned.
- Business lines come from `slogadapter.LoggingMiddleware` (or
  `integrations/zap`); protocol lines come from
  `server.AccessLogMiddleware`. Both carry `request_id` / `trace_id` when the
  corresponding middleware runs.
- `endpoint.Metrics.Snapshot()` exposes request/success/error counts and
  average duration without any exporter; `/health` echoes the request count.
- For production metric export use `oteladapter.NewMetrics`; spans use
  `oteladapter.TracingMiddleware`.

## Debug switches

| Switch | Effect |
| --- | --- |
| `debug.routes_enabled: true` | serves `GET /debug/routes` listing registered routes |
| `debug.print_routes: true` | prints all routes at startup |
| `endpoint.Builder.Describe()` | prints the middleware chain of one endpoint |
| `microgen extend -check` | validates manifest drift in a generated project |
