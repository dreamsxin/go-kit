# Zap adapter
English | [简体中文](README_zh.md)

`integrations/zap` owns the Zap-specific endpoint logging middleware. The core
`endpoint` package does not import Zap or the framework `log` package.

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

The middleware records the operation name, elapsed duration, call outcome,
and the request correlation the context carries: `trace_id`, `span_id` and
`request_id` when present. The field names match the slog adapter
(`operation`, `duration`, `success`), so a deployment mixing the two can run
one log query; a failed or panicking call is logged at Error.
`NewErrorHandler` provides explicit transport error logging. A nil logger is
replaced by a no-op logger so optional assembly remains safe.
