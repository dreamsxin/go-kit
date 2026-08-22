# Core Concepts

English | [简体中文](concepts_zh.md)

Three invariants define go-kit v2. Everything else builds on them.

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

| Kind | HTTP status | gRPC code |
| --- | --- | --- |
| `KindInvalidArgument` | 400 | InvalidArgument |
| `KindUnauthenticated` | 401 | Unauthenticated |
| `KindPermissionDenied` | 403 | PermissionDenied |
| `KindNotFound` | 404 | NotFound |
| `KindAlreadyExists`, `KindConflict` | 409 | AlreadyExists / Aborted |
| `KindResourceExhausted` | 429 | ResourceExhausted |
| unclassified | 500 | Unknown (opaque to clients) |

> [!NOTE]
> Unclassified errors never leak internal details to clients. Only errors
> classified with `apperror` carry a public message.

The full flow, including custom error formats, is covered in
[error handling](errors.md).

## Lifecycle ownership

The process entry point owns signals and the root context. Framework services
own listeners and graceful shutdown after startup succeeds:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
if err := svc.Run(ctx); err != nil {
    return err
}
```

Startup errors surface synchronously; a service cannot be restarted after
shutdown. Optional components (background jobs, a gRPC listener) attach through
`kit.Lifecycle` and share the same bounded shutdown. See
[lifecycle](lifecycle.md).
