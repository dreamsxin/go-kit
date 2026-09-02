# etcd 集成

`integrations/etcd` 是独立的 provider 模块。它在服务前缀下为每个实例写入
带租约的注册键，并向 v2 `sd.Instancer` 契约发布不可变快照，不依赖核心模块。

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

注册方会续租，并在 keepalive 丢失后重新注册。instancer 在 watch 通知后重新读取
前缀，watch 断开后从上次 revision 继续。调用 `Close` 会取消 watch 并释放 provider
资源。
