# Zap 适配器

[English](README.md) | 简体中文

`integrations/zap` 提供 Zap 专用的 endpoint 日志中间件。核心 `endpoint` 包不会导入 Zap，也不会导入框架的 `log` 包。

```go
logger, err := log.New("info", "json")
if err != nil {
	return err
}
ep = zapadapter.LoggingMiddleware(logger, "CreateUser")(ep)

handler := server.NewServer(
	ep,
	decodeRequest,
	encodeResponse,
	server.ServerErrorHandler(zapadapter.NewErrorHandler(logger)),
)
```

该中间件记录操作名、耗时和调用结果。`NewErrorHandler` 提供显式的传输层错误日志。传入 nil logger 时会被替换为 no-op logger，从而保证可选装配依然安全。
