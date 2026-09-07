# 可观测性

[English](observability.md) | 简体中文

go-kit 把业务结果指标与协议访问指标分开。先按需要选择信号，维度保持有界，默认
不要记录请求或响应 body。

## 按信号选择

| 需求 | 使用 | 能看到什么 |
| --- | --- | --- |
| 业务成功、错误、耗时 | `endpoint.Recorder` / `RecordingMiddleware` | 解码后的端点调用 |
| 进程内计数和平均耗时 | `endpoint.Metrics` | 本进程总量 |
| 结构化业务日志 | `observability/slog` 或 `integrations/zap` | operation、结果、关联 ID |
| 导出 trace 和指标 | `observability/otel` | span、计数器、直方图 |
| HTTP 状态、字节和路径 | `server.AccessLogMiddleware` | 协议事实 |

## 标准库路径

`slogadapter.NewTelemetry` 用一个稳定的 operation 名称组装 trace、指标和日志：

```go
telemetry, err := slogadapter.NewTelemetry(slogadapter.TelemetryConfig{
    Operation: "users.create",
    Logger:    logger,
})
if err != nil {
    return err
}

ep := telemetry.Apply(endpoint.NewBuilder(createUser)).Build()
```

`Signals` 可以逐个选择维度——比如指标来自 OpenTelemetry meter 的服务用
`SignalTracing | SignalLogging`，这样没有一次调用被记录两遍。每个装配好的中间件
自带名称，因此少一个维度、或者往 `telemetry.Middlewares` 追加自己的适配器时，
`Builder.Describe` 依然正确。

如果服务需要内部快照或健康计数，保留 `telemetry.Metrics`。需要 HTTP 状态和响应
字节时，通过 `kit.WithHTTPMiddleware` 安装 `server.AccessLogMiddleware(logger)`。

## 被 scrape

推送不是唯一的模型。`observability/metrics` 把 `endpoint.Metrics` 已经持有的数字渲染成
Prometheus 文本 exposition，而模块里没有任何指标客户端：

```go
collector := &endpoint.Metrics{}

var routes httpserver.RouteRegistrar = mux
routes = httpserver.DecorateRoutes(mux,
    httpserver.RecordingMiddleware(metrics.HTTPRecorder(collector)))
routes.Handle("GET /users/{id}", usersHandler)

mux.Handle("GET /metrics", metrics.Handler(collector))
```

两个细节承载了整个设计。recording 安装在**注册处**，通过 `httpserver.RouteRegistrar`——
`http.Request.Pattern` 只在 mux 分派出去的那个请求上有值，所以包在 mux 外面的中间件会把
每个请求都记到空路由下。而 exposition 挂在 mux 本身、在装饰器之外，于是 scrape 报告的是
服务，而不是它自己。

gRPC 这边 recorder 是拦截器，桥接放在独立的 package 里——为的是不让一个只想要 scrape 端点的
纯 HTTP 服务被迫依赖 gRPC 库：

```go
component := kitgrpc.MustNew(":8081",
    grpc.ChainUnaryInterceptor(grpcserver.RecordingUnaryInterceptor(metricsgrpc.Recorder(collector))),
    grpc.ChainStreamInterceptor(grpcserver.RecordingStreamInterceptor(metricsgrpc.Recorder(collector))),
)
```

operation 标签就是完整方法名 `/users.Users/Get`。它不像 HTTP 路由那样需要被"防守"：方法集合
由服务定义固定，而调用一个不存在的方法在拦截器运行之前就被拒绝了。流在结束时被记录，并带
`Stream: true`——它的 duration 是一段生命周期而不是一次延迟，不要和后者一起平均。

exposition 给什么、不给什么：每个 operation 的次数、错误数与耗时总和——是均值，不是分位数。
没有 bucket，因为收集器持有的是总量；p99 来自下面的 OpenTelemetry 直方图。计数器在进程启动
时从零开始，采集方会把这识别为一次 reset。

生成服务用 `server.metrics_path`（`APP_METRICS_PATH`），默认为空即关闭：该端点会公开你的路由名
与流量形状，应该放在 admin 监听或网络策略之后。

## OpenTelemetry

`oteladapter.Setup` 装配 provider、OTLP exporter、resource、全局 W3C propagator
以及唯一的 `Shutdown`：

```go
providers, err := oteladapter.Setup(ctx, oteladapter.Config{
    ServiceName: "users",
    Endpoint:    "collector:4317",
    Insecure:    true,
})
if err != nil {
    return err
}
defer providers.Shutdown(context.Background())

metrics, err := oteladapter.NewMetrics(providers.Meter())
if err != nil {
    return err
}

ep := endpoint.NewBuilder(createUser).
    Use(oteladapter.TracingMiddleware(providers.Tracer(), "users.create")).
    Use(endpoint.RecordingMiddleware("users.create", metrics)).
    Build()
```

`oteladapter.Metrics` 实现了 `endpoint.Recorder`，也可以传给 `kit.WithRecorder`，
覆盖所有 JSON endpoint。instrument 遵循语义约定：耗时以秒计，失败的调用携带
`error.type`，而不是再喂一个独立的错误计数器。

要从指标看响应状态，用 `oteladapter.NewHTTPMetrics` 记录
`http.server.request.duration`，携带 `http.route` 与 `http.response.status_code`。
用 `kit.WithHTTPRecorder(httpMetrics)` 安装——它把 recorder 接在每条路由上，那是
匹配到的 pattern 唯一存在的地方。

应用自己管理 provider 配置时可以跳过 `Setup`：传入任意 `trace.Tracer` 与
`metric.Meter`，关闭仍归你。

## 关联与基数

- 启用 `kit.WithRequestID()`，响应会带 `X-Request-ID`，端点日志使用同一个 ID。
  `AccessLogMiddleware` 会从 request context 或响应 header 读取它。
- W3C `traceparent` 在传输边界无需接线就会被提取：`kit.NewHTTP` 与 `kit/grpc.New`
  都会做，`integrations/grpc/client` 在出方向注入。端点 middleware 随后记录当前
  trace ID。手工装配的服务端与客户端可用 `transporthttp.ExtractTraceparent` /
  `InjectTraceparent` 和 `transportgrpc.ExtractTraceparent` / `InjectTraceparent`。
- `interaction.Runtime.WithLogger(logger)` 用同一个 logger 与同样的 `trace_id` /
  `request_id` 上报工具调用，于是一次 MCP 工具调用能和承载它的请求对上。
- 指标 label 使用路由 pattern 或固定 operation 名称。不要用用户 ID、任意 URL、
  原始错误文本或请求 body 作为维度。
- 需要记录传输错误时安装 `ServerErrorHandler`。默认 handler 是 no-op；错误编码器
  只负责决定线上响应。

## 所有权与停机

日志 handler、OpenTelemetry provider、exporter 及其关闭由应用拥有。框架适配器只拥有
自己创建的 instrument 或 middleware。服务停止后再 flush/关闭 exporter，并确保停机
期限长于 exporter flush 超时。

告警阈值和部署检查见[生产指南](../PRODUCTION_zh.md)，自定义维度或新后端见[自定义](customization_zh.md)。
