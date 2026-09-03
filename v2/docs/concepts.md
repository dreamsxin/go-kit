# Core Concepts

English | [简体中文](concepts_zh.md)

Three invariants define go-kit v2. Everything else builds on them.

## The Short Version

| Question | Answer |
| --- | --- |
| Where does business logic live? | `service` functions, independent of protocols. |
| Where do timeout, retry, and admission policies live? | Endpoint middleware or the discovery execution layer. |
| Who owns listeners, clients, and goroutines? | The component or constructor that created them; close consumers before providers. |
| How do errors cross protocols? | `apperror.Kind` is mapped to HTTP or gRPC status at the transport boundary. |

Read [Middleware](middleware.md) for request policy, [Error handling](errors.md)
for public failures, and [Lifecycle](lifecycle.md) for startup and shutdown.

## The request path

```text
Transport request
    -> decode
    -> endpoint middleware
    -> endpoint
    -> service method
    -> encode
    -> transport response
```

Each layer owns one kind of decision and must not own the others':

| Layer | Owns | Must not own |
| --- | --- | --- |
| Service | Business rules and domain orchestration | HTTP/gRPC types and status mapping |
| Endpoint | Transport-neutral request boundary and middleware | Socket/server lifecycle |
| Transport | Protocol decode, encode, headers, and status | Business rules and retry policy |
| Assembly | Dependency wiring and process lifecycle | Hidden global state |

The practical consequence: business logic is a plain function of
`(context, Request) -> (Response, error)`; it can be unit-tested without a
network and is reusable across HTTP, gRPC, and MCP transports.

## Error classification

Libraries return errors; the transport layer maps them to protocol statuses.
Business errors are classified with `apperror`:

```go
return Todo{}, apperror.New(apperror.KindNotFound, "todo.not_found", "todo not found")
```

A kind is the only thing business code says about a failure; each transport
turns it into a protocol status — `KindNotFound` becomes HTTP 404 and gRPC
`NotFound`, `KindDeadlineExceeded` becomes 504 and `DeadlineExceeded`, and so
on. The complete kind table for both transports lives in
[error handling](errors.md#the-apperror-reference).

> [!NOTE]
> An error the framework cannot classify at all encodes as 500 with a redacted
> message — the internal error string never reaches the client. "Classified" is
> wider than "built with `apperror`", though: the encoders also honor
> `transporthttp.StatusCoder` and `PublicMessager`, which is how
> `endpoint.ValidationError` becomes 400, the admission-control sentinels become
> 503/429, and a relayed `client.HTTPStatusError` keeps its upstream status.
> Bare `context.DeadlineExceeded` and `context.Canceled` also map to 504 and 499
> rather than 500. `server.HTTPStatusForError` is the one function that answers
> "what status will this error get".

The full flow, including custom error formats, is covered in
[error handling](errors.md).

## Lifecycle ownership

The process entry point owns signals and the root context. Framework services
own listeners and graceful shutdown after startup succeeds:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
if err := host.Run(ctx); err != nil {
    return err
}
```

Startup errors surface synchronously; a host cannot be restarted after
shutdown. Optional components (background jobs, a gRPC listener, the HTTP
component) attach through `kit.Lifecycle` and share the same bounded shutdown.
See [lifecycle](lifecycle.md).
