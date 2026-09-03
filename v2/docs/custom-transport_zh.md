# 自定义传输协议

[English](custom-transport.md) | 简体中文

当线路协议不是内置的 JSON HTTP 或 gRPC 时阅读本章。原则很简单：传输层只负责
翻译字节和协议元数据，endpoint 与 service 不变。

## 选择最小适配器

| 场景 | 使用 |
| --- | --- |
| HTTP 上使用其他 body 格式 | `server.RawBodyCodec` + `server.NewServer` |
| 自定义 HTTP 错误格式 | `server.ServerErrorEncoder` |
| 新的 socket、队列或 RPC 协议 | 围绕 `endpoint.Endpoint` 编写小适配器 |
| 长连接消息 | 支持取消和显式关闭的流式适配器 |

HTTP 编解码先看可运行的 [customcodec 示例](../examples/customcodec/main.go)；常用
HTTP/gRPC 入口见 [transport 参考](../transport/README_zh.md)。

## 自定义 HTTP Body 格式

协议仍是 HTTP 时，只需要两个函数：

```go
decode, encode := server.RawBodyCodec(
    func(body []byte) (any, error) { return decodeMessage(body) },
    func(response any) ([]byte, error) { return encodeMessage(response) },
    "application/x-message",
)

handler := server.NewServer(
    ep,
    decode,
    encode,
    server.ServerErrorEncoder(server.TextErrorEncoder("application/x-message")),
)
```

`RawBodyCodec` 使用默认请求体上限；需要其他上限时使用
`RawBodyCodecWithMaxBytes`。错误编码器也使用同一媒体类型，状态码和错误分类规则
仍由传输层统一处理。

## 新协议适配器

一个适配器分四步：

1. 从连接或消息派生 request context；
2. 把线路请求解码为领域 request；
3. 每条消息调用一次 endpoint；
4. 编码响应，或把错误映射为协议错误。

核心可以保持很小：

```go
type Adapter struct {
    Endpoint endpoint.Endpoint
    Decode   func(context.Context, []byte) (any, error)
    Encode   func(context.Context, any) ([]byte, error)
    Error    func(context.Context, error) error
}

func (a Adapter) Handle(ctx context.Context, wire []byte) ([]byte, error) {
    request, err := a.Decode(ctx, wire)
    if err != nil {
        return nil, a.Error(ctx, err)
    }
    response, err := a.Endpoint(ctx, request)
    if err != nil {
        return nil, a.Error(ctx, err)
    }
    return a.Encode(ctx, response)
}
```

适配器拥有分帧、消息大小、协议状态、header/trailer 和连接关闭；endpoint 拥有超时、
校验、认证中间件、指标、重试策略和业务错误。不要把业务决策塞进 `Decode`、
`Encode` 或协议错误映射器。

## 错误与 Context 规则

- 保留原始错误，确保日志和 `errors.Is`/`errors.As` 仍然有效。
- 在一个位置把 `apperror.Kind` 映射为协议状态或错误码。
- 协议有对外消息时使用 `PublicMessage`；`Error()` 只用于诊断。
- 把连接取消传播给 endpoint；context 结束后流不能继续读写。
- 为每个 frame、请求、响应和队列设上限；自定义 decoder 是不可信输入边界。

如果协议响应含有 failure 字段，要先决定它是业务数据还是错误。只有响应类型无法
返回 error 时才使用 `endpoint.Failer`，否则正常返回 error。

## 流式协议与生命周期

长连接协议必须定义一次 `Done` 何时完成：一条消息结束、连接关闭，还是整个流终止。
通用 endpoint 层无法推断这个边界，必须由适配器决定。适配器还要提供有界的关闭
路径，取消后不能留下阻塞在连接或队列上的 goroutine。

建议分三级测试：

- 纯 decode/encode 测试：覆盖畸形和超大 frame；
- endpoint 测试：覆盖取消、错误分类和 `Done` 时机；
- 协议 smoke 测试：使用内存连接或测试服务器。

替换适配器不应要求修改 service 或 endpoint，这正是本章要保护的边界。
