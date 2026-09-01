# sd - Service Discovery
English | [简体中文](README_zh.md)

The root `sd` package owns the protocol-neutral `Event`, `Instancer`,
`Registrar`, `Balancer`, and `ErrNoEndpoints` contracts. Concrete components
live in focused subpackages and can be used independently.

## Quick start (no Consul needed)

```go
import (
    "github.com/dreamsxin/go-kit/v2/sd"
	"github.com/dreamsxin/go-kit/v2/sd/client"
	"github.com/dreamsxin/go-kit/v2/sd/endpointer"
    "github.com/dreamsxin/go-kit/v2/sd/instance"
)

factory := endpointer.Factory(func(instance string) (endpoint.Endpoint, io.Closer, error) {
	return makeClientEndpoint(instance), nil, nil
})

// In-memory instancer — perfect for tests and local dev
cache := instance.NewCache()
cache.Update(sd.Event{Instances: []string{"host1:8080", "host2:8080"}})

ep, closer, err := client.NewEndpoint(cache, factory, logger,
    client.WithMaxAttempts(3),
    client.WithTimeout(500*time.Millisecond),
)
if err != nil {
    return err
}
defer closer.Close()
resp, err := ep(ctx, request)
```

## With Consul

```go
import (
	"github.com/dreamsxin/go-kit/v2/sd/client"
	"github.com/dreamsxin/go-kit/v2/integrations/consul"
)

instancer := consul.NewInstancer(consulClient, logger, "my-service", true)

ep, closer, err := client.NewEndpoint(instancer, factory, logger,
    client.WithMaxAttempts(3),
    client.WithTimeout(500*time.Millisecond),
    client.WithInvalidateOnError(5*time.Second),
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
| `WithBalancer(f)` | round robin | Replace the selection strategy |

Invalid options and nil required dependencies return an error before any
background goroutine starts.

## Selection strategies

`sd.Balancer` is the extension point; `sd/balancer` ships four strategies.

| Strategy | Constructor | Picks by | Use it when |
|----------|-------------|----------|-------------|
| Round robin | `balancer.NewRoundRobin` | atomic counter | Default; uniform instances, one client |
| Random | `balancer.NewRandom` | uniform draw | Many clients share one instance set and lockstep counters would synchronise |
| Weighted random | `balancer.NewWeightedRandom` | caller-supplied weight per instance | Heterogeneous capacity, canary shares, draining an instance |
| Consistent hash | `balancer.NewConsistentHash` | hash of a request key | Cache affinity or per-entity ordering |

```go
// Uniform random
client.WithBalancer(func(set endpointer.InstanceEndpointer) sd.Balancer {
	return balancer.NewRandom(set)
})

// Weighted: capacity from instance metadata, zero drains an instance without
// waiting for service discovery to withdraw it.
client.WithBalancer(func(set endpointer.InstanceEndpointer) sd.Balancer {
	return balancer.NewWeightedRandom(set, func(instance string) int {
		return weights[instance]
	})
})

// Consistent hash: every request for one tenant lands on the same instance
// for as long as that instance stays in the set.
client.WithBalancer(func(set endpointer.InstanceEndpointer) sd.Balancer {
	return balancer.NewConsistentHash(set, func(_ context.Context, request any) string {
		return request.(*GetProfileRequest).TenantID
	}, balancer.WithReplicas(200))
})
```

Weighted and hash strategies need to know which instance produced an endpoint,
so they take `endpointer.InstanceEndpointer` rather than the narrower
`endpointer.Endpointer`. `endpointer.NewEndpointer` already returns it.

Two behaviours are worth knowing before you pick one:

- Weighted random reports `sd.ErrNoEndpoints` when endpoints exist but every
  weight is zero or below. That is a selectable-instance shortage, and the
  default retry classifier treats it as temporary.
- Consistent hash routes through `sd.RequestBalancer`, the request-aware
  contract. `sd/retry` prefers it automatically. Calling `Endpoint()` directly
  has no request to key on and falls back to a random pick, so an unkeyed
  request is never pinned to one instance.

Retry re-selects on every attempt, so a failed instance is not retried unless
the balancer hands it back. With consistent hashing that is intentional: the
same key resolves to the same instance until the set changes.

## Architecture

```
Instancer  →  Endpointer  →  Balancer  →  Retry  →  Endpoint
```

Each layer is independently usable:

```go
// Manual assembly (full control)
ep   := endpointer.NewEndpointer(instancer, factory, logger)
defer ep.Close()
lb   := balancer.NewRoundRobin(ep)
call := retry.Retry(3, 500*time.Millisecond, lb)
```

For low-level assembly, cache invalidation is configured with
`endpointer.InvalidateOnError`. The higher-level `client.NewEndpoint`
constructor exposes the equivalent `client.WithInvalidateOnError` option.

## Retry strategies

```go
// Fixed max attempts
retry.Retry(3, time.Second, lb)

// Production calls should provide an explicit retry classifier.
retry.WithClassifier(time.Second, lb,
    func(n int, err error) (keepTrying bool, replacement error) {
        return n < 5, nil
    },
	func(err error) bool {
		var retryable interface{ Retryable() bool }
		return errors.As(err, &retryable) && retryable.Retryable()
	},
)
```

The default classifier retries explicit `Retryable() == true` errors and
temporary no-endpoint conditions. Unknown and protocol errors are permanent.
For gRPC, pass `integrations/grpc.Retryable` explicitly through
`client.WithRetryable`; domain write safety remains an application decision.

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
