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

`Signals` selects the dimensions one by one — `SignalTracing | SignalLogging`
for a service whose metrics come from an OpenTelemetry meter, so no call is
recorded twice. Each assembled middleware carries its own name, so
`Builder.Describe` stays correct when a dimension is missing or an adapter of
your own is appended to `telemetry.Middlewares`.

Keep `telemetry.Metrics` if the service exposes an internal snapshot or health
counter. Add `server.AccessLogMiddleware(logger)` with
`kit.WithHTTPMiddleware` when HTTP status and response bytes matter.

## Being Scraped

Pushing is not the only model. `observability/metrics` renders the numbers
`endpoint.Metrics` already holds as Prometheus text exposition, with no metrics
client anywhere in the module:

```go
collector := &endpoint.Metrics{}

var routes httpserver.RouteRegistrar = mux
routes = httpserver.DecorateRoutes(mux,
    httpserver.RecordingMiddleware(metrics.HTTPRecorder(collector)))
routes.Handle("GET /users/{id}", usersHandler)

mux.Handle("GET /metrics", metrics.Handler(collector))
```

Two details carry the design. Recording is installed *at registration*, through
`httpserver.RouteRegistrar` — `http.Request.Pattern` is only set on the request the
mux dispatched, so middleware wrapped around the mux would report every request
under an empty route. And the exposition is mounted on the mux itself, outside the
decorator, so a scrape reports on the service instead of on itself.

For gRPC, the recorder is an interceptor and the bridge lives in its own package so
that an HTTP-only service is not made to depend on the gRPC libraries just because it
wanted a scrape endpoint:

```go
component := kitgrpc.MustNew(":8081",
    grpc.ChainUnaryInterceptor(grpcserver.RecordingUnaryInterceptor(metricsgrpc.Recorder(collector))),
    grpc.ChainStreamInterceptor(grpcserver.RecordingStreamInterceptor(metricsgrpc.Recorder(collector))),
)
```

The operation label is the full method name, `/users.Users/Get`. It needs no
defending the way an HTTP route does: the set of methods is fixed by the service
definition, and a call to a method that does not exist is refused before an
interceptor runs. A stream is recorded when it ends, and carries `Stream: true` —
its duration is a lifetime, not a latency, so do not average the two together.

What the exposition does and does not give you: counts, error counts, and a duration
sum per operation — a mean, not quantiles. There are no buckets because the collector
holds a total; a p99 comes from the OpenTelemetry histogram below. Counters start at
zero on process start, which a scraper detects as a reset.

Generated services take `server.metrics_path` (`APP_METRICS_PATH`), empty and off by
default: the endpoint publishes your route names and traffic shape, so it belongs on
an admin listener or behind a network policy.

## OpenTelemetry

`oteladapter.Setup` assembles providers, OTLP exporters, the resource, the
global W3C propagator, and one `Shutdown`:

```go
providers, err := oteladapter.Setup(ctx, oteladapter.Config{
    ServiceName: "users",
    Endpoint:    "collector:4317",
    Insecure:    true,
})
if err != nil {
    return err
}
defer providers.Shutdown(context.Background())

metrics, err := oteladapter.NewMetrics(providers.Meter())
if err != nil {
    return err
}

ep := endpoint.NewBuilder(createUser).
    Use(oteladapter.TracingMiddleware(providers.Tracer(), "users.create")).
    Use(endpoint.RecordingMiddleware("users.create", metrics)).
    Build()
```

`oteladapter.Metrics` implements `endpoint.Recorder`, so it can also be passed
to `kit.WithRecorder` for every JSON endpoint. Instruments follow the semantic
conventions: duration in seconds, and a failed call carries `error.type` rather
than feeding a separate error counter.

For response status, `oteladapter.NewHTTPMetrics` records
`http.server.request.duration` with `http.route` and `http.response.status_code`.
Install it with `kit.WithHTTPRecorder(httpMetrics)`, which wires the recorder per
route — the only place the matched pattern exists.

Skip `Setup` when the application owns its provider configuration; pass any
`trace.Tracer` and `metric.Meter` instead, and keep shutdown yours.

## Correlation And Cardinality

- Enable `kit.WithRequestID()` for an `X-Request-ID` response header and the
  same ID in endpoint logs. `AccessLogMiddleware` reads it from the request
  context or response header.
- W3C `traceparent` is extracted at the transport boundary without wiring:
  `kit.NewHTTP` and `kit/grpc.New` both do it, and `integrations/grpc/client`
  injects it on the way out. Endpoint middleware then records the current trace
  ID. `transporthttp.ExtractTraceparent` / `InjectTraceparent` and
  `transportgrpc.ExtractTraceparent` / `InjectTraceparent` cover hand-assembled
  servers and clients.
- `interaction.Runtime.WithLogger(logger)` reports tool calls through the same
  logger and the same `trace_id` / `request_id`, so an MCP tool call joins the
  request that carried it.
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
