# metrics exposition
English | [简体中文](README_zh.md)

`observability/metrics` renders what `endpoint.Metrics` already collected as
Prometheus text exposition. It uses the standard library only — a dependency gate
keeps any metrics client out of it, so mounting a scrape endpoint adds nothing to the
build.

```go
collector := &endpoint.Metrics{}

var routes httpserver.RouteRegistrar = mux
routes = httpserver.DecorateRoutes(mux,
    httpserver.RecordingMiddleware(metrics.HTTPRecorder(collector)))
routes.Handle("GET /users/{id}", usersHandler)

mux.Handle("GET /metrics", metrics.Handler(collector))
```

Recording is installed at registration, not around the mux: `http.Request.Pattern` is
only set on the request a `ServeMux` dispatched, so middleware wrapped around the mux
would file every request under an empty route. The exposition is mounted on the mux
itself, outside the decorator, so a scrape reports on the service rather than on
itself.

What a scrape returns: a request count, an error count, a duration sum and a last-seen
timestamp per operation, where the operation is the matched route pattern. That is a
mean, not quantiles — the collector holds totals, and there are no buckets to derive a
p99 from. Use the OpenTelemetry histogram in `observability/otel` when you need one.
Counters start at zero when the process starts, which a scraper reads as a reset.

For gRPC, the bridge is `observability/metrics/grpc`, a package of its own so that this
one stays reachable with the standard library alone. It labels each series with the full
method name, which the service definition bounds.

Mounting is the application's decision: `kit` does not import this package, and the
dependency gate keeps it that way, so the route, the listener it lives on, and whether
it exists at all stay with the deployment. Generated services expose
`server.metrics_path` (`APP_METRICS_PATH`), empty and off by default — the endpoint
publishes route names and traffic shape, so put it behind an admin listener or a network
policy rather than on the port that serves users.
