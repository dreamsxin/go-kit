# 自定义

[English](customization.md) | 简体中文

如何在不 fork 框架的前提下自定义日志、中间件与错误。所有做法都保持分层不变
量：分类留在 service 层，横切行为在 endpoint 中间件，协议事实归传输层。

## 自定义日志

### 选择日志去向

生成服务按设计把结构化日志写到 stdout；框架故意不提供日志路径配置项。需要
文件存储时自建 `slog.Handler`——任何实现 `io.Writer` 的目标都可用：

```go
file, err := os.OpenFile("service.log",
    os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
if err != nil {
    return err
}
logger := slog.New(slog.NewJSONHandler(file, &slog.HandlerOptions{
    Level: slog.LevelInfo,
}))
```

同时写 stdout 与文件用 `io.MultiWriter(os.Stdout, file)`。轮转与留存是应用
决策：任何实现 `io.Writer` 的第三方轮转写入器都能接入同一个 handler。

生成项目的日志器在用户自有的 `cmd/main.go` 中由 `logging.level` /
`logging.format` 构建；在那里修改（或在 `config/custom.go` 钩子里构建日志
器），不要修改生成文件。

### 选择记录内容

业务日志行来自 slog 适配器；级别与附加属性是选项：

```go
logging := slogadapter.LoggingMiddleware(logger, "CreateUser",
    slogadapter.WithLevel(slog.LevelWarn),
    slogadapter.WithAttrs(func(ctx context.Context) []slog.Attr {
        return []slog.Attr{slog.String("tenant", tenantFrom(ctx))}
    }),
)
```

自定义属性必须有界且不含敏感信息；适配器绝不记录请求与响应载荷。协议日志行
（方法、路径、状态码、字节数、耗时）来自 `server.AccessLogMiddleware`。

错误默认只被**映射**，不被**记录**：默认的 `ServerErrorHandler` 是空操作。
要在日志里看到传输与端点失败，安装处理器：

```go
kit.NewHTTP(":8080", kit.WithJSONServerOptions(
    server.ServerErrorEncoder(server.JSONErrorEncoder),
    server.ServerErrorHandler(slogadapter.NewErrorHandler(logger)),
))
```

## 编写自定义中间件

### 端点中间件

端点中间件是包裹端点的函数。骨架：

```go
func AuditMiddleware(audit AuditLog) endpoint.Middleware {
    return func(next endpoint.Endpoint) endpoint.Endpoint {
        return func(ctx context.Context, req any) (any, error) {
            // before：读请求、检查前置条件、富化 ctx
            resp, err := next(ctx, req)
            // after：观察 resp/err、记录、装饰
            audit.Record(ctx, req, err)
            return resp, err
        }
    }
}
```

保持安全的规则：

- 保持传输中立：不出现 `http` 或 gRPC 类型；中间件只看到解码后的请求与业务
  错误。
- 用 `apperror` 拒绝，让每种传输一致映射失败。
- 从 context 读关联 ID（`endpoint.RequestIDFromContext(ctx)`、
  `TraceContextFromContext`），不要另造管道。
- 有意识地选择模式：不调用 `next` 直接短路（校验、舱壁），或包裹调用（超
  时、指标）。四种模式见[中间件](middleware_zh.md#流控)。

security 包提供了打包好的中间件范例：

```go
security.Middleware(myAuthenticator)   // 确立 security.Subject
security.RequireRole("admin")          // 在 endpoint 层强制
```

### 安装位置

| 作用域 | API |
| --- | --- |
| 每条路由 | `kit.WithEndpointMiddleware(mw...)` |
| 单条路由（类型化） | `kit.HandleJSONTypedWithMiddleware(svc, pattern, handler, compose)` |
| 单条路由（已有端点） | `endpoint.NewBuilder(ep).Use(mw).Build()` + `kit.HandleJSONEndpoint` |
| 单个端点临时组合 | `endpoint.NewBuilder(ep).Use(mw).Build()` |

第一个中间件位于最外层；路由级中间件运行在组件级链**之内**。启动时用
`endpoint.Builder.Describe()` 打印装配好的链（自定义中间件用 `UseNamed` 打
标签）。

### HTTP 中间件

协议级行为（panic 恢复、请求转储、IP 策略）使用标准 `http.Handler` 中间
件：

```go
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            defer func() {
                if p := recover(); p != nil {
                    logger.Error("panic", "err", p, "path", r.URL.Path)
                    http.Error(w, "internal error", http.StatusInternalServerError)
                }
            }()
            next.ServeHTTP(w, r)
        })
    }
}

svc, err := kit.NewHTTP(":8080",
    kit.WithHTTPMiddleware(Recover(logger)),
)
```

边界规则：HTTP 中间件看到方法、路径、头部与状态码——绝不解读业务结果。需要
解码后请求的行为属于端点中间件。浏览器策略用 `security/http.Chain` 组合。

## 自定义错误

错误自定义的完整教学在[错误处理](errors_zh.md)；这里是决策表：

| 需求 | 做法 | 位置 |
| --- | --- | --- |
| 分类业务失败 | `apperror.New/Wrap(kind, code, message)` | service 层 |
| 校验请求字段 | `endpoint.Validatable` + `WithValidation()` | 请求类型 + builder |
| 自定义种类 + 自定义状态码 | `server.JSONErrorEncoderWithKindMapper`（HTTP）/ `grpcserver.ErrorEncoderWithKindMapper`（gRPC） | 装配处 |
| 自定义线上格式/信封 | `server.ServerErrorEncoder` + `server.ServerResponseEncoder` | 装配处 |
| 所有 JSON 路由统一信封 | `kit.WithJSONServerOptions` | 装配处 |
| 与内置映射组合 | `server.HTTPStatusForError` / `HTTPStatusForErrorKind` | 自定义编码器 |

经验法则：在 service 层用 `apperror` 分类；业务代码绝不返回协议类型；4xx 携
带公开消息，500 永远不带；重试决策属于调用方，只重试已分类且幂等的失败。

### 用 `{code, msg, data}` 信封包装响应

有些团队规范要求固定信封与数字业务码。两个钩子即可覆盖——成功路径用
`server.ServerResponseEncoder`，错误路径用 `server.ServerErrorEncoder`——再用
`kit.WithJSONServerOptions` 一次性装到所有 JSON 路由上：

```go
type envelope struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data,omitempty"`
}

func encodeEnvelope(_ context.Context, w http.ResponseWriter, response any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(envelope{Code: 0, Msg: "ok", Data: response})
}

func encodeEnvelopeError(ctx context.Context, err error, w http.ResponseWriter) {
	status := server.HTTPStatusForError(err) // 复用框架映射
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status) // 状态码仍然写在头上
	_ = json.NewEncoder(w).Encode(envelope{
		Code: businessCode(err, status),
		Msg:  publicMessage(err, status),
	})
}

svc := kit.MustNewHTTP(":8080", kit.WithJSONServerOptions(
	server.ServerResponseEncoder(encodeEnvelope),
	server.ServerErrorEncoder(encodeEnvelopeError),
))
```

路由级 option 覆盖服务级，因此需要保持裸格式的路由，给 `kit.HandleJSON*` 传
`server.ServerResponseEncoder(server.EncodeJSONResponse)` 即可。

三件必须做对的事：

- **状态码留在头上。** `HTTPStatusForError` 给出的就是内置编码器用的那套映射，
  含 `StatusCoder`、apperror kind 与 context 错误。回 `200 {"code": 40401}` 会
  同时打坏缓存、网关重试、`WithRetry`、5xx 告警与负载均衡健康检查——它们只看状态码。
- **数字码从分类推导，不要从状态码推导。** 用一张表把
  `transporthttp.ErrorCoder`（`apperror` 的稳定码）映射到你的数字空间；service 层的
  `apperror` 不动。
- **5xx 必须脱敏。** `status >= 500` 时不要回显 `err.Error()`，按 `JSONErrorEncoder`
  的做法给固定文案。

一个取舍：`client.HTTPStatusError.ErrorCode()` 读的是消息体里字符串型的 `code`，
因此数字信封会让服务间自动透传业务码失效。基于状态码的分类、重试与 `Retry-After`
处理都照常工作，只有业务码需要你自己做一次转译。

流式路由不适用：信封无法包裹 SSE 流，`kit.HandleSSETyped` 与原生的
`HTTP.HandleSSE` 方法都自行写帧，并忽略 `ServerResponseEncoder`。


