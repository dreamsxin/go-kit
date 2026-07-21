# Observability Guide

Purpose:
- Explain how to add tracing, metrics, logging, and request correlation to `go-kit` services without breaking the `service -> endpoint -> transport` layering.

Read this when:
- You are preparing a service for production operations.
- You are deciding where tracing, metrics, logging, or OpenTelemetry wiring should live.
- You are changing endpoint or transport hooks and need to preserve observability behavior.

See also:
- [endpoint/README.md](endpoint/README.md)
- [transport/README.md](transport/README.md)
- [v2/MAINTAINING.md](v2/MAINTAINING.md)

## Layering Rule

Observability should follow the framework architecture:

- `transport` extracts protocol metadata and records protocol-level facts.
- `endpoint` owns transport-neutral policy such as tracing, metrics, timeout, rate limiting, and error handling.
- `service` keeps business logic focused on domain behavior.

Avoid putting metrics counters, tracing spans, or audit side effects directly into business methods unless the signal is truly domain-specific.

## Request Correlation

Use request IDs and trace IDs as context values:

- `endpoint.TracingMiddleware()` generates trace and request IDs when missing.
- `endpoint.TraceIDFromContext(ctx)` reads the current trace ID.
- `endpoint.RequestIDFromContext(ctx)` reads the current request ID.
- `kit.WithRequestID()` propagates request ID behavior in the high-level service entry point.

Transport adapters should copy inbound correlation headers into context before endpoint execution. Endpoint middleware should then preserve those IDs through the rest of the call.

Recommended HTTP headers:

- `X-Request-ID`
- `X-Trace-ID`

## Metrics

Use `endpoint.MetricsMiddleware` for transport-neutral request accounting:

- total requests
- successes
- errors
- total duration
- last request time

Use `Metrics.Snapshot()` when exposing counters from handlers so readers do not race with live requests.

For production metrics backends:

- keep the endpoint middleware as the measurement boundary
- export snapshots to Prometheus, OpenTelemetry metrics, or another backend at the edge
- avoid coupling business services directly to one metrics provider

## Logging

Use logging middleware for endpoint-level operation logs and transport finalizers for protocol access logs.

Recommended fields:

- operation or route
- trace ID
- request ID
- duration
- status or error class
- remote address when available

If a logger is nil, framework entry points should degrade to a safe no-op logger where that behavior is documented.

## Transport Hooks

Use transport hooks for protocol metadata and final request accounting:

- `Before`: extract headers, metadata, auth context, trace context, and request IDs
- `After`: enrich response metadata or headers after successful endpoint work
- `Finalizer`: record protocol latency, access logs, metrics, and cleanup

Do not use transport hooks for business workflows. If behavior is protocol-independent, put it in endpoint middleware or interaction runtime hooks.

## OpenTelemetry Integration

`go-kit` does not require OpenTelemetry as a core dependency. Stable framework packages should remain useful without a global telemetry provider.

Recommended integration pattern:

1. Install OpenTelemetry SDK/exporters in the application or generated project.
2. Use transport `Before` hooks to extract trace context from HTTP headers or gRPC metadata.
3. Use endpoint middleware to start and end application spans around endpoint execution.
4. Use transport `Finalizer` hooks to record protocol-level attributes such as method, route, status, and latency.
5. Export metrics from endpoint counters or dedicated OTel instruments at the application boundary.

This keeps OpenTelemetry replaceable while preserving the framework's stable layering.

## AI Interaction

The `interaction` package provides a transport-neutral runtime for AI-facing tool loops. For observability:

- use `interaction.AuthorizationHook` for policy decisions
- use `interaction.AuditHook` for audit records
- store durable audit records in application-owned infrastructure
- keep interaction events separate from stable endpoint metrics

The `interaction/mcp` StreamableHandler supports server-initiated notifications (`notifications/message`) for logging, which can be used to relay server-side log events to MCP clients over SSE streams. The current log level is adjustable via `logging/setLevel`.

## Release Expectations

Before a release:

- run the release validation loop in [RELEASE.md](RELEASE.md)
- check that new transport hooks preserve `Before`, `After`, and `Finalizer` semantics
- check that new endpoint middleware composes through `endpoint.Chain` and `endpoint.Builder`
- update this guide when public observability behavior changes
