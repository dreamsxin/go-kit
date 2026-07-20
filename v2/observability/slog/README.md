# slog adapter

`observability/slog` is an optional adapter for endpoint logging. It uses the
standard-library `log/slog` API and does not replace `github.com/dreamsxin/go-kit/v2/log`.

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
endpointFn = slogadapter.LoggingMiddleware(logger, "CreateUser")(endpointFn)
```

The adapter records operation, duration, success, request/trace IDs, and an
optional application-owned bounded attribute set. It deliberately does not log
request or response payloads. Handler and level selection remain application
assembly concerns.
