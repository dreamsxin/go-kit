# Consul 集成

[English](README.md) | 简体中文

`integrations/consul` 是一个独立的 provider 模块。它依赖 Consul SDK 和 Go 标准库，但不依赖 go-kit 运行时模块。

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

`Instancer` 发布经过拷贝的、按约定不可变（immutable-by-convention）的快照，并在结构上满足核心的 `sd.Instancer` 契约。使用 `sd/client` 的应用可以直接传入它，无需适配器。Provider 的生命周期仍由应用负责：请在调用 `Stop` 之前关闭所有服务发现消费方。
