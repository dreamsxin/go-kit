# Consul Integration
English | [简体中文](README_zh.md)

`integrations/consul` is an independent provider module. It depends on the
Consul SDK and the Go standard library, but not on the go-kit runtime module.

```go
package main

import (
	"log/slog"

	kitconsul "github.com/dreamsxin/go-kit/v2/integrations/consul"
	consulapi "github.com/hashicorp/consul/api"
)

func main() {
	raw, err := consulapi.NewClient(consulapi.DefaultConfig())
	if err != nil {
		panic(err)
	}
	client := kitconsul.NewClient(raw)

	registrar := kitconsul.NewRegistrar(client, slog.Default(), "users", "127.0.0.1", 8080,
		kitconsul.MetaRegistrarOptions(map[string]string{
			"zone":    "cn-north-1a",
			"version": "v2",
			"weight":  "5",
		}),
	)
	if err := registrar.Register(); err != nil {
		panic(err)
	}
	defer registrar.Deregister() //nolint:errcheck

	instancer := kitconsul.NewInstancer(client, slog.Default(), "users", true)
	defer instancer.Close() //nolint:errcheck
}
```

`Instancer` publishes copied, immutable-by-convention snapshots and satisfies
the core `sd.Instancer` contract structurally. Applications that use
`sd/client` can pass it directly without an adapter. Provider lifecycle remains
application owned: close discovery consumers before calling `Close`.

## Instance labels

`MetaRegistrarOptions` reports static labels with the registration, and the
instancer lifts the service `Meta` of every discovered entry back into
`Instance.Metadata`, so `sd/endpointer` filters and `sd/balancer` weights can
read them. Repeated calls merge rather than replace, which lets separate config
sources each contribute labels.

Two boundaries are deliberate:

- Tags are not metadata. They are a set, not key/value pairs, and
  `TagsInstancerOptions` already filters on them (the first tag server-side, the
  rest locally).
- Live load does not belong in the catalog. A registry write per metric sample
  would hammer Consul and consumers would still read stale numbers; use
  `feedback.Table.LeastRequest`, which measures in-flight requests in process.

A metadata-only change counts as a change: relabelling an instance is broadcast
to subscribers even when the address set is identical.
