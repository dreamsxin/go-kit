# OpenTelemetry adapter
English | [简体中文](README_zh.md)

`observability/otel` is an optional package in the
`github.com/dreamsxin/go-kit/v2` module. Services that do not import it do not
resolve its OpenTelemetry dependencies in their package dependency closure.

## One Call To Assemble The Pipeline

`Setup` builds the providers, the OTLP exporters, and the resource, installs
them globally together with the W3C trace context and baggage propagator, and
hands back the one `Shutdown` that flushes and stops all of it:

```go
providers, err := oteladapter.Setup(ctx, oteladapter.Config{
    ServiceName:    "checkout",
    ServiceVersion: "1.4.0",
    Environment:    "production",
    Endpoint:       "collector:4317",
    Insecure:       true,       // a collector on localhost or inside the pod
    SampleRatio:    0.05,       // root spans only; a sampled parent keeps its children
})
if err != nil {
    return err
}
defer func() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    _ = providers.Shutdown(ctx)
}()
```

- `Protocol` selects `ProtocolGRPC` (the default, port 4317) or `ProtocolHTTP`
  (port 4318). An empty `Endpoint` defers to `OTEL_EXPORTER_OTLP_ENDPOINT`.
- `Signals` selects `SignalTraces`, `SignalMetrics`, or both. The zero value
  assembles both.
- `SpanExporter` and `MetricReader` replace the OTLP pipeline — a Prometheus
  reader, or a recording exporter in a test.
- Give `Shutdown` a deadline. It blocks while the last batch is exported, and a
  collector that stopped answering would otherwise hold process exit.

`Setup` installs the global propagator because that is what makes a trace
continue across a hop: the transports in this framework read and write the
`traceparent` header, and any library using the OpenTelemetry API interoperates
with them through that propagator.

## Endpoint Telemetry

Middleware and instruments come from the providers, so an application that owns
its own provider setup can skip `Setup` entirely:

```go
endpointFn = oteladapter.TracingMiddleware(providers.Tracer(), "GetUser")(endpointFn)

metrics, err := oteladapter.NewMetrics(providers.Meter(),
    oteladapter.WithMetricAttributes(attribute.String("component", "users")),
)
endpointFn = metrics.Middleware("GetUser")(endpointFn)
```

`Metrics` implements `endpoint.Recorder`, so `kit.WithRecorder(metrics)` covers
every route with its pattern as the operation. It records:

- `go_kit.endpoint.requests` — counter, `{request}`
- `go_kit.endpoint.duration` — histogram, seconds

Both carry `operation`, plus `error.type` when the call failed. The error kind
comes from the error itself through `interface{ ErrorKindName() string }` —
every `apperror` value names its kind — so error rate is a ratio over one series
instead of a second counter.

## HTTP Server Telemetry

`HTTPMetrics` records `http.server.request.duration` under the HTTP semantic
conventions, with `http.request.method`, `url.scheme`, `http.route`, and
`http.response.status_code`. Response status is therefore alertable from
metrics, with no log pipeline in between:

```go
httpMetrics, err := oteladapter.NewHTTPMetrics(providers.Meter())
if err != nil {
    return err
}
component, err := kit.NewHTTP(":8080", kit.WithHTTPRecorder(httpMetrics))
```

`kit.WithHTTPRecorder` installs the recorder per route, which is where the
matched pattern exists — middleware wrapped around the mux would record every
request with an empty route. A request that matched no route carries no
`http.route` rather than the raw URL path, which is what keeps the dimension
bounded.

Only bounded application, operation, and protocol attributes are added. Request
and response payloads are never recorded.
