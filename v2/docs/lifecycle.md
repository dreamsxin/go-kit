# Lifecycle

English | [简体中文](lifecycle_zh.md)

`kit.Host` orchestrates lifecycle components without owning any transport;
the `kit.HTTP` component owns the HTTP listener. After startup succeeds the
Host owns graceful shutdown. This page covers startup, shutdown, background
jobs, and optional servers.

## Lifecycle At A Glance

```text
main creates signal context
  -> assemble components
  -> Host.Start
  -> serve and watch component errors
  -> Host.Drain: readiness fails, Draining components are told
  -> wait the drain delay
  -> bounded reverse-order shutdown
```

The process owns signals. A component owns the resources it creates. Generated
projects have an equivalent standalone loop; they do not use `kit.Host`.

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
	kit.WithDrainDelay(5*time.Second),
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
  then stops in three steps: announce, wait, tear down.
- A host cannot be restarted after shutdown.

## Draining

Stopping starts with an announcement. `Host.Drain` -- which `Run` calls for you,
and `Shutdown` calls if you did not -- fails readiness with `kit.ErrDraining` and
tells every attached `kit.Draining` component, in reverse attachment order,
*before* anything is torn down. Liveness keeps passing: a process finishing
in-flight work should not be restarted.

`kit.WithDrainDelay` is the wait between the announcement and the teardown. It
defaults to zero, which stops immediately. Set it to a little more than the
interval at which whatever routes traffic here re-reads readiness or discovery --
otherwise the announcement has not been heard by the time the listener closes, and
requests already in flight toward this instance fail.

Implement `Draining` on a component that accepts work of its own:

```go
func (c *Consumer) Drain(ctx context.Context) error {
	c.stopPulling() // announce; do not wait for in-flight work here
	return nil
}
```

`Drain` announces, `Shutdown` finishes: the grace period belongs to `Shutdown`,
and a `Drain` that blocks spends the budget of every component behind it. A
`Drain` error is reported and the sequence continues -- a component that cannot
stop accepting work still has to be shut down.

## Health probes

`kit.NewHTTP` registers three routes unconditionally:

- `/livez` -- liveness. The process is up. A failure means restart the container.
- `/readyz` -- readiness. The service can take traffic. A failure means remove
  the instance from the load balancer, but leave it running.
- `/health` -- both scopes, for tooling that only knows one URL.

Each returns 503 when any check in its scope fails, with a 2s default timeout per
check. Point Kubernetes at `/livez` and `/readyz` rather than `/health`, so a
failing dependency drains the pod instead of restarting it.

Generated projects (`microgen`) expose the same three routes but run their own
`main` loop -- they do not use `kit.Host`. Their readiness flips on once the
listeners are serving and off again at the first shutdown signal.

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
through the same lifecycle, so `SIGTERM` stops jobs with the process. The
`Runner` below is a template for your own `jobs` package — the framework supplies
`kit.Lifecycle` and the host, not the runner:

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

- `kit.Lifecycle.Start()` takes no context, so the runner creates its own —
  `context.WithCancel(context.Background())` in `Start`, cancelled in
  `Shutdown` — and passes it to every `Run`, so cancellation reaches the
  database and HTTP clients through the service layer;
- a failing run is reported through `Errors()` and retried on the next tick;
  the runner never launches an overlapping run of the same job;
- intervals and job toggles belong in configuration, validated at startup.

The same sketch, with the surrounding package layout, is in
[PRODUCTION: Background Jobs](../PRODUCTION.md). Neither is a package this
framework ships.
