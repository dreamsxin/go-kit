# Production Guidance
English | [简体中文](PRODUCTION_zh.md)

This guide covers the framework-level checks needed before deploying a service.
Application-specific authentication, authorization, data governance, and
operations remain the application's responsibility.

## Lifecycle

The process entry point owns signals and the root context:

```go
ctx, stop := signal.NotifyContext(
	context.Background(),
	os.Interrupt,
	syscall.SIGTERM,
)
defer stop()

if err := svc.Run(ctx); err != nil {
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
├── cmd/main.go        # wire-up: kit.New(..., kit.WithLifecycle(runner))
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
`kit.Lifecycle`:

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
svc, err := kit.New(":8080", kit.WithLifecycle(runner))
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

Streaming protocols require different timeout choices. MCP SSE responses are
long-lived, so the HTTP write timeout must be `0` or longer than the supported
session duration.

When generated contract support is enabled, `/openapi.json`, `/schema.json`, and
`/swagger/` expose the service contract. Keep them public only when that is an
intentional product decision; otherwise restrict or disable them at the
deployment boundary.

## HTTP Clients

Always set a client timeout or request deadline. JSON clients return
`HTTPStatusError` for non-2xx responses and bound the captured error body.

`NewJSONClientWithTimeout` adds a per-call context timeout. Use `sd/client.NewEndpoint`
and an explicit retry policy when retries are actually required.

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
runtime state. Close it before stopping the Instancer so subscriptions are
removed and factory-created client connections are released.

The built-in default retry classifier retries only explicit
`Retryable() == true` errors, no-endpoint discovery errors, and known transient
gRPC statuses. Unknown errors are permanent. Production callers should still
prefer `RetryWithRetryable` with a domain-specific classifier.

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

## Browser-Facing HTTP

Use the optional [`security/http`](security/http/README.md) package for CORS,
signed double-submit CSRF, security headers, trusted-proxy resolution, and
client-IP policy. Enable each middleware only with deployment-specific policy.

At minimum, review:

- allowed origins, methods, headers, and credentials;
- CSRF protection for cookie-authenticated state changes;
- forwarded-header trust boundaries;
- TLS termination and redirect behavior;
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
FROM golang:1.25 AS build
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
- the entry point already cancels `Service.Run` on `SIGTERM` (see
  Lifecycle); add a `preStop` sleep only when the service mesh or ingress
  keeps routing to the pod after the endpoint is deregistered;
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
