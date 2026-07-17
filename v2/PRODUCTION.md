# Production Guidance

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

## HTTP Clients

Always set a client timeout or request deadline. JSON clients return
`HTTPStatusError` for non-2xx responses and bound the captured error body.

`NewJSONClientWithTimeout` adds a per-call context timeout. Use `sd.NewEndpoint`
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

The built-in default retry classifier retries only explicit
`Retryable() == true` errors, no-endpoint discovery errors, and known transient
gRPC statuses. Unknown errors are permanent. Production callers should still
prefer `RetryWithRetryable` with a domain-specific classifier.

Backoff and calls honor context cancellation. The total timeout must cover all
attempts and waits, not each attempt independently.

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

CORS, CSRF, security headers, and trusted-proxy/IP handling are optional HTTP
integration packages. Enable them only with deployment-specific policies.

At minimum, review:

- allowed origins, methods, headers, and credentials;
- CSRF protection for cookie-authenticated state changes;
- forwarded-header trust boundaries;
- TLS termination and redirect behavior;
- cache and content-type headers.

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

OpenTelemetry support belongs in optional adapters. Propagate context through
service, endpoint, transport, discovery, and interaction calls. Create spans at
meaningful boundaries without creating a span for every small helper.

## Health

- Liveness answers whether the process can continue running.
- Readiness answers whether it should receive traffic.
- Dependency checks need short, independent timeouts.
- Do not expose secrets, stack traces, or full dependency errors in public
  health responses.

`kit` exposes `/health`, `/livez`, and `/readyz`. Generated projects expose
`/health`; add deployment-specific readiness behavior as needed.

## Pre-Deployment Checklist

- Configuration validates in the deployment environment.
- HTTP/gRPC limits and timeouts match the workload.
- MCP write timeout supports long-lived responses when enabled.
- Shutdown is exercised with `SIGTERM`.
- Retry is limited to classified, safe operations.
- Database migration behavior is explicit.
- Authentication and authorization are tested at protocol and business layers.
- Logs, metrics, and traces avoid secrets and unbounded dimensions.
- `go test ./...` and targeted race tests pass.
