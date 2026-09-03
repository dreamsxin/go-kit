# transport（传输层）

[English](README.md) | 简体中文

`transport` 层将外部协议适配到框架的端点模型。

它的职责窄且有明确意图：

- 解码传入请求
- 调用端点
- 编码传出响应
- 暴露协议特定的钩子

它不应承载业务逻辑。

## 在架构中的角色

在框架的三层模型中：

- `service` 拥有业务逻辑
- `endpoint` 拥有运行时策略与中间件组合
- `transport` 拥有协议适配

如果一个行为可以表达为端点中间件，它通常就应该放在那里，而不是放在传输层代码中。

## 包概览

核心 transport 模块暴露以下 HTTP 区域：

- `transport/http/server`
- `transport/http/client`

gRPC 是一个可选模块，包含两个公开区域：

- `integrations/grpc/server`
- `integrations/grpc/client`

通用辅助函数还位于：

- `transport/error_handler.go`
- `transport/http`
- `integrations/grpc`

## 双协议绑定

同一个业务 endpoint 可以同时服务 HTTP 与 gRPC，无需重复装配。
`transport.Binding[Req, Resp]` 承载一次构建好的中间件端点；每个协议只
拥有自己的线上编解码：

```go
binding := transport.Binding[HelloRequest, HelloResponse]{
    Endpoint: endpoint.TypedEndpoint[HelloRequest, HelloResponse](sayHello).Wrap(),
}

// HTTP：一次调用完成严格类型化 JSON。
kit.HandleJSONTyped(httpComponent, "POST /hello", binding.TypedEndpoint())

// gRPC：protobuf 编解码把线上消息映射为领域类型。
srv := grpcserver.NewServer(
    binding.Endpoint,
    func(_ context.Context, m any) (any, error) {
        r := m.(*pb.HelloRequest)
        return HelloRequest{Name: r.Name}, nil
    },
    func(_ context.Context, resp any) (any, error) {
        r := resp.(HelloResponse)
        return &pb.HelloReply{Message: r.Message}, nil
    },
)
pb.RegisterGreeterServer(grpcComponent.Server(), srv)
```

HTTP 侧无需额外工作，因为 `Binding.TypedEndpoint()` 返回 JSON 服务器接受
的类型化端点。gRPC 侧是一次 `grpcserver.NewServer` 调用加两个 protobuf
映射函数。同一条 endpoint 中间件链（超时、指标、熔断等）在两个协议上
运行，因为绑定承载的是已组合好的端点。

完整的可运行测试（一个真实 HTTP 服务器加一个 bufconn 上的真实 gRPC
服务器，由同一个绑定驱动）见 `integrations/grpc/dual_protocol_test.go`。

## 自定义 Body 格式

HTTP 路由不是只能 JSON。两个纯函数即可构成传输编解码器；错误编码器
共享同一内容类型，使错误响应与格式保持一致：

```go
decode, encode := server.RawBodyCodec(
    unmarshalProto,   // func([]byte) (any, error)
    marshalProto,     // func(any) ([]byte, error)
    "application/x-protobuf",
)
svc.Handle("POST /shout", server.NewServer(ep, decode, encode,
    server.ServerErrorEncoder(server.TextErrorEncoder("application/x-protobuf")),
))
```

`RawBodyCodec` 限制请求体（默认 1 MiB，可配置），并保留响应上的
`StatusCoder`/`Headerer` 行为。`TextErrorEncoder` 保持 4xx 消息公开、
5xx 不透明，但以路由格式而非 JSON 输出。`RawBodyCodecWithMaxBytes` 接受
显式上限。

> [!NOTE]
> 这里的 apperror 是可选项。自定义路由可以完全不做分类，让自己的错误
> 编码器决定状态码与响应体；框架不强制任何错误模型。apperror 的存在是
> 为了让常见的 HTTP/gRPC 映射在你需要时自动生效。

可运行的演练见 [examples/customcodec](../examples/README_zh.md)。

## 组合与嵌套

组件按两种明确的风格组合。

**累积式** - 传多少个都可以，按添加顺序叠加：

- `ServerBefore` / `ServerAfter` / `ServerFinalizer` 钩子（以及客户端的
  `Before` / `After` / finalizer 等价物）按注册顺序执行；
- endpoint 中间件（`endpoint.Chain`、`Builder.Use`、
  `kit.WithEndpointMiddleware`）可以任意嵌套：第一个中间件在最外层，
  被包装的 endpoint 本身也可以是一条链（例如 `Fallback` 的兜底端点可以
  是一条带完整中间件的链）；
- `security/http.Chain` 与 `kit.WithHTTPMiddleware` 以相同方式组合标准
  `http.Handler` 中间件；
- `kit.WithJSONServerOptions` 可以多次传入，其选项追加到每个 JSON 路由。

**替换式** - 每条路由只有一个生效，最后设置者胜出：

- 成功响应编码器（`server.ServerResponseEncoder`，默认
  `EncodeJSONResponse`）——注意它只作用于 **JSON 入口**（`NewJSONServer`、
  `NewJSONEndpoint` 及其类型化与 `WithBodyLimit` 变体）。用 `NewServer` 构造的
  服务端在构造参数里接收编码函数，因此传给它的 `ServerResponseEncoder` 会被静默
  覆盖；
- 错误编码器（`server.ServerErrorEncoder`）；
- 请求体解码器（构造时的 `DecodeRequestFunc`）。

多重格式转换仍然可以组合，但发生在被安装的那一个函数内部：信封编码器
可以自行序列化、后处理或委托其他逻辑。

**组合多个请求解析器。** 一条路由只有一个 body 解码器，但 body、路径、
查询与 multipart 输入可以在其中组合。`DecodeQueryRequest` 会**同时**从路径和查询
填充结构体，同名字段路径值优先，因此 GET 风格的路由只需要它：

```go
func decodeList(ctx context.Context, r *http.Request) (any, error) {
    var req ListOrdersRequest          // form/json 标签映射查询与路径字段
    if err := transporthttp.DecodeQueryRequest(r, &req); err != nil {
        return nil, err                // 查询与路径都写入这里
    }
    page, err := transporthttp.ParsePage(r)
    if err != nil {
        return nil, err
    }
    req.Page = page
    return req, nil
}
```

文件上传以同样方式组合：在解码器内用 `server.ParseMultipartForm` 解析表单
字段，再读取文件部分。

**启动期结构体校验。** 基于反射的查询解码在首个请求时才暴露不支持的
字段类型。在装配期校验一次结构体，让错误在启动时快速失败：

```go
if err := transporthttp.ValidateQueryStruct[ListOrdersRequest](); err != nil {
    return err
}
```

**不重写编码器的信封组装。** `server.WrapJSONResponse` 包装响应值，同时
保留原响应的 `StatusCoder` 与 `Headerer` 行为：

```go
kit.NewHTTP(":8080", kit.WithJSONServerOptions(
    server.ServerResponseEncoder(server.WrapJSONResponse(func(response any) any {
        return envelope{Code: 0, Message: "ok", Data: response}
    })),
))
```

## 分页约定

列表端点共享来自 `transport/http` 的同一个分页契约：
`ParsePage` 读取 `?page=` 和 `?size=`（默认值分别为 1 和 20，size 上限为
`MaxPageSize` = 100），对格式非法的值返回编码为 400 的
`endpoint.ValidationError`；`Page.Limit` 和 `Page.Offset` 直接供 SQL 窗口使用；
`NewPageResult` 组装标准的 `items/total/page/size/has_next` 响应结构，
使客户端和生成的 SDK 看到同一个契约。

```go
page, err := transporthttp.ParsePage(r)
if err != nil {
    return err
}
rows, total := repo.List(ctx, page.Limit(), page.Offset())
return transporthttp.NewPageResult(page, total, rows), nil
```

## 钩子语义

在 HTTP 与 gRPC 之间，客户端与服务器传输共享同一套高层钩子模型，尽管它们的具体函数签名是协议特定的。

预期的语义契约是：

- `Before`
  在解码之前或出站调用发出之前运行。
  将它用于请求元数据、头部、认证上下文、追踪上下文和请求关联。
- `After`
  在端点调用成功或远程响应成功之后、但在传输层完成响应写入或返回之前运行。
  将它用于响应元数据、响应头部和可观测性增强。
- `Finalizer`
  无论成功或失败都在最后运行。
  将它用于延迟记录、访问日志、指标和清理。

设计规则：

- 在各传输层实现之间保持这一语义顺序
- 当关注点与传输层无关时，不要用传输层钩子替代端点中间件

## HTTP 服务器

在暴露 HTTP API 时使用 `transport/http/server`。

推荐入口：

- `server.NewServer` —— 通用服务端，解码与编码由你提供
- `server.NewJSONServer` / `server.NewTypedJSONServer` —— 传入处理函数，得到 JSON 路由
- `server.NewJSONEndpoint` —— 同上，但用于你已经构建好的 endpoint
- `server.NewJSONServerWithBodyLimit` / `server.NewTypedJSONServerWithBodyLimit` /
  `server.NewJSONEndpointWithBodyLimit` —— 上述三者的显式请求体上限版本
- `server.NewJSONServerWithMiddleware` / `server.NewTypedJSONServerWithMiddleware`
- `server.NewJSONEndpointWithDecodeOptions` —— 改变严格性本身的逃生口
- `server.DecodeJSONRequest` / `server.DecodeJSONRequestWithOptions` /
  `server.DecodeJSONBody`
- `server.StrictJSONDecodeOptions`、`server.JSONDecodeOptions`、
  `server.DefaultMaxJSONBodyBytes`
- `server.EncodeJSONResponse`、`server.WrapJSONResponse`
- `server.JSONErrorEncoder`、`server.DefaultErrorEncoder`、
  `server.JSONErrorEncoderWithKindMapper`、`server.TextErrorEncoder`
- `server.HTTPStatusForError` / `server.HTTPStatusForErrorKind` —— 某个错误会映射到
  的状态码，用于转发上游失败或编写自定义编码器
- `server.RawBodyCodec` / `server.RawBodyCodecWithMaxBytes` 用于非 JSON 请求体
- `server.NopRequestDecoder` / `server.NopResponseEncoder`
- `server.NewSSEServer` / `server.NewSSEServerTyped` 用于 Server-Sent Events 流
- `server.ParseMultipartForm` 用于有界的 multipart/form-data 上传
- `server.WriteAttachment` 用于文件下载
- `server.AccessLogMiddleware` 用于基于标准库 `slog` 的访问日志

所有 JSON 入口都**严格**解码：未知对象字段、第二个 JSON 值、以及超过
`DefaultMaxJSONBodyBytes` 的请求体都会以 400 拒绝。`WithBodyLimit` 变体只改变
大小上限；只有 `NewJSONEndpointWithDecodeOptions` 能放宽严格性。

`ParseMultipartForm` 在分片溢出到临时文件之前强制执行总请求体上限、单文件上限
和内存阈值；超限违规通过标准错误编码器归类为 413，格式错误的请求归类为
415/400。`WriteAttachment` 设置经过净化的 `Content-Disposition`（非 ASCII
名称使用 RFC 2231），并根据文件名推导内容类型。

### Server-Sent Events 流

`server.NewSSEServer` 与 `NewSSEServerTyped` 把流式处理器适配为
`http.Handler`：`SSEStreamHandler` 拿到解码后的请求与事件写出器 `SSEStream`
（方法集为 Data、Event、EventJSON、Comment、Retry）。钩子语义：

- `ServerBefore` 在流启动前运行；
- 解码函数在任何 SSE 头写出之前运行，解码失败经 ErrorEncoder 映射为常规错误响应（如 400）；
- `ServerErrorHandler` 观察解码失败与流中途返回的错误；
- `ServerFinalizer` 在流结束时总是运行；
- `ServerAfter` 与 `ServerResponseEncoder` 不适用于流，会被忽略。

与 endpoint 中间件组合时，一个流计为一次请求：指标计 1 次，超时中间件约束的是**总流时长**（长时流应避免或放宽全局超时），认证与校验在流启动前执行。流中途返回的错误无法到达客户端；需要告知客户端失败时，业务应自行发送终结事件。

主要扩展点：

- `ServerBefore`
- `ServerAfter`
- `ServerFinalizer`
- `ServerErrorHandler`
- `ServerErrorEncoder`
- `ServerResponseEncoder`（仅 JSON 入口）

默认的错误处理器是 no-op。当错误必须被记录或上报时，应安装应用自有的处理器，
例如来自 `integrations/zap` 的 `zapadapter.NewErrorHandler(logger)`。

典型流程：

1. `ServerBefore` 钩子从请求填充上下文。
2. 解码函数将 HTTP 输入映射为领域请求。
3. 端点被调用。
4. `ServerAfter` 钩子检查或增强响应路径。
5. 编码函数写入响应。
6. 无论成功或失败，Finalizer 都会运行。

最小示例：

```go
handler := server.NewTypedJSONServer(
    func(ctx context.Context, req HelloReq) (HelloResp, error) {
        return hello(ctx, req)
    },
    server.ServerErrorEncoder(server.JSONErrorEncoder),
)

http.Handle("/hello", handler)
```

当响应具有具体类型时，优先使用完全类型化的辅助函数。所有 JSON 辅助函数都是
严格的：它们会拒绝未知对象字段、第二个 JSON 值，以及超过默认字节上限的
请求体。当某个路由需要不同的请求体上限时，使用 `WithBodyLimit` 辅助函数：

```go
handler := server.NewJSONEndpointWithBodyLimit[HelloReq](
    ep,
    64<<10,
    server.ServerErrorEncoder(server.JSONErrorEncoder),
)
```

JSON 请求解码器返回的解码错误会为 `JSONErrorEncoder` 携带 HTTP 400 状态
元数据。

`JSONErrorEncoder` 写入 `code`、`message` 和可选的 `request_id` 字段。在应用
代码中优先使用 `apperror.New` 或 `apperror.Wrap`，使失败分类保持独立于
HTTP。HTTP 传输层将应用错误类别映射到状态码，并使用其稳定代码与公开消息。
底层 HTTP 集成可实现
`transporthttp.StatusCoder`、`transporthttp.ErrorCoder` 和
`transporthttp.PublicMessager`。三个内置错误编码器对消息共用一条规则：
`PublicMessager` 优先，低于 500 的状态码可以回落到 `err.Error()`，500 一律读作
"Internal Server Error"——绝不是内部错误字符串。

## HTTP 客户端

在通过端点风格的抽象调用 HTTP API 时使用 `transport/http/client`。

推荐入口：

- `client.NewClient`
- `client.NewJSONClient`
- `client.NewJSONClientWithMaxResponseBodyBytes`
- `client.NewJSONClientWithTimeout`
- `client.EncodeJSONRequest`
- `client.DecodeJSONResponse`
- `client.DecodeJSONResponseWithMaxBodyBytes`

`NewJSONClient` 将 GET/HEAD 请求编码为路径/查询参数，并将请求体保持为空。
成功的 JSON 响应默认上限为 4 MiB；当需要刻意采用更大的契约时，使用
`NewJSONClientWithMaxResponseBodyBytes`。`NewJSONClientWithTimeout` 会添加
上下文超时；当需要重试时，应使用带显式重试分类器的 `sd/client.NewEndpoint`。

主要扩展点：

- `ClientBefore`
- `ClientAfter`
- `ClientFinalizer`
- 自定义请求编码器
- 自定义响应解码器
- 自定义 HTTP 客户端注入

`DecodeJSONResponse` 是默认解码器的导出形式，使用 `NewExplicitClient` 组装
自定义客户端（或在其外层包一层）时，可以复用同样的状态处理与响应体限制，
而无需重新实现。

最小示例：

```go
ep, err := client.NewJSONClient[HelloResp](
    http.MethodPost,
    "http://localhost:8080/hello",
)
if err != nil {
    return err
}

resp, err := ep(ctx, HelloReq{Name: "world"})
```

典型流程：

1. `ClientBefore` 钩子增强出站请求上下文或头部。
2. 请求被编码并发送。
3. 响应被解码。
4. `ClientAfter` 钩子检查成功响应路径。
5. 无论成功或失败，Finalizer 都会运行。

## gRPC 服务器

在暴露 gRPC API 时使用 `integrations/grpc/server`。

推荐入口：

- `server.NewServer`
- 公开的请求/响应编解码钩子

主要扩展点：

- `ServerBefore`
- `ServerAfter`
- `ServerFinalizer`

典型流程与 HTTP 服务器路径一致：

1. 请求元数据被读入上下文
2. 请求被解码为领域请求
3. 端点被调用
4. 可以写入响应元数据
5. 响应被编码并返回给 gRPC 调用方

默认错误编码器保留既有的 gRPC status 错误，将传输层无关的 `apperror` 错误
类别映射到 gRPC 码，并将未知失败脱敏为 `codes.Internal`。仅当应用需要不同的
线上错误策略时才安装 `ServerErrorEncoder`；错误处理器仍会收到原始错误，
用于日志和指标。

## gRPC 客户端

在通过框架抽象发起 gRPC 调用时使用 `integrations/grpc/client`。

推荐入口：

- `client.NewClient`
- 公开的编码/解码函数

主要扩展点：

- `ClientBefore`
- `ClientAfter`
- `ClientFinalizer`

典型流程与 HTTP 客户端路径一致：

1. `ClientBefore` 钩子增强出站元数据。
2. 请求被编码并发送。
3. 响应被解码。
4. `ClientAfter` 钩子检查成功的响应元数据。
5. 无论成功或失败，Finalizer 都会运行。

当前元数据说明：

- gRPC 客户端的响应头部与 trailer 通过 `integrations/grpc` 的上下文键暴露在上下文中，供解码/Finalizer 时点检查。

## 什么属于传输层

良好的传输层职责：

- HTTP 请求解析
- gRPC 元数据提取
- JSON 编码与解码
- 响应状态映射
- 线上错误编码
- 协议特定的钩子

## 什么不属于传输层

避免把这些关注点放到这里：

- 领域决策逻辑
- 属于服务层逻辑的业务校验
- 当超时、重试、日志、限流或熔断可以被建模为端点中间件时，就不要放在传输层
- 一次性产品工作流行为

这些是框架反模式，因为它们会削弱协议与业务逻辑之间的分离。

## 最佳实践

1. 保持请求/响应映射显式。
2. 为可复用的运行时策略优先使用端点中间件。
3. 常见 HTTP 场景使用 JSON 辅助函数，而不是手写样板代码。
4. 保持传输层代码小巧且易于替换。
5. 将传输层钩子用于元数据和可观测性，而非业务工作流。

## 稳定性说明

已发布的 `v2.0.0` transport API 仍是历史性的稳定基线。获得批准的 `v2.1.0`
SemVer 例外将该契约重置为评审过的 API 快照。在 `v2.1.0` 之后，兼容性覆盖
文档化的行为，而不覆盖内部执行细节，例如确切的 writer 拦截或内部请求
生命周期结构。

## 相关文档

- [README.md](../README_zh.md)
- [ARCHITECTURE.md](../ARCHITECTURE_zh.md)
- [PRODUCTION.md](../PRODUCTION_zh.md)
