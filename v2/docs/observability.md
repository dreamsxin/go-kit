# Observability

English | [简体中文](observability_zh.md)

go-kit separates business outcome telemetry from protocol access telemetry.
Choose the signal you need, keep dimensions bounded, and never record request or
response payloads by default.

## Choose A Signal

| Need | Use | Sees |
| --- | --- | --- |
| business success, error, duration | `endpoint.Recorder` / `RecordingMiddleware` | decoded endpoint calls |
| local counters and average duration | `endpoint.Metrics` | in-process totals |
| structured business logs | `observability/slog` or `integrations/zap` | operation, outcome, IDs |
| traces and exported metrics | `observability/otel` | spans, counters, histogram |
| HTTP status, bytes, and path | `server.AccessLogMiddleware` | protocol facts |

## The Standard-Library Path

`slogadapter.NewTelemetry` assembles tracing, metrics, and logging with one
stable operation name:

```go
telemetry, err := slogadapter.NewTelemetry(slogadapter.TelemetryConfig{
    Operation: "users.create",
    Logger:    logger,
})
if err != nil {
    return err
}

ep := telemetry.Apply(endpoint.NewBuilder(createUser)).Build()
```

Keep `telemetry.Metrics` if the service exposes an internal snapshot or health
counter. Add `server.AccessLogMiddleware(logger)` with
`kit.WithHTTPMiddleware` when HTTP status and response bytes matter.

## OpenTelemetry

The optional `observability/otel` module uses the application's providers:

```go
metrics, err := oteladapter.NewMetrics(otel.Meter("users"))
if err != nil {
    return err
}

ep := endpoint.NewBuilder(createUser).
    Use(oteladapter.TracingMiddleware(otel.Tracer("users"), "users.create")).
    Use(endpoint.RecordingMiddleware("users.create", metrics)).
    Build()
```

`oteladapter.Metrics` implements `endpoint.Recorder`, so it can also be passed
to `kit.WithRecorder` for every JSON endpoint. Provider setup, exporters,
sampling, resource attributes, and shutdown remain application-owned.

## Correlation And Cardinality

- Enable `kit.WithRequestID()` for an `X-Request-ID` response header and the
  same ID in endpoint logs. `AccessLogMiddleware` reads it from the request
  context or response header.
- Propagate W3C `traceparent` at the transport boundary; endpoint middleware
  records the current trace ID.
- Use route patterns or fixed operation names as metric labels. Never use user
  IDs, arbitrary URLs, raw error text, or request payloads as dimensions.
- Install `ServerErrorHandler` when transport errors must be logged. The default
  handler is a no-op; error encoders only decide the wire response.

## Ownership And Shutdown

The application owns logger handlers, OpenTelemetry providers, exporters, and
their shutdown. The framework adapters own only the instruments or middleware
they create. Flush or shut down exporters after serving stops, and keep the
shutdown deadline longer than the exporter flush timeout.

For alert thresholds and deployment checks, see the [Production guide](../PRODUCTION.md).
For custom dimensions or a new backend, see [Customization](customization.md).
