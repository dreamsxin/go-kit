# Customization

English | [简体中文](customization_zh.md)

How to customize logging, middleware, and errors without forking the framework.
Use the decision table below to choose the narrowest extension point. Every
recipe keeps classification in the service layer, cross-cutting behavior in
endpoint middleware, and protocol facts in transport.

## Choose The Extension Point

| Need | Use |
| --- | --- |
| change log destination or fields | a `slog.Handler` or the logging adapter |
| observe every endpoint call | `endpoint.Recorder` and `RecordingMiddleware` |
| enforce request policy | `endpoint.Middleware` / `endpoint.Builder` |
| change HTTP parsing or encoding | transport decoder/encoder options |
| change public error shape | `ServerErrorEncoder` and, for JSON success, `ServerResponseEncoder` |
| add a protocol-specific concern | HTTP/gRPC transport hooks or middleware |

Start with [Middleware](middleware.md) for ordering and [Error handling](errors.md)
for status and message rules.

## Customizing logging

### Choosing where logs go

Generated services write structured logs to stdout by design; the framework
has no log-path setting on purpose. For file storage, build your own
`slog.Handler` — anything implementing `io.Writer` works:

```go
file, err := os.OpenFile("service.log",
    os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
if err != nil {
    return err
}
logger := slog.New(slog.NewJSONHandler(file, &slog.HandlerOptions{
    Level: slog.LevelInfo,
}))
```

Write to stdout and a file at once with `io.MultiWriter(os.Stdout, file)`.
Rotation and retention are application decisions: any third-party rotating
writer that implements `io.Writer` plugs into the same handler.

In generated projects the logger is constructed in the user-owned
`cmd/main.go` from `logging.level` / `logging.format`; change it there (or
build the logger in a `config/custom.go` hook) instead of editing generated
files.

### Choosing what gets logged

Business lines come from the slog adapter; level and extra attributes are
options:

```go
logging := slogadapter.LoggingMiddleware(logger, "CreateUser",
    slogadapter.WithLevel(slog.LevelWarn),
    slogadapter.WithAttrs(func(ctx context.Context) []slog.Attr {
        return []slog.Attr{slog.String("tenant", tenantFrom(ctx))}
    }),
)
```

Custom attributes must stay bounded and non-sensitive; request and response
payloads are never logged by the adapters. Protocol lines (method, path,
status, bytes, duration) come from `server.AccessLogMiddleware`.

Errors are only *mapped* by default, not *logged*: the default
`ServerErrorHandler` is a no-op. Install a handler to see transport and
endpoint failures in the log:

```go
kit.NewHTTP(":8080", kit.WithJSONServerOptions(
    server.ServerErrorEncoder(server.JSONErrorEncoder),
    server.ServerErrorHandler(slogadapter.NewErrorHandler(logger)),
))
```

## Writing custom middleware

### Endpoint middleware

Endpoint middleware is a function wrapping an endpoint. The skeleton:

```go
func AuditMiddleware(audit AuditLog) endpoint.Middleware {
    return func(next endpoint.Endpoint) endpoint.Endpoint {
        return func(ctx context.Context, req any) (any, error) {
            // before: read the request, check preconditions, enrich ctx
            resp, err := next(ctx, req)
            // after: observe resp/err, record, decorate
            audit.Record(ctx, req, err)
            return resp, err
        }
    }
}
```

Rules that keep it safe:

- Stay transport-neutral: no `http` or gRPC types; the middleware sees
  decoded requests and business errors only.
- Reject with `apperror` so every transport maps the failure consistently.
- Read correlation IDs from the context
  (`endpoint.RequestIDFromContext(ctx)`, `TraceContextFromContext`) instead
  of inventing new plumbing.
- Decide the pattern consciously: short-circuit without calling `next`
  (validation, bulkhead), or wrap the call (timeout, metrics). See the four
  patterns in [middleware](middleware.md#flow-control).

Security ships a ready-made example of packaged middleware:

```go
security.Middleware(myAuthenticator)   // establishes security.Subject
security.RequireRole("admin")          // enforces at the endpoint layer
```

### Where to install it

| Scope | API |
| --- | --- |
| Every route | `kit.WithEndpointMiddleware(mw...)` |
| One route (typed) | `kit.HandleJSONTypedWithMiddleware(svc, pattern, handler, compose)` |
| One route (existing endpoint) | `endpoint.NewBuilder(ep).Use(mw).Build()` + `kit.HandleJSONEndpoint` |
| One endpoint ad hoc | `endpoint.NewBuilder(ep).Use(mw).Build()` |

The first middleware is the outermost. Middleware applied per route runs
*inside* the component-level chain. Print the assembled chain at startup with
`endpoint.Builder.Describe()` (label custom middleware with `UseNamed`).

### HTTP middleware

Protocol-level behavior (recovery, request dumps, IP policy) uses standard
`http.Handler` middleware:

```go
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            defer func() {
                if p := recover(); p != nil {
                    logger.Error("panic", "err", p, "path", r.URL.Path)
                    http.Error(w, "internal error", http.StatusInternalServerError)
                }
            }()
            next.ServeHTTP(w, r)
        })
    }
}

svc, err := kit.NewHTTP(":8080",
    kit.WithHTTPMiddleware(Recover(logger)),
)
```

Boundary rule: HTTP middleware sees method, path, headers, and status — it
never interprets business results. Behavior that needs the decoded request
belongs in endpoint middleware. Compose browser policies with
`security/http.Chain`.

## Customizing errors

Error customization lives in [error handling](errors.md); this is the
decision guide:

| Need | Recipe | Where |
| --- | --- | --- |
| Classify a business failure | `apperror.New/Wrap(kind, code, message)` | service layer |
| Validate request fields | `endpoint.Validatable` + `WithValidation()` | request type + builder |
| Custom kind with a custom status | `server.JSONErrorEncoderWithKindMapper` (HTTP) / `grpcserver.ErrorEncoderWithKindMapper` (gRPC) | assembly |
| Custom wire format / envelope | `server.ServerErrorEncoder` + `server.ServerResponseEncoder` | assembly |
| Same envelope on every JSON route | `kit.WithJSONServerOptions` | assembly |
| Compose with the built-in mapping | `server.HTTPStatusForError` / `HTTPStatusForErrorKind` | custom encoders |

Rules of thumb: classify with `apperror` in the service layer; never return
protocol types from business code; 4xx carries a public message and 500 never
does; retry decisions belong to the caller and only classified, idempotent
failures should be retried.

### Wrapping responses in a `{code, msg, data}` envelope

Some house styles require a fixed envelope with a numeric code. Two hooks cover
it — `server.ServerResponseEncoder` for the success path and
`server.ServerErrorEncoder` for the error path — and
`kit.WithJSONServerOptions` installs both on every JSON route:

```go
type envelope struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data,omitempty"`
}

func encodeEnvelope(_ context.Context, w http.ResponseWriter, response any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(envelope{Code: 0, Msg: "ok", Data: response})
}

func encodeEnvelopeError(ctx context.Context, err error, w http.ResponseWriter) {
	status := server.HTTPStatusForError(err) // reuse the framework mapping
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status) // the status still belongs in the header
	_ = json.NewEncoder(w).Encode(envelope{
		Code: businessCode(err, status),
		Msg:  publicMessage(err, status),
	})
}

svc := kit.MustNewHTTP(":8080", kit.WithJSONServerOptions(
	server.ServerResponseEncoder(encodeEnvelope),
	server.ServerErrorEncoder(encodeEnvelopeError),
))
```

A per-route option overrides the service-wide one, so a route that must stay
unwrapped passes `server.ServerResponseEncoder(server.EncodeJSONResponse)` to
`kit.HandleJSON*`.

Three things to get right:

- **Keep the status in the header.** `HTTPStatusForError` gives you the same
  mapping the built-in encoders use, including `StatusCoder`, apperror kinds, and
  context errors. Answering `200 {"code": 40401}` breaks caches, gateway retries,
  `WithRetry`, 5xx alerting, and load-balancer health checks, all of which read
  only the status.
- **Derive the numeric code from the classification, not the status.** Map
  `transporthttp.ErrorCoder` (`apperror`'s stable code) to your numeric space in
  one table; keep `apperror` in the service layer untouched.
- **Redact 5xx.** `status >= 500` must not echo `err.Error()`; use a fixed
  message the way `JSONErrorEncoder` does.

One trade-off: `client.HTTPStatusError.ErrorCode()` reads a string `code` from
the body, so a numeric envelope loses the automatic code relay between your own
services. The status-based classification, retry, and `Retry-After` handling all
keep working; only the code needs a translation of your own.

Streaming routes are exempt: an envelope cannot wrap an SSE stream, so both
`kit.HandleSSETyped` and the raw `HTTP.HandleSSE` method write their own frames
and ignore `ServerResponseEncoder`.


