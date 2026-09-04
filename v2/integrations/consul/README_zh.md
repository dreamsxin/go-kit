# Consul 集成

[English](README.md) | 简体中文

`integrations/consul` 是 `github.com/dreamsxin/go-kit/v2` 模块中的可选 provider package。
它依赖 Consul SDK，而通用服务发现 package 仍保持 provider 中立。

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

`Instancer` 发布经过拷贝的、按约定不可变（immutable-by-convention）的快照，并在结构上满足核心的 `sd.Instancer` 契约。使用 `sd/client` 的应用可以直接传入它，无需适配器。Provider 的生命周期仍由应用负责：请在调用 `Close` 之前关闭所有服务发现消费方。

## 实例标签

`MetaRegistrarOptions` 在注册时上报静态标签，instancer 会把发现到的每个条目的服务 `Meta` 还原成 `Instance.Metadata`，从而供 `sd/endpointer` 过滤和 `sd/balancer` 取权重使用。多次调用是合并而非覆盖，便于不同配置来源各自贡献标签。

有两条边界是刻意划定的：

- Tags 不是元数据。它是集合而非键值对，且 `TagsInstancerOptions` 已经在做过滤（第一个 tag 在服务端，其余在本地）。
- 实时负载不该进 catalog。每采样一次指标就写一次注册中心会打爆 Consul，而消费者读到的仍是过期数字；请改用 `feedback.Table.LeastRequest`，它在进程内统计在途请求。

只改元数据也算变更：即便地址集合完全相同，重新打标签也会广播给订阅方。
