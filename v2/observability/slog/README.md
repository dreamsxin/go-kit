# slog adapter
English | [简体中文](README_zh.md)

`observability/slog` is an optional adapter for endpoint logging. It uses only
the standard-library `log/slog` API and is independent of the Zap adapter.

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
endpointFn = slogadapter.LoggingMiddleware(logger, "CreateUser")(endpointFn)
transportErrorHandler := slogadapter.NewErrorHandler(logger)
```

The adapter records operation, duration, success, request/trace IDs, and an
optional application-owned bounded attribute set. It deliberately does not log
request or response payloads. Handler and level selection remain application
assembly concerns. `NewErrorHandler` can be passed to HTTP or gRPC server
options when transport errors should be logged.
