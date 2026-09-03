# 核心概念

[English](concepts.md) | 简体中文

三条不变量定义了 go-kit v2，其他一切都建立在其上。

## 先记住这四件事

| 问题 | 答案 |
| --- | --- |
| 业务逻辑放在哪里？ | 放在与协议无关的 `service` 函数中。 |
| 超时、重试和准入策略放在哪里？ | 端点中间件，或服务发现的执行层。 |
| 谁拥有监听器、客户端和 goroutine？ | 创建它的组件或构造函数；先关消费者，再关 provider。 |
| 错误如何跨协议？ | 传输边界把 `apperror.Kind` 映射为 HTTP 或 gRPC 状态码。 |

请求策略见[中间件](middleware_zh.md)，对外错误见[错误处理](errors_zh.md)，
启动和停机见[生命周期](lifecycle_zh.md)。

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

业务代码对一次失败只需表达 kind，由各传输层翻译成协议状态：`KindNotFound`
对应 HTTP 404 与 gRPC `NotFound`，`KindDeadlineExceeded` 对应 504 与
`DeadlineExceeded`，以此类推。两种传输的完整 kind 映射表见
[错误处理](errors_zh.md#apperror-参考)。

> [!NOTE]
> 框架完全无法分类的错误编码为 500 且消息已脱敏——内部错误字符串绝不会到达客户端。
> 但"已分类"比"用 `apperror` 构造"更宽：编码器同样识别
> `transporthttp.StatusCoder` 与 `PublicMessager`，因此 `endpoint.ValidationError`
> 得到 400，准入控制哨兵错误得到 503/429，转发的 `client.HTTPStatusError` 保留上游
> 状态码。裸的 `context.DeadlineExceeded` 与 `context.Canceled` 也分别映射为 504
> 与 499，而不是 500。`server.HTTPStatusForError` 是回答"这个错误会得到什么状态码"
> 的唯一函数。

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
