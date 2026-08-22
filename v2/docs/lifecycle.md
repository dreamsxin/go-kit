# Lifecycle

English | [简体中文](lifecycle_zh.md)

`kit.Service` owns the HTTP listener and graceful shutdown after startup
succeeds. This page covers startup, shutdown, background jobs, and optional
servers.

## Startup and shutdown

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

svc, err := kit.New(":8080", kit.WithShutdownTimeout(10*time.Second))
if err != nil {
	return err
}
// register routes...

if err := svc.Run(ctx); err != nil {
	return err
}
```

- `kit.New` validates configuration; startup failures are synchronous.
- `svc.Run(ctx)` blocks until the context is cancelled or a server fails, then
  shuts down within the configured deadline.
- A service cannot be restarted after shutdown.

## Optional servers

A gRPC listener attaches through `kit.Lifecycle` and shares the same bounded
shutdown:

```go
grpcComponent, err := kitgrpc.New(":8081")
if err != nil {
	return err
}
pb.RegisterGreeterServer(grpcComponent.Server(), greeter)

svc, err := kit.New(":8080", kit.WithLifecycle(grpcComponent))
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
svc, err := kit.New(":8080", kit.WithLifecycle(runner))
```

Rules that keep jobs safe next to serving traffic:

- each `Run` receives a context derived from shutdown, so cancellation reaches
  the database and HTTP clients through the service layer;
- a failing run is reported through `Errors()` and retried on the next tick;
  the runner never launches an overlapping run of the same job;
- intervals and job toggles belong in configuration, validated at startup.

A complete reference implementation is in
[PRODUCTION: Background Jobs](../PRODUCTION.md).
