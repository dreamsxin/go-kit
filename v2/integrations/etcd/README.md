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
