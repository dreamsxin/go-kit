# 路由

[English](routing.md) | 简体中文

如何定义一个服务的路由：模式语法、该选哪个注册调用、路径与查询参数、影响每条
路由的选项，以及 gRPC 与生成项目对同一问题的回答。

一个 `kit.HTTP` 组件的所有 HTTP 路由都挂在同一个 `*http.ServeMux` 上，模式语法
就是 Go 1.22 的 ServeMux 语法——本章讲的是框架在其上增加的约定，而不是另一个
路由器。

## 模式语法

路由模式是 Go 的 ServeMux 模式。框架约定为 `"METHOD /path"`：

```go
kit.HandleJSONTyped(svc, "POST /todos", createTodo)
kit.HandleJSONTyped(svc, "GET /todos/{id}", getTodo)
```

- 方法前缀可省略。`"POST /todos"` 响应 POST，其他方法得到 `405` 和一个
  `Allow` header——这是 mux 替你做的，无需编写任何代码。不带方法的模式
  （`"/api/me"`）响应所有方法。
- `{id}` 声明路径参数，handler 用 `r.PathValue("id")` 读取。`{name...}` 通配符
  和以斜杠结尾的模式（`"/swagger/"`）匹配子树，遵循标准 ServeMux 规则。
- 两个模式同时匹配时，**最具体的获胜**——对同一路径，`POST /todos/done`
  胜过 `POST /todos/{id}`，与注册顺序无关。
- 注册两个相同的模式会在启动时 panic。这是有意的：路由冲突是编程错误，应该在
  第一个请求之前遇上，而不是之后。

`kit.NewHTTP` 注册的探针路由——`/health`、`/livez`、`/readyz`——是方法前缀约定
的唯一例外：它们不带方法挂载，因此响应所有方法。生成项目把注册写成
`"GET /health"`，mux 会把它同时服务给 GET 和 HEAD。

## 选择注册调用

每个注册都挂在同一个 mux 上，得到相同的 HTTP context：在途计数、放入 context
的请求与写入器、`kit.Stopping`、traceparent 提取、请求 ID 处理、`WithTimeout`
截止时间，以及任何 `WithHTTPRecorder` 记录器。这些调用只在两个轴上不同——
端点中间件链与端点记录器是否生效，以及采用哪个请求体大小限制。

| 调用 | 使用时机 |
| --- | --- |
| `kit.HandleJSONTyped` | 默认。类型化的请求与响应，完整链；请求结构体从请求本身填充（见下文）。 |
| `kit.HandleJSON` | 相同，但响应类型是动态的。 |
| `kit.HandleJSONTypedWithMiddleware` / `HandleJSONWithMiddleware` | 单条路由需要额外中间件；它落在组件链的**内侧**。 |
| `kit.HandleJSONEndpoint` | 你已经持有 `endpoint.Endpoint`。 |
| `kit.HandleJSONTypedWithBodyLimit` / `HandleJSONEndpointWithBodyLimit` | 单条路由接受不同的最大请求体；零或负数会 panic。保留完整链——不像直接改用 `Handle`。 |
| `kit.HandleSSETyped` | Server-Sent Events 流。见 [SSE 路由](#sse-路由)。 |
| `svc.HandleFunc` / `svc.Handle` | 逃生舱：静态文件、第三方 handler、kit 不建模的协议。 |

逃生舱的代价值得写明。原生 handler 保留 HTTP context——请求 ID、截止时间、记录
器——但**没有端点中间件、没有端点记录器**：`WithEndpointMiddleware`、
`WithMetrics`、`WithRecorder`、限流与熔断都看不到它。路由需要这些时，请注册为
JSON。

`kit.NewJSONHandler` / `NewJSONTypedHandler` 不注册任何路由；它们返回一个由你
自己挂载的 handler。（`kit.JSON` / `JSONTyped` 是已弃用的拼写，`HandleSSE` 也是
——一个承诺 SSE 行为却悄悄跳过链的名字，正是流意外丢失中间件的方式。）

## 路径与查询参数

类型化路由从请求本身填充请求结构体，因此路径与查询参数是类型化路由的一部分——
而不是退回原生 handler 的理由：

```go
type getTodoRequest struct {
	ID      int64  `form:"id"       json:"id"`
	Keyword string `form:"keyword"  json:"keyword"`
}

kit.HandleJSONTyped(svc, "GET /todos/{id}", func(ctx context.Context, req getTodoRequest) (Todo, error) {
	return todos.Get(ctx, req.ID) // req.ID 来自路径
})
```

- GET 与 HEAD 从查询串**和**路径值填充结构体。
- 其他方法读严格解码的 JSON 体再加路径值——一个结构体可以同时携带 body 字段与
  `{id}`。
- 完全没有请求体的请求解为零值，这让不带体的 `POST /todos/{id}/done` 也能保持
  类型化；handler 的校验用应用自己的话回答"什么都没发"。
- 字段按 `form` tag 命名，其次 `json` tag，其次小写字段名；同名时路径值优先于查询
  参数——与生成传输的解码规则完全一致。

应答不是 JSON（204、文件）或需要手工解码时，原生 handler 仍是答案：
`transporthttp.DecodePathRequest` 与 `DecodeQueryRequest` 按需从原生请求填充结构
体，`server.DecodeJSONBody` 按传输规则裁决请求体。客户端侧也有对应的 URL 构造
helper：`transporthttp.EncodePath` 与 `EncodePathAndQuery` 在 `{占位符}` 未解析
时宁可报错也不发送。

## 路由分组

分组让前缀只声明一次，而不是在每条路由上重复：

```go
api := svc.Group("/api/v1")
kit.HandleJSONTyped(api, "POST /todos", createTodo)
kit.HandleJSONTyped(api, "GET /todos/{id}", getTodo)
v2 := svc.Group("/api/v2") // 分组可嵌套：两个前缀都会拼接
```

分组只拼接模式：路由照常经过组件的 HTTP context、端点中间件（以拼接后的模式标
注）、记录器与 JSON 服务器选项，与把模式写全没有分别。它不是子 mux、不接收选
项、不携带自己的中间件——中间件属于组件或路由。`svc.Group("")` 即组件自身视图；
`Handle`/`HandleFunc` 可经分组用于原生逃生舱。

## JSON 请求体之外

- **分页**：`transporthttp.ParsePage(r)` 读取分页约定，
  `transporthttp.NewPageResult(items, page, total)` 写出结果信封。见 transport
  README 的分页一节。
- **Multipart**：`server.ParseMultipartForm(r, limits)` 配合
  `server.MultipartLimits`；超限与错误媒体类型的失败和其他错误一样带着分类到达
  （413/415）。
- **文件下载**：`server.WriteAttachment(w, name, body)` 设置 disposition 头。
- **请求 context**：`transporthttp.PopulateRequestContext` 把方法、URI、header
  字段复制进 context，供不便直接触碰请求的 handler 使用。请求 ID 默认不信任调用
  方：入站 `X-Request-ID` 超过 `transporthttp.MaxRequestIDLength` 或未通过
  `kit.WithRequestIDValidator` 时会被替换为新生成的。

## 影响每条路由的选项

| 选项 | 对路由的影响 |
| --- | --- |
| `WithHTTPMiddleware(mw...)` | 包裹**每条**路由，包括探针；第一个参数在最外层。 |
| `WithEndpointMiddleware(mw...)` | 包裹每个 JSON/SSE 端点；每次包裹都以路由模式标注，因此指标与日志能说出路由名。 |
| `WithJSONServerOptions(opts...)` | 组件级响应信封与错误编码器。每路由选项在其后运行并获胜。 |
| `WithJSONMaxBodyBytes(n)` | 组件级严格 JSON 请求体限制；`<= 0` 关闭大小检查，严格性保留。单条路由不同：用 body-limit 注册。 |
| `WithTimeout(d)` | **每条**路由（含原生 handler）的请求 context 截止时间。忽略取消的 handler 仍会跑完。无单路由豁免——见 SSE。 |
| `WithRequestID()` / `WithRequestIDValidator(v)` | 提取并回显 `X-Request-ID`；validator 覆盖信任策略。 |
| `WithProbePaths(p)` / `WithoutProbes()` | 重命名或去掉 `/health`、`/livez`、`/readyz`；空路径省略该路由。 |
| `WithHTTPRecorder(recs...)` | HTTP 层观测（方法、路由、状态）。覆盖应用路由，从不覆盖探针。 |
| `WithRecorder(recs...)` / `WithMetrics(m)` | 端点层结果，以路由模式标注。 |

HTTP 记录器为何按路由安装而不是包在 mux 外面：`Observation.Route` 来自
`http.Request.Pattern`，而包在 mux 外的包装器永远看不到它。未匹配的 URL 上报空
`Route`，这正是未路由路径不会变成无界指标维度的原因。

## SSE 路由

```go
kit.HandleSSETyped(svc, "GET /events/{tenant}", func(
	ctx context.Context, req eventsRequest, w *httpserver.SSEStream,
) error {
	for {
		select {
		case <-kit.Stopping(ctx):
			return nil
		case evt := <-events:
			if err := w.EventJSON("todo", evt); err != nil {
				return err
			}
		}
	}
}, decodeEventsRequest)
```

解码器拿到 `*http.Request`，流可以自己读取路径与查询参数。流运行在端点链内：`WithTimeout` 约束**整条流**，且没有单路由
豁免，所以长生命周期流应当挂在自己的第二个 `kit.NewHTTP` 组件上。流一旦应答过
200，错误就只能被记录而无法渲染。停机期间 handler 观察 `kit.Stopping(ctx)` 并结
束自己的响应。header 语义与 hook 细节在 [transport README](../transport/README.md)，
停机交互在[生命周期](lifecycle_zh.md)。

## gRPC 路由

gRPC 服务按 protobuf 的本意注册到 `*grpc.Server` 上：

```go
comp, err := kitgrpc.New(":8081") // kit/grpc 组件
if err != nil {
	log.Fatal(err)
}
idl.RegisterTodoServiceServer(comp.Server(), todoServer)
```

`kit/grpc` 组件预装了 trace-context 拦截器，并注册 gRPC 健康服务（`Check` 与
`Watch`），由与 HTTP 组件相同的探针注册表支撑。handler 侧是 `integrations/grpc`
的 `grpcserver.NewServer(endpoint, decode, encode)`，选项形状
（`ServerBefore`/`ServerAfter`/错误编码器）与 HTTP 相同；把完整方法名放进
context 的是 `grpcserver.Interceptor`。一个 handler、两种协议：`transport.Binding`
把一个类型化函数同时承载为 `TypedEndpoint()`（给 `HandleJSONTyped`）和
`GRPCServer()`（给生成的注册调用）。按调用指标使用以完整方法名标注的记录拦截
器。

## 生成项目中的路由

`microgen` 通过 `transport/http/server.RouteRegistrar` 为每个一元方法注册一条
路由——`*http.ServeMux` 天然满足它，而注册点是 handler 与其模式唯一同时可见的
地方，这正是按路由中间件安装在那里的原因。`-prefix` 标志设置路由前缀；带
`-prefix /api` 时 `userservice` 的路由变成 `/api/userservice/...`。

你的路由放进只写一次的钩子 `cmd/custom_routes.go`：

```go
func registerCustomRoutes(r *http.ServeMux) {
	// 注册直接写入 mux，仅标准库
}
```

要让它们出现在 `/debug/routes` 与启动清单里，在 `customRouteDescriptions()` 中
以 `"METHOD /path"` 字符串描述。

设置 `debug.routes_enabled` 后，`GET /debug/routes` 以 JSON 数组应答
`{"Method","Path","Handler"}` 条目——health、debug、生成、自定义、OpenAPI 与
MCP 路由都在其中，并标注每个的角色。设置 `debug.print_routes` 后，同一清单在启
动时打印。按路由指标的接线方式是包装注册器：
`httpserver.DecorateRoutes(registrar, httpserver.RecordingMiddleware(...))`。

## 健康路由与三种"健康"

`kit.NewHTTP` 从探针注册表挂载 `/health`（全部）、`/livez`（存活）与
`/readyz`（就绪）；`WithLivenessCheck` 与 `WithReadinessCheck` 填充它，`Probes()`
把注册表交还给你做自定义挂载。回答"是否健康"的有三种不同的东西：

- `health` 包——**本进程**的注册表与探针路由（挂到你自己的 mux 或第二个端口的
  方法见它的 [README](../health/README.md)）；
- kit 的探针选项——把该注册表装配到 HTTP 组件上；
- `sd/health`——**远端实例**是否通过主动探测。两者互不回答对方的问题。

## 接下来去哪

- [核心概念](concepts_zh.md)：路由在请求路径中的位置
- [中间件](middleware_zh.md)：JSON 路由加入的那条链
- [错误处理](errors_zh.md)：返回的错误如何变成状态码
- [transport README](../transport/README.md)：编解码器、SSE hook、分页、
  multipart 全文
