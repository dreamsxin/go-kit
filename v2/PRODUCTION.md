# Production Guidance
English | [简体中文](PRODUCTION_zh.md)

This guide covers the framework-level checks needed before deploying a service.
Application-specific authentication, authorization, data governance, and
operations remain the application's responsibility.

## Deployment Gate

Before shipping, verify these six areas:

1. [Lifecycle](#lifecycle): bounded startup and reverse-order shutdown.
2. [HTTP Server](#http-server) and [HTTP Clients](#http-clients): limits,
   deadlines, and streaming timeouts.
3. [Service Discovery And Retry](#service-discovery-and-retry): explicit,
   idempotency-aware retry policy and resource closure.
4. [Authentication And Authorization](#authentication-and-authorization):
   protocol authentication plus business authorization.
5. [Logging](#logging), [Metrics](#metrics), and [Tracing](#tracing): bounded,
   non-sensitive telemetry with request correlation.
6. [Pre-Deployment Checklist](#pre-deployment-checklist): run it in CI and again
   in the deployment environment.

## Lifecycle

The process entry point owns signals and the root context:

```go
ctx, stop := signal.NotifyContext(
	context.Background(),
	os.Interrupt,
	syscall.SIGTERM,
)
defer stop()

if err := host.Run(ctx); err != nil {
	return err
}
```

Use bounded graceful shutdown. Treat listener bind errors and asynchronous
server errors as startup/runtime failures instead of logging them and continuing.

## Background Jobs

Periodic work (cleanup, reconciliation, cache warmup) belongs in its own
package beside the service layer, wired into the same lifecycle as the HTTP
server so `SIGTERM` stops jobs with the process:

```text
service/
├── cmd/main.go        # wire-up: kit.NewHost(kit.WithLifecycle(http, runner))
├── service/           # business logic; jobs call these methods
├── repository/        # storage
├── transport/         # HTTP handlers
└── jobs/              # one file per job plus the lifecycle runner
    ├── cleanup.go
    └── runner.go
```

Jobs are callers of the service layer, not endpoints: they must not import
transport packages, and their business logic stays in `service/` so HTTP
handlers and jobs share one implementation. The runner implements
`kit.Lifecycle`.

`jobs.Runner` below is a sketch to write in your own service, not a package this
framework ships — the framework contributes `kit.Lifecycle` and the host that
drives it:

```go
type Job struct {
    Name     string
    Interval time.Duration
    Run      func(ctx context.Context) error
}

type Runner struct {
    Jobs []Job
    // ... ticker bookkeeping
}

func (r *Runner) Start() error                         { /* start one goroutine per job */ }
func (r *Runner) Errors() <-chan error                 { /* job panics and hard failures */ }
func (r *Runner) Shutdown(ctx context.Context) error   { /* stop tickers, wait in-flight */ }
```

Attach it in `main`:

```go
runner := &jobs.Runner{Jobs: []jobs.Job{
    {Name: "cleanup-expired", Interval: time.Hour, Run: svc.CleanupExpired},
}}
host, err := kit.NewHost(kit.WithLifecycle(httpComponent, runner))
```

Rules that keep jobs safe next to serving traffic:

- each `Run` receives a context derived from shutdown, so cancellation
  reaches the database and HTTP clients through the service layer;
- a failing run is reported through `Errors()` and retried on the next tick;
  the runner never launches an overlapping run of the same job;
- intervals and job toggles belong in configuration, validated at startup,
  so a deployment can disable a misbehaving job without a code change;
- in generated projects, put the package under the same topology (for
  example `jobs/` beside `service/`) and wire it in the user-owned
  `cmd/main.go`; regeneration preserves user-owned files.

## HTTP Server

Configure all of the following explicitly for the deployment:

- read-header timeout;
- read timeout;
- write timeout;
- idle timeout;
- maximum header bytes;
- maximum JSON request body bytes;
- graceful shutdown timeout.

Strict JSON endpoints reject unknown fields and trailing JSON values. Keep body
limits enabled unless a specific route has a documented reason to accept larger
payloads.

Put `endpoint.RecoveryMiddleware` outermost in every endpoint chain. It turns
business or middleware panics into a classified, redacted 500; the supplied
`PanicHandler` should report the recovered value to logs or an error service,
never to the response body.

Streaming protocols require different timeout choices, and which ones apply
depends on the MCP revision a client selects. On `2025-06-18` responses are
long-lived, so the HTTP write timeout must be `0` or longer than the supported
session duration. Set MCP `SessionTTL`, `MaxSessions`, `MaxPostBodyBytes`, and
`AllowedOrigins`, start cleanup, and call `StreamableHandler.Shutdown(ctx)` (or
`mcp.Serve`) during termination so SSE writers and interaction sessions are
released.

On `2026-07-28` there is no session and no stream: every request is one bounded
POST, so ordinary write timeouts apply and `SessionTTL`/`MaxSessions` bound only
the clients still on the older revision. `MaxPostBodyBytes` and `AllowedOrigins`
apply to both. Deploy the stateless revision behind an ordinary round-robin
balancer — no sticky routing, no shared session store — and set `ListCacheTTL`
and `ListCacheScope` to what the catalog can honestly promise, since clients
cache on them.

Set `RequestStateKey` when tools ask their callers questions, and give every
instance the same key: without one, the `requestState` a tool receives is
whatever the caller sent back, and with one an edited or expired state is refused
before the tool runs. `RequestStateTTL` bounds replay; keep it short enough that a
stale confirmation cannot be resubmitted later. Treat the key as a secret injected
by the deployment, like any other signing key.

When generated contract support is enabled, `/openapi.json`, `/schema.json`, and
`/swagger/` expose the service contract. Keep them public only when that is an
intentional product decision; otherwise restrict or disable them at the
deployment boundary.

## TLS Termination

Decide where TLS ends, and write the decision down. Both places are supported and
the default is a proxy.

Terminate in front of the process — sidecar, ingress, load balancer — when the
platform already rotates certificates, when several services share a hostname, or
when mTLS and cipher policy are the mesh's job. The process then serves plaintext on
a network only the proxy can reach, and that reachability is the assumption you are
accepting; state it in the deployment, not in a reader's head.

Terminate in the process when the hop to it is not trusted, when a compliance
requirement names the service rather than the edge, or when there is no proxy —
a single binary on a VM, or an admin port that must not be plaintext:

```
APP_TLS_CERT_FILE=/etc/tls/tls.crt
APP_TLS_KEY_FILE=/etc/tls/tls.key
```

Both keys or neither: setting one fails validation instead of quietly serving
plaintext on the port a client is about to speak TLS to. The pair is loaded before
the listener serves, so a wrong path or a mismatched key stops startup with the path
in the message. The minimum version is TLS 1.2. Cipher suites, client certificates,
SNI, and rotation without a restart are policy — `kit.WithTLSConfig` takes a
`tls.Config` you built, and in a generated service the same config is yours to set in
`cmd/main.go`.

What in-process termination changes operationally:

- HTTP/2 arrives with it, through ALPN. Streaming still works; hijack-based upgrades
  do not, because h2 has no `101 Switching Protocols`. A WebSocket endpoint therefore
  needs the plaintext listener or a proxy that terminates TLS and speaks HTTP/1.1
  onwards.
- Readiness and drain are unchanged, with one addition: a certificate mounted from a
  secret is rotated by replacing files. `kit.WithTLS` reads them once, so that
  arrangement needs a restart; `kit.WithTLSCertificateSource` with
  `kit.NewCertificateFiles` asks on every handshake and re-reads on
  `Reload`, which is what lets a renewal be picked up without a deploy. A generated
  service wires that for you and re-reads on `SIGHUP` — `kill -HUP` the process after
  the renewal, or have the renewing job do it. A failed reload keeps the previous
  certificate serving, so alert on the reload failing rather than on the expiry.
- Health probes must speak the same scheme as the listener. A probe still configured
  for `http` against a TLS port reads as an unhealthy instance, and the kubelet will
  restart a process that is working.
- An upgraded connection is not drained in either arrangement — see the drain notes
  under Lifecycle. Terminating in-process does not change that; it only removes the
  option to have one.

Cleartext HTTP/2 (h2c) is not served. If a proxy in front wants to speak h2 to the
backend, configure it to speak HTTP/1.1, or give the backend a certificate and let
ALPN do it.

## HTTP Clients

A client built by `transport/http/client` no longer uses `http.DefaultClient`. Two
things about that default were wrong for a service and neither was about policy: it is
a package-level variable any library in the process can reconfigure, and its transport
allows two idle connections per host — right for a tool that calls many hosts once,
wrong for a service that calls the same upstream on every request, where it shows up as
latency the application code cannot explain.

The client this package uses instead is its own, with `MaxIdleConnsPerHost` 100, a
60-second idle timeout, and dial and handshake timeouts. `client.DefaultClient()`
returns it, so you can reuse the same pool for calls made outside this package, and
`client.NewTransport()` returns a fresh transport to start from when you need a proxy
or a `tls.Config` but want the pool sizing.

It sets no `Timeout`, deliberately. A timeout there would cap every call in the process
at a number this framework invented, invisibly from the call site. Set the deadline
where the call is: `NewJSONClientWithTimeout`, `endpoint.Builder.WithTimeout`, or a
context you derive. Always set one — a request with no deadline is the one that is still
running when the incident starts.

JSON clients return `HTTPStatusError` for non-2xx responses and bound the captured
error body. Use `sd/client.NewEndpoint` and an explicit retry policy when retries are
actually required.

Retry only operations whose idempotency and error classification are known.
Unknown business errors should not be assumed transient.

## gRPC

- Register services before starting listeners.
- Use a new response value per client request.
- Preserve context deadlines and cancellation.
- Configure message limits and transport credentials at application assembly.
- Validate streaming behavior separately from unary RPC behavior.

## Service Discovery And Retry

Discovery subscribers receive immutable snapshots. Consumers should use buffered
update channels and must deregister or close their endpointer during shutdown.
`sd/client.NewEndpoint` returns `(endpoint, closer, error)`; treat the closer as owned
runtime state. Close it before calling `Instancer.Close` so subscriptions are
removed and factory-created client connections are released. Every
`Balancer.Pick` must be paired with `Picked.Done` after the endpoint returns.

The built-in default retry classifier (`sd/retry.DefaultClassifier`) retries only
errors that classify themselves through `interface{ Retryable() bool }` and
`sd.ErrNoEndpoints`; context cancellation and deadlines are never retried, and
anything unclassified is treated as permanent. It knows nothing about gRPC or
HTTP status codes on its own: a gRPC client contributes that knowledge with
`sdclient.WithRetryable(grpc.Retryable)`, and any other protocol plugs in the
same way — `client.WithRetryable(...)` on `sd/client`, or
`retry.WithClassifier(...)` in a hand-assembled path.

Backoff and calls honor context cancellation. The total timeout must cover all
attempts and waits, not each attempt independently.

Invalid attempt counts, non-positive timeouts, negative invalidation durations,
and nil required dependencies fail synchronously before an Endpointer starts.

## Configuration And Secrets

Generated config resolves local YAML, optional remote config, final environment
overrides, then validates the complete result.

- Do not commit credentials or production DSNs.
- Use environment/deployment injection for secrets.
- Fail startup on malformed duration, address, required database, logging,
  middleware, or remote-provider settings.
- Keep database migration disabled unless startup mutation is intentional.
- Log a redacted configuration summary, never a full secret-bearing config.

## Authentication And Authorization

Authentication and authorization are integration concerns, not framework core
features. Add them at the application boundary:

- authenticate protocol credentials in HTTP/gRPC middleware;
- place the verified principal in context;
- enforce business authorization in service or endpoint policy;
- return protocol-safe errors without leaking internal details.

Do not treat trusted proxy headers as identity unless the deployment has an
explicit trusted-proxy policy.

An MCP endpoint needs the same treatment, and has its own seam for it:
`mcp.StreamableHandler.Authorizer` is asked about every request — method, target
name, HTTP header, and the context the request is served under — before any
provider or tool runs. It is nil by default, and a nil authorizer serves every
implemented method, so an endpoint exposed beyond a trusted network needs one
written for the deployment. A refused request is `-32001`, or 403 when it carried
no id to answer. Two limits are worth knowing: the policy reads the principal
from the context, so something must authenticate the caller first, and on
`2025-06-18` the SSE stream (GET) and session teardown (DELETE) are not
authorized per request — the session those act on was authorized when
`initialize` created it, and its `Mcp-Session-Id` is a bearer credential from
then on.

## Browser-Facing HTTP

Use the optional [`security/http`](security/http/README.md) package for CORS,
signed double-submit CSRF, security headers, trusted-proxy resolution, and
client-IP policy. Enable each middleware only with deployment-specific policy.

At minimum, review:

- allowed origins, methods, headers, and credentials;
- CSRF protection for cookie-authenticated state changes, including the
  `SessionID` accessor that binds a token to one session and its `TokenTTL`;
- forwarded-header trust boundaries;
- TLS termination and redirect behavior, and `AssumeHTTPS` where TLS terminates
  upstream;
- cache and content-type headers.

Install trusted-proxy resolution before IP policy and HTTPS-dependent headers.
Only configured direct peers may influence forwarded client IP or scheme. Keep
CORS outside CSRF so browser preflight remains token-free. Scope CSRF to
cookie-authenticated browser routes; do not place it over Bearer-only APIs or
MCP POST routes unless those routes intentionally use browser cookies and can
echo the token header. These middleware do not wrap `http.ResponseWriter`, so
SSE flushing and other streaming interfaces remain available.

`kit` applications can install the compiled policies once with
`kit.WithHTTPMiddleware`; lower-level applications can use
`httpsecurity.Chain` around their root handler.

## Logging

Use structured logs with stable fields:

- service and version;
- request/trace ID;
- route or RPC method;
- duration;
- final status/error class;
- selected backend for discovered calls when useful.

Libraries return errors and do not call `Fatal`. Only `main` decides whether an
error terminates the process.

For standard-library logging, use the optional
[`observability/slog`](observability/slog/README.md) adapter. It records
endpoint outcome, duration, and correlation IDs without recording payloads;
the application still selects the `slog.Handler` and level.

### Log destinations and file storage

Generated services write structured logs to stdout by design (12-factor):
the container or node collector owns what happens to them. There is no
log-path setting in the framework on purpose — file paths, rotation, and
retention are application or deployment decisions.

For file storage, build your own `slog.Handler` and pass the logger into the
adapters — any `slog.Handler` implementation works:

```go
file, err := os.OpenFile("service.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
if err != nil {
	return err
}
logger := slog.New(slog.NewJSONHandler(file, &slog.HandlerOptions{Level: slog.LevelInfo}))
// plug into the adapters:
//   slogadapter.LoggingMiddleware(logger, op)
//   slogadapter.NewErrorHandler(logger)
```

In generated projects, `cmd/main.go` is user-owned: change the logger
construction there (or build the logger in a `config/custom.go` hook) instead
of editing generated files. Rotation and shipping stay application owned.

## Metrics

Measure at the endpoint boundary for business calls and at the transport boundary
for protocol details. Recommended signals include:

- request count and duration by operation/status class;
- in-flight requests;
- decode and encode failures;
- rate-limit and circuit-breaker rejection;
- retry attempts and exhausted retries;
- discovery instance count and update errors;
- MCP session and stream counts.

Avoid unbounded labels such as raw URL, user ID, request ID, or error text.

To be scraped rather than to push, set `APP_METRICS_PATH` (`server.metrics_path`)
on a generated service, or mount `metrics.Handler(collector)` yourself in an
assembly built on `kit`. Both are off by default: the endpoint publishes your route
names and traffic shape, so put it on an admin listener or behind network policy
rather than on the port that serves users.

What a scrape returns, and what it does not:

- `endpoint_requests_total{operation,outcome}` — one series per route pattern and
  outcome. `operation` is the matched pattern, never the request path, and a request
  that matched no route is not recorded at all;
- `endpoint_request_duration_seconds_sum` and `_count` by operation — a mean, not
  quantiles. There are no buckets because the collector measures a total; a p99 comes
  from the OpenTelemetry histogram, not from this endpoint;
- `endpoint_last_request_timestamp_seconds` by operation — for spotting a route that
  has gone quiet;
- counters start at zero when the process starts, because the collector is in
  memory. A scraper detects the reset; a query that subtracts raw values across a
  restart will not.

A scrape interval of 15–30s is enough for these series: they are counters and a
sum, so the resolution you lose between scrapes is resolution the collector never
had.

## Tracing

OpenTelemetry support belongs in the optional
[`observability/otel`](observability/otel/README.md) module. The application
owns provider setup, resources, exporters, sampling, and shutdown. Propagate
context through service, endpoint, transport, discovery, and interaction calls;
create spans at meaningful boundaries without creating a span for every small
helper.

For request correlation without a tracing backend, the core packages propagate
the W3C `traceparent` header end to end:

- Servers extract the incoming header with
  `transport/http.ExtractTraceparent` as a `ServerBefore` hook;
- `endpoint.TracingMiddleware` joins the caller's trace or mints a
  W3C-conformant one;
- Clients forward the active trace with `transport/http.InjectTraceparent`
  as a client `Before` hook.

`observability/otel` remains the right choice for full span trees; the core
helpers keep trace IDs connected across service boundaries with no
dependencies.

## Health

- Liveness answers whether the process can continue running.
- Readiness answers whether it should receive traffic.
- Dependency checks need short, independent timeouts.
- Health checks must return promptly when their context is canceled.
- Do not expose secrets, stack traces, or full dependency errors in public
  health responses.

`kit` exposes `/health`, `/livez`, and `/readyz`. Generated projects expose
`/health`; add deployment-specific readiness behavior as needed. Registered
`kit` checks run concurrently under one request budget. A named check never
overlaps with its previous invocation, which bounds damage from a dependency
probe that fails to honor cancellation.

When `kit.WithRequestID` is enabled, caller-supplied IDs are validated before
they are copied into context, responses, and logs. The default accepts common
ASCII token characters up to 128 bytes. Use `WithRequestIDValidator` only when
the deployment has a different trusted ID format.

## Deployment

The runtime is a static binary. Framework packages and the pure-Go SQLite
driver need no CGO, so a two-stage container build ends in a minimal base:

```dockerfile
FROM golang:1.26 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /out/service ./cmd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/service /service
USER nonroot
ENTRYPOINT ["/service"]
```

On Kubernetes, wire the health endpoints to probes and align the termination
budget with the framework's shutdown timeout:

- readinessProbe -> `/readyz`, livenessProbe -> `/livez`, both with short
  periods and timeouts; readiness gates traffic during rollout;
- `terminationGracePeriodSeconds` must exceed the kit shutdown timeout
  (`kit.WithShutdownTimeout`, default 10s) plus the platform's load-balancer
  deregistration delay, or in-flight requests are cut mid-shutdown;
- the entry point already cancels `Host.Run` on `SIGTERM` (see
  Lifecycle); prefer `kit.WithDrainDelay` over a `preStop` sleep — the Host fails
  readiness first and then waits, so the pod stops claiming to be ready while it is
  still answering, which a `preStop` sleep alone cannot express. A generated
  service does the same through `APP_DRAIN_DELAY` (`server.drain_delay`), which
  defaults to `0s`: set it above the interval at which your platform re-reads
  readiness, or the listener closes before the announcement has been read;
- prefer rolling updates with `maxUnavailable: 0` so readiness, not pod
  deletion, controls traffic shifts.

Inject configuration through the generated precedence chain (defaults ->
local YAML -> optional remote config -> environment overrides ->
validation). Environment variables are the container-native layer: every
generated setting has an `APP_`-prefixed variable. `Config.Validate` runs
before the listener binds, so a misconfigured deployment fails fast instead
of serving degraded traffic.

## Alerting

Turn the Metrics signals above into alerts before scaling incidents, not
after. A starter set:

- **Error rate**: ratio of 5xx responses (or classified `internal` errors) to
  total requests over 5 minutes; page when it exceeds 1% for two consecutive
  windows. Alert on the ratio, not the count, so scaling does not create
  false pages.
- **Latency**: p99 endpoint duration above the product's stated budget for
  10 minutes; warn at p95 to catch drift before paging.
- **Retry exhaustion**: exhausted retries for a dependency indicate the
  dependency, not the caller, is failing; page the owning service.
- **Circuit breaker open**: a breaker held open for more than one minute
  means the fallback path is carrying production traffic.
- **Discovery churn**: instance-count drops or repeated discovery update
  errors point at registrar, network, or health-check misconfiguration.
- **Health flapping**: readiness failing intermittently while liveness stays
  green isolates dependency trouble from process trouble.

Keep alerts on ratios and durations, keep labels bounded (see Metrics), and
route dependency-owned signals (retry exhaustion, breaker open) to the
dependency's on-call, not the caller's.

## Pre-Deployment Checklist

- Configuration validates in the deployment environment.
- HTTP/gRPC limits and timeouts match the workload.
- MCP write timeout supports long-lived responses when enabled.
- Shutdown is exercised with `SIGTERM`.
- Termination grace period exceeds the kit shutdown timeout plus
  deregistration delay.
- Readiness and liveness probes point at `/readyz` and `/livez`.
- Starter alerts (error rate, latency, retry exhaustion, breaker state) are
  defined and routed.
- Retry is limited to classified, safe operations.
- Database migration behavior is explicit.
- Authentication and authorization are tested at protocol and business layers.
- Logs, metrics, and traces avoid secrets and unbounded dimensions.
- `go test ./...` and targeted race tests pass.
