# etcd 集成

`integrations/etcd` 是发布版 v2 module 中的可选 provider package。它在服务前缀下为每个实例写入
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

## 测试

`go test ./...` 不需要 etcd：测试套件驱动的是一个 fake `Client`，这样锁顺序与超时预算
才是确定性的。

租约机制没法用 fake 诚实地模拟——key 什么时候过期是 etcd 决定的——所以
`live_test.go` 跑在真实集群上，且只在 `GOKIT_ETCD_ENDPOINTS` 指定了地址时才执行：

```sh
etcd --data-dir /tmp/etcd-live \
    --listen-client-urls http://127.0.0.1:23790 \
    --advertise-client-urls http://127.0.0.1:23790

GOKIT_ETCD_ENDPOINTS=127.0.0.1:23790 go test ./ -run Live -v
```

每个用例在以自身名字与启动时间命名的独立 namespace 下工作，并在 cleanup 时删除它，
因此共享集群也是安全的。五个用例覆盖：经 watch 完成发现、keepalive 活过三个 TTL、
停止续租后过期（进程被杀）、`Deregister` 主动撤销租约而不是等它过期，以及
`Register` 与 `Deregister` 竞争。
