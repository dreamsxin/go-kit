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

A registration attached with `kit.WithRegistrar` deregisters as part of the
announcement, not as part of the teardown: the instance leaves discovery, then the
drain delay gives the registry and every peer that cached its answer time to
notice, and only then does the listener close. What no server can promise is the
entry a peer has already read -- which is the reason the sequence waits instead of
assuming.

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

## Long-lived responses

A stream cannot be drained by asking politely: `http.Server.Shutdown` waits for
handlers to return, and a handler writing an event every second never does. So an
HTTP component tells its handlers instead:

```go
component.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
	for {
		select {
		case <-kit.Stopping(r.Context()):
			return // the process is going away; end the stream
		case <-r.Context().Done():
			return // this client went away
		case event := <-events:
			// write the event
		}
	}
})
```

`kit.Stopping(ctx)` closes when draining begins -- before anything is closed, so
the response can end itself and the client sees the end of a stream rather than a
broken connection. Outside a kit HTTP component it returns nil, and receiving from
a nil channel blocks forever, so the select above is correct either way.

For a handler that watches neither signal, the grace period still ends.
`Shutdown` tries `http.Server.Shutdown` first; when that runs out of budget it
cancels the request contexts, gives handlers a moment to unwind, closes what is
left, and returns an error wrapping `kit.ErrShutdownIncomplete` that says how many
requests were interrupted. It does not return while a connection it owns is still
open.

## Upgraded connections are not drained

One connection is outside all of it. Once a handler calls `Hijack` -- a WebSocket, or
any other upgrade -- the server stops tracking that connection: the graceful wait does
not include it, closing the listener does not close it, and the hard close cannot
reach it. `Shutdown` returns `nil` promptly and the upgraded connection keeps
carrying bytes.

That is not a gap to be fixed by the framework; it is what hijacking means. The seam
is the handler that did it, and the signal is the same one a stream uses:

```go
component.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	go readFrames(conn)
	<-kit.Stopping(r.Context()) // the process is going away; close the socket
})
```

Note the ordering this relies on: `Stopping` closes when draining begins, which is
before `Shutdown` runs, so an upgraded handler that watches it ends inside the grace
period rather than after it. Set `kit.WithDrainDelay` long enough for that to happen.

Over HTTP/2 there is no upgrade to hijack -- h2 has no `101 Switching Protocols`. A
hijack-based protocol therefore only works on the plaintext listener; see
[Serving TLS](configuration.md#serving-tls) for what else changes when a certificate
is configured.

## gRPC drains the same way

`kit/grpc.Component` is drained by the same sequence, and a gRPC handler reads the
same signal:

```go
func (s *server) Watch(req *pb.WatchRequest, stream pb.Watcher_WatchServer) error {
	for {
		select {
		case <-kit.Stopping(stream.Context()):
			return nil // the process is going away; end the stream
		case <-stream.Context().Done():
			return stream.Context().Err()
		case event := <-events:
			// stream.Send(event)
		}
	}
}
```

Draining a gRPC component announces; it does not start refusing calls. A client that
got `UNAVAILABLE` during the drain delay would retry, possibly at this same instance,
because the routing layer has not caught up yet — failing readiness is the signal
that actually moves traffic, and the gRPC health service reports it.

The limit is the mirror image of the HTTP one, and it is bounded the same way. The
component counts the calls in flight itself, closes the listener so no new connections
arrive, waits for those calls until the budget expires, and then closes the transports
— reporting `kit.ErrShutdownIncomplete` with the number it interrupted. It does not
use grpc's `GracefulStop`: that call holds the server's own mutex while it waits for
handlers, and `Stop` needs the same mutex, so a handler that never returns makes the
pair deadlock rather than time out. Unlike a hijacked HTTP connection, nothing here
escapes the shutdown; what survives is a handler goroutine that watched neither its
context nor the stopping signal, and it ends when the process does.

If you write your own transport, `kit.WithStopping(ctx, ch)` is the seam: carry it
into your handler contexts and `kit.Stopping` works there too.

## What each transport answers

| Question | HTTP (`kit.HTTP`) | gRPC (`kit/grpc.Component`) |
| --- | --- | --- |
| readiness | `/readyz`, `/livez`, `/health` | `grpc.health.v1` `Check` per call, and `Watch` streaming the current status then one message per change |
| drain announcement | yes, `kit.Draining` | yes, `kit.Draining` |
| stopping signal | `kit.Stopping(r.Context())` | `kit.Stopping(stream.Context())` |
| shutdown budget | its own share; cancels in-flight requests and closes the rest, reporting how many | its own share; waits for the calls in flight, then closes the transports, reporting how many |
| what escapes shutdown | a hijacked connection — not waited for, not closed | nothing; a handler goroutine that watched neither signal outlives its connection until the process exits |
| TLS | `kit.WithTLS` / `WithTLSConfig`, ALPN gives HTTP/2 | `grpc.Creds` from a `tls.Config` you build, as a `ServerOption` |
| metrics | `httpserver.Recorder` per route, labelled with the matched pattern | `grpcserver.Recorder` per call, labelled with the full method |
| trace context | extracted at the boundary, no wiring | extracted at the boundary, no wiring |

A test fails if `kit` grows a lifecycle interface that has not been classified for
both, so the next asymmetry is a build failure rather than a discovery.

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
