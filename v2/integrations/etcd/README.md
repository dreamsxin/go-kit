# etcd Integration

`integrations/etcd` is an independent provider module. It stores one leased
registration per instance below a service prefix and publishes immutable
snapshots to the v2 `sd.Instancer` contract without importing the core module.

```go
client := etcd.NewClient(rawClient)
registrar := etcd.NewRegistrar(client, slog.Default(), "users", "10.0.0.4", 8080,
    etcd.MetaRegistrarOptions(map[string]string{"zone": "cn-north-1a", "weight": "5"}),
)
if err := registrar.Register(); err != nil { return err }
defer registrar.Deregister() //nolint:errcheck

instancer := etcd.NewInstancer(client, slog.Default(), "users")
defer instancer.Close() //nolint:errcheck
```

Registrations renew their lease and re-register after a lost keepalive. The
instancer re-reads the prefix on watch notifications and resumes from the last
revision after a watch disconnect. Call `Close` to cancel the watch and release
provider resources.

## Tests

`go test ./...` needs no etcd: the suite drives a fake `Client`, which is what
makes lock ordering and timeout budgets deterministic.

The lease mechanics cannot be faked honestly — etcd decides when a key expires —
so `live_test.go` runs against a real cluster and is skipped unless
`GOKIT_ETCD_ENDPOINTS` names one:

```sh
etcd --data-dir /tmp/etcd-live \
    --listen-client-urls http://127.0.0.1:23790 \
    --advertise-client-urls http://127.0.0.1:23790

GOKIT_ETCD_ENDPOINTS=127.0.0.1:23790 go test ./ -run Live -v
```

Each test works under a namespace unique to its name and start time and deletes
it on cleanup, so a shared cluster is safe. The five cases cover discovery
through a watch, keepalive outliving three TTLs, expiry when renewal stops
(a killed process), `Deregister` revoking the lease rather than letting it
expire, and `Register` racing `Deregister`.
