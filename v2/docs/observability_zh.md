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

如果服务需要内部快照或健康计数，保留 `telemetry.Metrics`。需要 HTTP 状态和响应
字节时，通过 `kit.WithHTTPMiddleware` 安装 `server.AccessLogMiddleware(logger)`。

## OpenTelemetry

可选的 `observability/otel` 模块使用应用提供的 provider：

```go
metrics, err := oteladapter.NewMetrics(otel.Meter("users"))
if err != nil {
    return err
}

ep := endpoint.NewBuilder(createUser).
    Use(oteladapter.TracingMiddleware(otel.Tracer("users"), "users.create")).
    Use(endpoint.RecordingMiddleware("users.create", metrics)).
    Build()
```

`oteladapter.Metrics` 实现了 `endpoint.Recorder`，也可以传给 `kit.WithRecorder`，
覆盖所有 JSON endpoint。provider、exporter、采样、资源属性和关闭都由应用拥有。

## 关联与基数

- 启用 `kit.WithRequestID()`，响应会带 `X-Request-ID`，端点日志使用同一个 ID。
  `AccessLogMiddleware` 会从 request context 或响应 header 读取它。
- 在传输边界传播 W3C `traceparent`；端点 middleware 记录当前 trace ID。
- 指标 label 使用路由 pattern 或固定 operation 名称。不要用用户 ID、任意 URL、
  原始错误文本或请求 body 作为维度。
- 需要记录传输错误时安装 `ServerErrorHandler`。默认 handler 是 no-op；错误编码器
  只负责决定线上响应。

## 所有权与停机

日志 handler、OpenTelemetry provider、exporter 及其关闭由应用拥有。框架适配器只拥有
自己创建的 instrument 或 middleware。服务停止后再 flush/关闭 exporter，并确保停机
期限长于 exporter flush 超时。

告警阈值和部署检查见[生产指南](../PRODUCTION_zh.md)，自定义维度或新后端见[自定义](customization_zh.md)。
