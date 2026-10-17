# Zap 适配器

[English](README.md) | 简体中文

`integrations/zap` 提供 Zap 专用的 endpoint 日志中间件。核心 `endpoint` 包不会导入 Zap，也不会导入框架的 `log` 包。

```go
import "go.uber.org/zap"

logger, err := zap.NewProduction()
if err != nil {
	return err
}
defer func() { _ = logger.Sync() }()
ep = zapadapter.LoggingMiddleware(logger, "CreateUser")(ep)

handler := server.NewServer(
	ep,
	decodeRequest,
	encodeResponse,
	server.ServerErrorHandler(zapadapter.NewErrorHandler(logger)),
)
```

该中间件记录操作名、耗时、调用结果，以及 context 中携带的请求关联：存在时给出
`trace_id`、`span_id`、`request_id`。字段名与 slog 适配器一致
（`operation`、`duration`、`success`），混用两者的部署可以用同一条日志查询；
失败或 panic 的调用以 Error 级别记录。`NewErrorHandler` 提供显式的传输层错误
日志。传入 nil logger 时会被替换为 no-op logger，从而保证可选装配依然安全。
