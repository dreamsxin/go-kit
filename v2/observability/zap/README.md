# Zap adapter

`observability/zap` owns the Zap-specific endpoint logging middleware. The core
`endpoint` package does not import Zap or the framework `log` package.

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

The middleware records the operation name, elapsed duration, and call outcome.
`NewErrorHandler` provides explicit transport error logging. A nil logger is
replaced by a no-op logger so optional assembly remains safe.
