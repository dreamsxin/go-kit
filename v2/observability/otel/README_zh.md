# OpenTelemetry 适配器

[English](README.md) | 简体中文

`observability/otel` 是 `github.com/dreamsxin/go-kit/v2` 模块中的可选 package。
不导入它的服务不会在自己的 package 依赖闭包中解析 OpenTelemetry 依赖。

## 一次调用装好整条流水线

`Setup` 构建 provider、OTLP exporter 与 resource，把它们连同 W3C trace context
与 baggage propagator 一起装到全局，并返回唯一的 `Shutdown`——它负责冲刷并停止
其中每一样：

```go
providers, err := oteladapter.Setup(ctx, oteladapter.Config{
    ServiceName:    "checkout",
    ServiceVersion: "1.4.0",
    Environment:    "production",
    Endpoint:       "collector:4317",
    Insecure:       true,   // collector 在 localhost 或同一个 pod 内
    SampleRatio:    0.05,   // 只作用于根 span；被采样的父级会带上自己的子级
})
if err != nil {
    return err
}
defer func() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    _ = providers.Shutdown(ctx)
}()
```

- `Protocol` 在 `ProtocolGRPC`（默认，4317 端口）与 `ProtocolHTTP`（4318 端口）
  之间选择。`Endpoint` 为空时交给 `OTEL_EXPORTER_OTLP_ENDPOINT`。
- `Signals` 选择 `SignalTraces`、`SignalMetrics` 或两者。零值装配全部信号。
- `SpanExporter` 与 `MetricReader` 用来替换 OTLP 流水线——比如一个 Prometheus
  reader，或测试里的记录型 exporter。
- 给 `Shutdown` 一个截止时间。它会等最后一批数据导出完成，否则一个不再应答的
  collector 会一直拖住进程退出。

`Setup` 之所以装配全局 propagator：正是它让一条 trace 能跨越一跳——本框架的传输
读写 `traceparent` 头，而任何使用 OpenTelemetry API 的库都通过这个 propagator
与它们互通。

## Endpoint 遥测

中间件与 instrument 都从 provider 上取，因此自己管理 provider 的应用可以完全
不用 `Setup`：

```go
endpointFn = oteladapter.TracingMiddleware(providers.Tracer(), "GetUser")(endpointFn)

metrics, err := oteladapter.NewMetrics(providers.Meter(),
    oteladapter.WithMetricAttributes(attribute.String("component", "users")),
)
endpointFn = metrics.Middleware("GetUser")(endpointFn)
```

`Metrics` 实现 `endpoint.Recorder`，所以 `kit.WithRecorder(metrics)` 能以路由
pattern 作为 operation 覆盖每一条路由。它记录：

- `go_kit.endpoint.requests`——计数器，单位 `{request}`
- `go_kit.endpoint.duration`——直方图，单位秒

两者都带 `operation`，调用失败时再带 `error.type`。错误种类来自错误自身的
`interface{ ErrorKindName() string }`——每个 `apperror` 值都会说出自己的 kind——
因此错误率是同一条时间序列上的比值，而不是第二个计数器。

## HTTP 服务端遥测

`HTTPMetrics` 按 HTTP 语义约定记录 `http.server.request.duration`，携带
`http.request.method`、`url.scheme`、`http.route` 与 `http.response.status_code`。
于是响应状态可以直接从指标告警，中间不必再经过日志链路：

```go
httpMetrics, err := oteladapter.NewHTTPMetrics(providers.Meter())
if err != nil {
    return err
}
component, err := kit.NewHTTP(":8080", kit.WithHTTPRecorder(httpMetrics))
```

`kit.WithHTTPRecorder` 把 recorder 装在每条路由上——匹配到的 pattern 只存在于
那里；包在 mux 外面的中间件会把每个请求都记成空路由。未匹配任何路由的请求不带
`http.route`，也不会退化成原始 URL 路径，这正是这个维度保持有界的原因。

适配器只添加有数量上限的 application、operation 与协议属性。请求与响应载荷永远
不会被记录。
