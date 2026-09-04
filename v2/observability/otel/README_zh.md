# OpenTelemetry 适配器

[English](README.md) | 简体中文

`observability/otel` 是 `github.com/dreamsxin/go-kit/v2` 模块中的可选 package。
不导入它的服务不会在自己的 package 依赖闭包中解析 OpenTelemetry 依赖。

由应用负责 tracer/meter provider 的设置、导出器、采样和关闭。适配器只根据传入的 provider 创建 span 与 instruments：

```go
tracer := otel.Tracer("my-service")
endpointFn = oteladapter.TracingMiddleware(tracer, "GetUser")(endpointFn)

metrics, err := oteladapter.NewMetrics(otel.Meter("my-service"),
    oteladapter.WithMetricAttributes(attribute.String("component", "users")),
)
endpointFn = metrics.Middleware("GetUser")(endpointFn)
```

适配器只添加有数量上限的 application、operation 和 outcome 属性。诸如 `service.name` 之类的资源属性属于 provider 设置的范畴。请求与响应载荷永远不会被记录。
