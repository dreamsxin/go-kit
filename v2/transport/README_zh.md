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
  `EncodeJSONResponse`）；
- 错误编码器（`server.ServerErrorEncoder`）；
- 请求体解码器（构造时的 `DecodeRequestFunc`）。

多重格式转换仍然可以组合，但发生在被安装的那一个函数内部：信封编码器
可以自行序列化、后处理或委托其他逻辑。

**组合多个请求解析器。** 一条路由只有一个 body 解码器，但 body、路径、
查询与 multipart 输入可以在其中组合。共享的辅助函数从不同来源填充同一个
请求结构体，且同名字段路径值优先于查询值：

```go
func decodeList(ctx context.Context, r *http.Request) (any, error) {
    var req ListOrdersRequest          // form/json 标签映射查询与路径字段
    if err := transporthttp.DecodeQueryRequest(r, &req); err != nil {
        return nil, err                // 查询与路径都写入这里
    }
    if err := transporthttp.DecodePathRequest(r, &req); err != nil {
        return nil, err
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

- `server.NewServer`
- `server.NewTypedJSONServer`
- `server.NewJSONServer`
- `server.NewJSONEndpoint`
- `server.NewStrictTypedJSONServer`
- `server.NewStrictJSONServer`
- `server.NewStrictJSONEndpoint`
- `server.NewTypedJSONServerWithMiddleware`
- `server.NewJSONServerWithMiddleware`
- `server.DecodeJSONRequest`
- `server.DecodeJSONRequestWithOptions`
- `server.DecodeJSONBody`
- `server.StrictJSONDecodeOptions`
- `server.DefaultMaxJSONBodyBytes`
- `server.EncodeJSONResponse`
- `server.JSONErrorEncoder`
- `server.NewHTTPError`
- `server.WrapHTTPError`
- `server.ParseMultipartForm` 用于有界的 multipart/form-data 上传
- `server.WriteAttachment` 用于文件下载

`ParseMultipartForm` 在分片溢出到临时文件之前强制执行总请求体上限、单文件上限
和内存阈值；超限违规通过标准错误编码器归类为 413，格式错误的请求归类为
415/400。`WriteAttachment` 设置经过净化的 `Content-Disposition`（非 ASCII
名称使用 RFC 2231），并根据文件名推导内容类型。

主要扩展点：

- `ServerBefore`
- `ServerAfter`
- `ServerFinalizer`
- `ServerErrorHandler`
- `ServerErrorEncoder`
- `ServerErrorEncoder`
- `ServerErrorHandler`

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

当响应具有具体类型时，优先使用完全类型化的辅助函数。JSON 辅助函数默认是
严格的：它们会拒绝未知对象字段、第二个 JSON 值，以及超过默认字节上限的
请求体。当某个路由需要自定义请求体上限时，使用显式的 strict 辅助函数：

```go
handler := server.NewStrictJSONEndpoint[HelloReq](
    ep,
    server.DefaultMaxJSONBodyBytes,
    server.ServerErrorEncoder(server.JSONErrorEncoder),
)
```

JSON 请求解码器返回的解码错误会为 `JSONErrorEncoder` 携带 HTTP 400 状态
元数据。

`JSONErrorEncoder` 写入 `code`、`message` 和可选的 `request_id` 字段。在应用
代码中优先使用 `apperror.New` 或 `apperror.Wrap`，使失败分类保持独立于
HTTP。HTTP 传输层将应用错误类别映射到状态码，并使用其稳定代码与公开消息。
底层 HTTP 集成仍可返回 `server.NewHTTPError`，或实现
`transporthttp.StatusCoder`、`transporthttp.ErrorCoder` 和
`transporthttp.PublicMessager`。两个内置错误编码器都会对未分类的 5xx 细节
进行脱敏，而不是暴露内部错误字符串。

## HTTP 客户端

在通过端点风格的抽象调用 HTTP API 时使用 `transport/http/client`。

推荐入口：

- `client.NewClient`
- `client.NewJSONClient`
- `client.NewJSONClientWithMaxResponseBodyBytes`
- `client.NewJSONClientWithTimeout`
- `client.EncodeJSONRequest`

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
