# sd — Service Discovery

The `sd` package wires together service discovery, load balancing, and retry
into a single callable `endpoint.Endpoint`.

## Quick start (no Consul needed)

```go
import (
    "github.com/dreamsxin/go-kit/sd"
    "github.com/dreamsxin/go-kit/sd/instance"
)

// In-memory instancer — perfect for tests and local dev
cache := instance.NewCache()
cache.Update(events.Event{Instances: []string{"host1:8080", "host2:8080"}})

ep := sd.NewEndpoint(cache, factory, logger,
    sd.WithMaxRetries(3),
    sd.WithTimeout(500*time.Millisecond),
)
resp, err := ep(ctx, request)
```

## With Consul

```go
import "github.com/dreamsxin/go-kit/sd/consul"

instancer := consul.NewInstancer(consulClient, logger, "my-service", true)
defer instancer.Stop()

ep := sd.NewEndpoint(instancer, factory, logger,
    sd.WithMaxRetries(3),
    sd.WithTimeout(500*time.Millisecond),
    sd.WithInvalidateOnError(5*time.Second),
)
```

## Options

| Option | Default | Description |
|--------|---------|-------------|
| `WithMaxRetries(n)` | 3 | Max retry attempts; 0 = retry until timeout |
| `WithTimeout(d)` | 500ms | Total budget including all retries |
| `WithInvalidateOnError(d)` | disabled | Clear cache after SD error grace period |

## Architecture

```
Instancer  →  Endpointer  →  RoundRobin  →  Retry  →  Endpoint
```

Each layer is independently usable:

```go
// Manual assembly (full control)
ep   := endpointer.NewEndpointer(instancer, factory, logger)
lb   := balancer.NewRoundRobin(ep)
call := executor.Retry(3, 500*time.Millisecond, lb)
```

## Retry strategies

```go
// Fixed max attempts
executor.Retry(3, time.Second, lb)

// Unlimited within timeout
executor.RetryAlways(2*time.Second, lb)

// Custom callback — stop on non-retryable errors
executor.RetryWithCallback(time.Second, lb,
    func(n int, err error) (keepTrying bool, replacement error) {
        if errors.Is(err, ErrInvalidArgument) {
            return false, err   // stop immediately
        }
        return n < 5, nil       // retry up to 5 times
    },
)
```

## Consul registration

```go
registrar := consul.NewRegistrar(client, logger, "my-service", "10.0.0.1", 8080,
    consul.IDRegistrarOptions("my-service-1"),
    consul.CheckRegistrarOptions(&stdconsul.AgentServiceCheck{
        HTTP:     "http://10.0.0.1:8080/health",
        Interval: "10s",
    }),
)
registrar.Register()
defer registrar.Deregister()
```

## See also

- `examples/sd/` — runnable demo of every sd component
- `examples/profilesvc/client/` — Consul-backed client example
