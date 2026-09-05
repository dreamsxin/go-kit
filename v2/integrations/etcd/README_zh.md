# etcd 集成

`integrations/etcd` 是发布版 v2 module 中的可选 provider package。它在服务前缀下为每个实例写入
带租约的注册键，并向 `sd.Instancer` 契约发布不可变快照；`contract.go` 在编译期断言这一点。

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

instancer 在 watch 通知后重新读取前缀，watch 断开后从上次 revision 继续。调用 `Close`
会取消 watch 并释放 provider 资源。

## 注册冲突语义

etcd 在写入前会先比较，所以它是唯一能强制执行全部三种 `sd.Conflict` 的 provider：

```go
registrar := etcd.NewRegistrar(client, slog.Default(), "users", "10.0.0.4", 8080,
    etcd.ConflictRegistrarOptions(sd.ConflictCompareAndSwap),
)
```

- `sd.ConflictOverwrite` 是默认值，也是唯一能自行恢复的一种：上一次运行非正常退出的实例
  可以直接重新注册，不需要人去删掉它遗留的键。
- `sd.ConflictCreateOnly` 只在键不存在时注册。当"实例身份重复"是一个值得让启动失败的部署
  错误时选它，代价是非正常退出遗留的键会阻塞注册，直到其租约过期。
- `sd.ConflictCompareAndSwap` 在键不存在、或键仍持有本注册方上次写入的内容时注册。它对首次
  注册是 create-only，对本注册方自己的续注册是 overwrite，因此丢失租约仍能恢复，又不会把
  已经属于别人的键抢回来。

在比 overwrite 更严格的设置下，键被他人占用会让 `Register` 返回一个包装了 `sd.ErrConflict`
的错误。对这个实例来说这是永久状态：负责在丢失租约后重新注册的 supervisor 会停止而不是重试，
因为用同一个身份重试要么永远失败，要么把键抢回来。默认的 overwrite 模式下注册方会续租并在
keepalive 丢失后重新注册；在更严格的设置下，一旦另一个写入方拥有了该键，这种恢复就结束了。

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
