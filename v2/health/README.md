# health

Process health checks: a registry, two scopes, and the probe routes.

English | [简体中文](README_zh.md)

This package is **this process's** health registry: named liveness and
readiness checks, evaluated per request, with a JSON report shape that probe
tooling can depend on. It is what `kit.NewHTTP` uses to mount `/health`,
`/livez`, and `/readyz`, and it can be mounted on any mux of your own — a
gRPC-only binary, a worker, or a second admin port.

Not to be confused with:

- kit's probe options (`WithLivenessCheck`, `WithReadinessCheck`) — they
  populate this registry on the HTTP component;
- [`sd/health`](../sd/README.md) — whether **remote instances** pass active
  probes. Neither answers the other's question.

## The registry

```go
registry := health.NewRegistry(
	health.WithTimeout(2*time.Second),                     // per-check budget
	health.WithRequestCount(func() int64 { return served.Load() }), // optional
)

registry.AddLiveness("process", health.Healthy)            // always passes
registry.AddReadiness("database", pingDatabase)            // runtime dependency
```

- `Check` is `func(context.Context) error`; implementations must stop
  promptly when the context is canceled. A check that fails, times out,
  panics, or is already running marks its scope `unavailable`.
- Checks can be added after the registry is mounted; the next probe sees
  them.
- `WithRequestCount` adds the served-request count to the report, giving an
  operator one number that says the process is doing work rather than merely
  answering.

## Mounting

`Mount(mux, paths)` registers the routes:

```go
paths := health.DefaultPaths() // /health, /livez, /readyz
registry.Mount(mux, paths)
```

Routes are registered without a method prefix, so they answer every method.
An empty path in `paths` omits that route — expose readiness alone, or keep
probes off the traffic port entirely by mounting the registry on a second
listener of your own:

```go
admin := http.NewServeMux()
registry.Mount(admin, health.Paths{
	All:       "",
	Liveness:  "/healthz/live",
	Readiness: "/healthz/ready",
})
go http.ListenAndServe(":8081", admin)
```

The report body is `{"status","requests"?,"checks":[...]}` — its field names
are a stable contract (`health.report-shape`), as is the empty-path rule
(`health.mount-skips-empty-path`). `Handler(scope)` hands out the underlying
handler for a `ScopeAll` / `ScopeLiveness` / `ScopeReadiness` scope if you
need it outside `Mount`.

## Where to go next

- [lifecycle](../docs/lifecycle.md): how kit mounts the probes and what
  readiness failing means during shutdown
- [production guide](../PRODUCTION.md): probe configuration for deployments
