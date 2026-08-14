# Consul Integration

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

	registrar := kitconsul.NewRegistrar(client, slog.Default(), "users", "127.0.0.1", 8080)
	if err := registrar.Register(); err != nil {
		panic(err)
	}
	defer registrar.Deregister() //nolint:errcheck

	instancer := kitconsul.NewInstancer(client, slog.Default(), "users", true)
	defer instancer.Stop()
}
```

`Instancer` publishes copied, immutable-by-convention snapshots and satisfies
the core `sd.Instancer` contract structurally. Applications that use
`sd/client` can pass it directly without an adapter. Provider lifecycle remains
application owned: close discovery consumers before calling `Stop`.
