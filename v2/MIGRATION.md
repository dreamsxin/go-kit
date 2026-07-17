# Migrating From v1 To v2

v2 is a new Go major-version module and does not preserve v1 source
compatibility. Migrate one service at a time and review generated output instead
of mechanically replacing every import in a repository.

## Module Path

Change framework imports from:

```go
github.com/dreamsxin/go-kit/...
```

to:

```go
github.com/dreamsxin/go-kit/v2/...
```

Then run:

```bash
go mod tidy
go test ./...
```

Do not add a local `replace` for v2 unless you are intentionally developing the
framework and application together.

## `kit` Construction

v1:

```go
svc := kit.New(":8080", options...)
```

v2:

```go
svc, err := kit.New(":8080", options...)
if err != nil {
	return err
}
```

Options validate during construction and return errors instead of panicking.
`kit.MustNew` remains available for tests and small examples.

## `kit` Lifecycle

v1 owned process signals inside the framework:

```go
svc.Run()
```

v2 requires a caller-owned context and returns startup, serve, and shutdown
errors:

```go
ctx, stop := signal.NotifyContext(
	context.Background(),
	os.Interrupt,
	syscall.SIGTERM,
)
defer stop()

if err := svc.Run(ctx); err != nil {
	return err
}
```

Use `kit.WithShutdownTimeout` to configure the graceful-shutdown deadline.
Service instances cannot be restarted after shutdown.

## gRPC Registration

v1:

```go
grpcServer := svc.GRPCServer()
```

v2:

```go
grpcServer, err := svc.GRPCServer()
if err != nil {
	return err
}
```

`WithGRPC` must be configured before requesting the server.

## HTTP Route Registration

Business JSON routes should use:

```go
kit.HandleJSON[Request](svc, "/route", handler)
```

or:

```go
kit.HandleJSONEndpoint[Request](svc, "/route", ep)
```

`Service.Handle` and `Service.HandleFunc` are raw HTTP escape hatches and do not
apply endpoint middleware. Code that previously expected endpoint metrics,
logging, timeout, rate limit, or circuit breaking around a raw handler must move
to an endpoint-backed registration path.

Generated HTTP routes now use Go's `http.ServeMux` method patterns. Custom route
hooks receive `*http.ServeMux`, and GET requests use `form` tags (falling back to
`json` tags and field names) for both client encoding and server decoding.

`NewJSONClientWithRetry` was removed because it never retried. Use
`NewJSONClientWithTimeout` for an HTTP client deadline and apply an explicit,
operation-aware retry policy outside the transport when the call is safe to
repeat.

## Service Discovery

The v2 `Instancer` registration contract returns the initial snapshot
synchronously:

```go
Register(chan events.Event) events.Event
Deregister(chan events.Event)
```

Custom instancers must return an immutable current event and publish later
updates through the registered buffered channel. Do not close subscriber channels
from the producer.

Retry counts are now expressed as total attempts: replace `WithMaxRetries(n)`
with `WithMaxAttempts(n)`. The default is one attempt, and unknown errors are not
retryable. Mark transient application errors with `Retryable() bool` when retrying
them is safe.

`sd.NewEndpoint` and `sd.NewEndpointWithDefaults` now return
`(endpoint.Endpoint, io.Closer, error)`. The former `NewEndpointCloser` duplicate
was removed. Handle validation errors and close the returned resource before
stopping the Instancer:

```go
call, closer, err := sd.NewEndpoint(instancer, factory, logger)
if err != nil {
    return err
}
defer instancer.Stop()
defer closer.Close()
```

## Circuit Breakers

v2 keeps one endpoint circuit-breaker adapter: `circuitbreaker.Gobreaker`.
HandyBreaker and the built-in Hystrix implementation were removed to avoid three
overlapping state machines with different timeout and error semantics.

## Generated Configuration

v2 generated config uses this precedence:

```text
defaults -> local YAML -> optional remote config -> final environment overrides -> validation
```

`Config.Validate()` runs before runtime wiring. Existing deployments should
verify address, timeout, logging, database, middleware, and remote-provider
values instead of relying on zero values.

Database `AutoMigrate` is disabled by default. Enable it explicitly only when
startup schema mutation is intended.

## Generated API Documentation

v2 generates OpenAPI 3.1 and a standalone JSON Schema 2020-12 bundle directly
from the normalized `microgen` IR. Generated HTTP transport files no longer
carry swag annotations, and `swagger_host` / `APP_SWAGGER_HOST` have been
removed. Replace integrations that read `/swagger/doc.json` with
`/openapi.json`; reusable message schemas are available at `/schema.json`.
Embedded Swagger UI 5 remains available at `/swagger/` when generation uses
`-openapi`. The v1 `-swag` flag is not retained.

`docs/docs.go`, `docs/openapi.json`, and `docs/schema.json` are generator-owned
and refreshed on generation or extend. Keep custom descriptions in the source
contract rather than editing generated documentation.

## Generated Projects

Regenerate into a new directory and compare ownership boundaries before replacing
an existing v1 project:

```bash
microgen -idl idl.go -out ./v2-preview -import example.com/service
cd v2-preview
go mod tidy
go test ./...
```

Do not overwrite user-owned service implementations with generated files. For a
v2-generated project, use `microgen extend -check -out .` before extend mode.

## Interaction And MCP

Re-test MCP clients against the generated v2 endpoint. Configure HTTP write
timeouts for long-lived SSE responses, keep request-body limits enabled, and
validate the negotiated protocol version.

In-memory resource and prompt providers now copy mutable inputs and no longer
invoke prompt render callbacks while holding provider locks. Code that depended
on mutating registered slices or maps must update through provider APIs instead.

## Migration Checklist

- Update module imports to `/v2`.
- Handle `kit.New`, `Run`, and `GRPCServer` errors.
- Move signal handling to `main`.
- Register business routes through endpoint-backed APIs.
- Update custom service-discovery implementations.
- Review generated config precedence and validation.
- Confirm `AutoMigrate` policy.
- Run package tests and race tests.
- Exercise shutdown and long-lived MCP responses.
