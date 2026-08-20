# slog 适配器

[English](README.md) | 简体中文

`observability/slog` 是一个可选的 endpoint 日志适配器。它只使用标准库 `log/slog` API，并且独立于 Zap 适配器。

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
endpointFn = slogadapter.LoggingMiddleware(logger, "CreateUser")(endpointFn)
transportErrorHandler := slogadapter.NewErrorHandler(logger)
```

该适配器记录操作名、耗时、是否成功、请求/链路追踪 ID，以及一个可选的、由应用持有且有数量上限的属性集合。它刻意不记录请求或响应载荷。Handler 与日志级别的选择仍由应用装配决定。当需要记录传输层错误时，可以把 `NewErrorHandler` 传给 HTTP 或 gRPC 服务器选项。
