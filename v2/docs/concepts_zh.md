# 核心概念

[English](concepts.md) | 简体中文

三条不变量定义了 go-kit v2，其他一切都建立在其上。

## 请求路径

```text
Transport request
    -> decode
    -> endpoint middleware
    -> endpoint
    -> service method
    -> encode
    -> transport response
```

每一层各自拥有一类决策，且不得染指其他层的决策：

| 层 | 拥有 | 不得拥有 |
| --- | --- | --- |
| 服务层 | 业务规则与领域编排 | HTTP/gRPC 类型与状态码映射 |
| 端点 | 传输中立的请求边界与中间件 | 套接字/服务器生命周期 |
| 传输层 | 协议解码、编码、头部与状态码 | 业务规则与重试策略 |
| 装配层 | 依赖装配与进程生命周期 | 隐藏的全局状态 |

其实际结果是：业务逻辑就是一个 `(context, Request) -> (Response, error)` 的普通
函数；它无需网络即可单元测试，并且可以在 HTTP、gRPC 与 MCP 传输层之间复用。

## 错误分类

库返回错误；传输层负责把它们映射为协议状态码。业务错误用 `apperror` 分类：

```go
return Todo{}, apperror.New(apperror.KindNotFound, "todo.not_found", "todo not found")
```

| Kind | HTTP 状态码 | gRPC 码 |
| --- | --- | --- |
| `KindInvalidArgument` | 400 | InvalidArgument |
| `KindUnauthenticated` | 401 | Unauthenticated |
| `KindPermissionDenied` | 403 | PermissionDenied |
| `KindNotFound` | 404 | NotFound |
| `KindAlreadyExists`、`KindConflict` | 409 | AlreadyExists / Aborted |
| `KindResourceExhausted` | 429 | ResourceExhausted |
| 未分类 | 500 | Unknown（对客户端不透明） |

> [!NOTE]
> 未分类的错误绝不会向客户端泄露内部细节。只有用 `apperror` 分类的错误才携带
> 公开消息。

完整流程（包括自定义错误格式）见[错误处理](errors_zh.md)。

## 生命周期归属

进程入口拥有信号与根 context。框架服务在启动成功后拥有监听器与优雅停机：

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
if err := host.Run(ctx); err != nil {
    return err
}
```

启动错误同步浮现；Host 在停机后不能重启。可选组件（后台任务、gRPC 监听器、
HTTP 组件）通过 `kit.Lifecycle` 挂载，并共享同一段有边界的停机过程。参见[生命周期](lifecycle_zh.md)。
