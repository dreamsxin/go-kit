# etcd Integration

English | [简体中文](README_zh.md)

`integrations/etcd` is an optional discovery provider package in the published
v2 module. It stores one leased registration per
instance below a service prefix and publishes immutable snapshots through the
`sd.Instancer` contract; `contract.go` asserts that at compile time.

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

The instancer re-reads the prefix on watch notifications and resumes from the
last revision after a watch disconnect. Call `Close` to cancel the watch and
release provider resources.

## Registration conflict semantics

etcd compares before it writes, so this is the one provider that can enforce all
three `sd.Conflict` values:

```go
registrar := etcd.NewRegistrar(client, slog.Default(), "users", "10.0.0.4", 8080,
    etcd.ConflictRegistrarOptions(sd.ConflictCompareAndSwap),
)
```

- `sd.ConflictOverwrite` is the default, and the only one that recovers unaided:
  an instance whose previous run exited uncleanly registers again without a human
  deleting the key it left behind.
- `sd.ConflictCreateOnly` registers only while the key is absent. Choose it when a
  duplicate instance identity is a deployment mistake worth failing start-up
  over, and accept that a key outliving an unclean exit blocks registration until
  its lease expires.
- `sd.ConflictCompareAndSwap` registers while the key is absent or still holds
  what this registrar last wrote. It is create-only for a first registration and
  overwrite for the registrar's own renewals, so a lost lease is recovered
  without stealing a key that now belongs to somebody else.

Under anything stricter than overwrite, a contested key makes `Register` return
an error wrapping `sd.ErrConflict`. That is a permanent condition for this
instance: the supervisor that re-registers after a lost lease stops instead of
retrying, because retrying the same identity either fails forever or steals the
key back. Registrations renew their lease and re-register after a lost keepalive
in the default overwrite mode; under a stricter setting that recovery ends the
moment another writer owns the key.

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
