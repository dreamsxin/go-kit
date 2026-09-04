# OpenTelemetry adapter
English | [简体中文](README_zh.md)

`observability/otel` is an optional package in the
`github.com/dreamsxin/go-kit/v2` module. Services that do not import it do not
resolve its OpenTelemetry dependencies in their package dependency closure.

The application owns tracer/meter provider setup, exporters, sampling, and
shutdown. The adapter only creates spans and instruments from the providers it
is given:

```go
tracer := otel.Tracer("my-service")
endpointFn = oteladapter.TracingMiddleware(tracer, "GetUser")(endpointFn)

metrics, err := oteladapter.NewMetrics(otel.Meter("my-service"),
    oteladapter.WithMetricAttributes(attribute.String("component", "users")),
)
endpointFn = metrics.Middleware("GetUser")(endpointFn)
```

Only bounded application, operation, and outcome attributes are added by the
adapter. Resource attributes such as `service.name` belong in provider setup.
Request and response payloads are never recorded.
