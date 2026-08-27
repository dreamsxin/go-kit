# Lifecycle

English | [简体中文](lifecycle_zh.md)

`kit.Host` orchestrates lifecycle components without owning any transport;
the `kit.HTTP` component owns the HTTP listener. After startup succeeds the
Host owns graceful shutdown. This page covers startup, shutdown, background
jobs, and optional servers.

## Startup and shutdown

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

svc, err := kit.NewHTTP(":8080")
if err != nil {
	return err
}
// register routes...

host, err := kit.NewHost(
	kit.WithLifecycle(svc),
	kit.WithShutdownTimeout(10*time.Second),
)
if err != nil {
	return err
}

if err := host.Run(ctx); err != nil {
	return err
}
```

- `kit.NewHTTP` and `kit.NewHost` validate configuration; startup failures
  are synchronous.
- `host.Run(ctx)` blocks until the context is cancelled or a component fails,
  then shuts down within the configured deadline.
- A host cannot be restarted after shutdown.

## Optional servers

A gRPC listener attaches through `kit.Lifecycle` and shares the same bounded
shutdown:

```go
grpcComponent, err := kitgrpc.New(":8081")
if err != nil {
	return err
}
pb.RegisterGreeterServer(grpcComponent.Server(), greeter)

host, err := kit.NewHost(kit.WithLifecycle(svc, grpcComponent))
```

Components start in order and shut down in reverse order.

## Background jobs

Periodic work lives in its own package beside the service layer and attaches
through the same lifecycle, so `SIGTERM` stops jobs with the process:

```go
type Job struct {
	Name     string
	Interval time.Duration
	Run      func(ctx context.Context) error
}

type Runner struct {
	Jobs []Job
	// ticker bookkeeping
}

func (r *Runner) Start() error                       { /* one goroutine per job */ }
func (r *Runner) Errors() <-chan error               { /* job failures */ }
func (r *Runner) Shutdown(ctx context.Context) error { /* stop tickers, wait in-flight */ }

runner := &jobs.Runner{Jobs: []jobs.Job{
	{Name: "cleanup-expired", Interval: time.Hour, Run: svc.CleanupExpired},
}}
host, err := kit.NewHost(kit.WithLifecycle(httpComponent, runner))
```

Rules that keep jobs safe next to serving traffic:

- each `Run` receives a context derived from shutdown, so cancellation reaches
  the database and HTTP clients through the service layer;
- a failing run is reported through `Errors()` and retried on the next tick;
  the runner never launches an overlapping run of the same job;
- intervals and job toggles belong in configuration, validated at startup.

A complete reference implementation is in
[PRODUCTION: Background Jobs](../PRODUCTION.md).
