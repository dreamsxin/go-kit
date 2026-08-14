# Zap adapter

`observability/zap` owns the Zap-specific endpoint logging middleware. The core
`endpoint` package does not import Zap or the framework `log` package.

```go
logger, err := log.NewProduction()
if err != nil {
	return err
}
ep = zapadapter.LoggingMiddleware(logger, "CreateUser")(ep)
```

The middleware records the operation name, elapsed duration, and call outcome.
A nil logger is replaced by a no-op logger so optional assembly remains safe.
