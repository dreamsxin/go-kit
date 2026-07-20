# sd — Service Discovery

The `sd` package wires together service discovery, load balancing, and retry
into a single callable `endpoint.Endpoint`.

## Quick start (no Consul needed)

```go
import (
    "github.com/dreamsxin/go-kit/v2/sd"
    "github.com/dreamsxin/go-kit/v2/sd/instance"
)

// In-memory instancer — perfect for tests and local dev
cache := instance.NewCache()
cache.Update(events.Event{Instances: []string{"host1:8080", "host2:8080"}})

ep, closer, err := sd.NewEndpoint(cache, factory, logger,
    sd.WithMaxAttempts(3),
    sd.WithTimeout(500*time.Millisecond),
)
if err != nil {
    return err
}
defer closer.Close()
resp, err := ep(ctx, request)
```

## With Consul

```go
import "github.com/dreamsxin/go-kit/v2/sd/consul"

instancer := consul.NewInstancer(consulClient, logger, "my-service", true)

ep, closer, err := sd.NewEndpoint(instancer, factory, logger,
    sd.WithMaxAttempts(3),
    sd.WithTimeout(500*time.Millisecond),
    sd.WithInvalidateOnError(5*time.Second),
)
if err != nil {
    instancer.Stop()
    return err
}
defer instancer.Stop()
defer closer.Close() // runs first: deregister and close endpoint connections
```

## Options

| Option | Default | Description |
|--------|---------|-------------|
| `WithMaxAttempts(n)` | 1 | Total attempts; must be at least 1 |
| `WithTimeout(d)` | 500ms | Positive total budget including all retries |
| `WithInvalidateOnError(d)` | disabled | Clear cache after SD error grace period |

Invalid options and nil required dependencies return an error before any
background goroutine starts.

## Architecture

```
Instancer  →  Endpointer  →  RoundRobin  →  Retry  →  Endpoint
```

Each layer is independently usable:

```go
// Manual assembly (full control)
ep   := endpointer.NewEndpointer(instancer, factory, logger)
defer ep.Close()
lb   := balancer.NewRoundRobin(ep)
call := executor.Retry(3, 500*time.Millisecond, lb)
```

## Retry strategies

```go
// Fixed max attempts
executor.Retry(3, time.Second, lb)

// Production calls should provide an explicit retry classifier.
executor.RetryWithRetryable(time.Second, lb,
    func(n int, err error) (keepTrying bool, replacement error) {
        return n < 5, nil
    },
	func(err error) bool {
		var retryable interface{ Retryable() bool }
		return errors.As(err, &retryable) && retryable.Retryable()
	},
)
```

The default classifier retries explicit `Retryable() == true` errors,
no-endpoint discovery errors, and known transient gRPC statuses. Unknown errors
are permanent. Use a domain-specific classifier when write safety or business
error semantics matter.

`Endpointer.Close` waits for its update loop and closes all resources returned
by the endpoint factory. Treat the closer as part of the constructor contract,
not as an optional cleanup hook.

## Consul registration

```go
registrar := consul.NewRegistrar(client, logger, "my-service", "10.0.0.1", 8080,
    consul.IDRegistrarOptions("my-service-1"),
    consul.CheckRegistrarOptions(&stdconsul.AgentServiceCheck{
        HTTP:     "http://10.0.0.1:8080/health",
        Interval: "10s",
    }),
)
if err := registrar.Register(); err != nil {
    return err
}
defer func() { _ = registrar.Deregister() }()
```

`Instancer.Stop` cancels and joins the active Consul blocking query, so call it
after endpoint-owned resources have been closed.

## See also

- `examples/sd/` — runnable demo of every sd component
- `examples/profilesvc/client/` — Consul-backed client example
