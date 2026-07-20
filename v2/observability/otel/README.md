# OpenTelemetry adapter

`observability/otel` is an optional module for endpoint tracing and metrics.
It is intentionally outside the main `github.com/dreamsxin/go-kit/v2` module,
so services that do not use OpenTelemetry do not acquire its dependencies.

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
