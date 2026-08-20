# transport
English | [简体中文](README_zh.md)

The `transport` layer adapts external protocols to the framework's endpoint model.

Its responsibility is narrow and intentional:

- decode incoming requests
- call endpoints
- encode outgoing responses
- expose protocol-specific hooks

It should not own business logic.

## Role In The Architecture

Within the framework's three-layer model:

- `service` owns business logic
- `endpoint` owns runtime policy and middleware composition
- `transport` owns protocol adaptation

If a behavior can be expressed as endpoint middleware, it should usually live there instead of in transport code.

## Package Overview

The core transport module exposes the HTTP areas:

- `transport/http/server`
- `transport/http/client`

gRPC is an optional module with two public areas:

- `integrations/grpc/server`
- `integrations/grpc/client`

Common helpers also live under:

- `transport/error_handler.go`
- `transport/http`
- `integrations/grpc`

## Pagination Convention

List endpoints share one pagination contract from `transport/http`:
`ParsePage` reads `?page=` and `?size=` (defaults 1 and 20, size capped at
`MaxPageSize` = 100), returning an `endpoint.ValidationError` for malformed
values that encodes as 400; `Page.Limit` and `Page.Offset` feed SQL windows
directly; `NewPageResult` assembles the standard `items/total/page/size/
has_next` response shape so clients and generated SDKs see one contract.

```go
page, err := transporthttp.ParsePage(r)
if err != nil {
    return err
}
rows, total := repo.List(ctx, page.Limit(), page.Offset())
return transporthttp.NewPageResult(page, total, rows), nil
```

## Hook Semantics

Across HTTP and gRPC, client and server transports share the same high-level hook model even though their concrete function signatures are protocol-specific.

The intended semantic contract is:

- `Before`
  Runs before decode or before the outbound call is sent.
  Use it for request metadata, headers, auth context, tracing context, and request correlation.
- `After`
  Runs after a successful endpoint call or successful remote response, but before the transport finishes writing or returning the response.
  Use it for response metadata, response headers, and observability enrichment.
- `Finalizer`
  Runs at the end regardless of success or failure.
  Use it for latency recording, access logging, metrics, and cleanup.

Design rule:

- preserve this semantic ordering across transport implementations
- do not use transport hooks as a substitute for endpoint middleware when the concern is transport-agnostic

## HTTP Server

Use `transport/http/server` when exposing HTTP APIs.

Recommended entry points:

- `server.NewServer`
- `server.NewTypedJSONServer`
- `server.NewJSONServer`
- `server.NewJSONEndpoint`
- `server.NewStrictTypedJSONServer`
- `server.NewStrictJSONServer`
- `server.NewStrictJSONEndpoint`
- `server.NewTypedJSONServerWithMiddleware`
- `server.NewJSONServerWithMiddleware`
- `server.DecodeJSONRequest`
- `server.DecodeJSONRequestWithOptions`
- `server.DecodeJSONBody`
- `server.StrictJSONDecodeOptions`
- `server.DefaultMaxJSONBodyBytes`
- `server.EncodeJSONResponse`
- `server.JSONErrorEncoder`
- `server.NewHTTPError`
- `server.WrapHTTPError`
- `server.ParseMultipartForm` for bounded multipart/form-data uploads
- `server.WriteAttachment` for file downloads

`ParseMultipartForm` enforces a total body cap, a per-file cap, and the
in-memory threshold before parts spill to temporary files; limit violations
classify as 413 and malformed requests as 415/400 through the standard error
encoders. `WriteAttachment` sets a sanitized `Content-Disposition` (RFC 2231
for non-ASCII names) and derives the content type from the filename.

Primary extension points:

- `ServerBefore`
- `ServerAfter`
- `ServerFinalizer`
- `ServerErrorHandler`
- `ServerErrorEncoder`
- `ServerErrorEncoder`
- `ServerErrorHandler`

The default error handler is a no-op. Install an application-owned handler when
errors must be logged or recorded, for example
`zapadapter.NewErrorHandler(logger)` from `integrations/zap`.

Typical flow:

1. `ServerBefore` hooks populate context from the request.
2. A decode function maps HTTP input into a domain request.
3. The endpoint is invoked.
4. `ServerAfter` hooks inspect or enrich the response path.
5. An encode function writes the response.
6. Finalizers run regardless of success or failure.

Minimal example:

```go
handler := server.NewTypedJSONServer(
    func(ctx context.Context, req HelloReq) (HelloResp, error) {
        return hello(ctx, req)
    },
    server.ServerErrorEncoder(server.JSONErrorEncoder),
)

http.Handle("/hello", handler)
```

Prefer the fully typed helpers when the response has a concrete type. The JSON
helpers are strict by default: they reject unknown object fields, a second JSON
value, and bodies larger than the default byte limit.
Use the explicit strict helpers when a route needs a custom body limit:

```go
handler := server.NewStrictJSONEndpoint[HelloReq](
    ep,
    server.DefaultMaxJSONBodyBytes,
    server.ServerErrorEncoder(server.JSONErrorEncoder),
)
```

Decode errors returned by JSON request decoders carry HTTP 400 status metadata
for `JSONErrorEncoder`.

`JSONErrorEncoder` writes `code`, `message`, and optional `request_id` fields.
Prefer `apperror.New` or `apperror.Wrap` in application code so the failure
classification remains independent of HTTP. The HTTP transport maps application
kinds to status codes and uses their stable code and public message. Low-level
HTTP integrations may still return `server.NewHTTPError` or implement
`transporthttp.StatusCoder`, `transporthttp.ErrorCoder`, and
`transporthttp.PublicMessager`. Both built-in error encoders redact unclassified
5xx details instead of exposing the internal error string.

## HTTP Client

Use `transport/http/client` when calling HTTP APIs through endpoint-style abstractions.

Recommended entry points:

- `client.NewClient`
- `client.NewJSONClient`
- `client.NewJSONClientWithMaxResponseBodyBytes`
- `client.NewJSONClientWithTimeout`
- `client.EncodeJSONRequest`

`NewJSONClient` encodes GET/HEAD requests as path/query parameters and keeps the
request body empty. Successful JSON responses are capped at 4 MiB by default;
use `NewJSONClientWithMaxResponseBodyBytes` for an intentional larger contract.
`NewJSONClientWithTimeout` adds a context timeout; use `sd/client.NewEndpoint` with an
explicit retry classifier when retries are required.

Primary extension points:

- `ClientBefore`
- `ClientAfter`
- `ClientFinalizer`
- custom request encoders
- custom response decoders
- custom HTTP client injection

Minimal example:

```go
ep, err := client.NewJSONClient[HelloResp](
    http.MethodPost,
    "http://localhost:8080/hello",
)
if err != nil {
    return err
}

resp, err := ep(ctx, HelloReq{Name: "world"})
```

Typical flow:

1. `ClientBefore` hooks enrich the outbound request context or headers.
2. The request is encoded and sent.
3. The response is decoded.
4. `ClientAfter` hooks inspect the successful response path.
5. Finalizers run regardless of success or failure.

## gRPC Server

Use `integrations/grpc/server` when exposing gRPC APIs.

Recommended entry points:

- `server.NewServer`
- public request/response encode/decode hooks

Primary extension points:

- `ServerBefore`
- `ServerAfter`
- `ServerFinalizer`

Typical flow mirrors the HTTP server path:

1. request metadata is read into context
2. the request is decoded into a domain request
3. the endpoint is invoked
4. response metadata can be written
5. the response is encoded back to the gRPC caller

The default error encoder preserves existing gRPC status errors, maps
transport-neutral `apperror` kinds to gRPC codes, and redacts unknown failures
as `codes.Internal`. Install `ServerErrorEncoder` only when an application needs
a different wire-error policy; error handlers still receive the original error
for logging and metrics.

## gRPC Client

Use `integrations/grpc/client` when making gRPC calls through framework abstractions.

Recommended entry points:

- `client.NewClient`
- public encode/decode functions

Primary extension points:

- `ClientBefore`
- `ClientAfter`
- `ClientFinalizer`

Typical flow mirrors the HTTP client path:

1. `ClientBefore` hooks enrich outgoing metadata.
2. The request is encoded and sent.
3. The response is decoded.
4. `ClientAfter` hooks inspect successful response metadata.
5. Finalizers run regardless of success or failure.

Current metadata note:

- gRPC client response headers and trailers are exposed in context for decode/finalizer-time inspection via `integrations/grpc` context keys.

## What Belongs In Transport

Good transport responsibilities:

- HTTP request parsing
- gRPC metadata extraction
- JSON encoding and decoding
- response status mapping
- wire-level error encoding
- protocol-specific hooks

## What Does Not Belong In Transport

Avoid putting these concerns here:

- domain decision logic
- business validation that belongs in service logic
- timeout, retry, logging, rate limiting, or circuit breaking when they can be modeled as endpoint middleware
- one-off product workflow behavior

These are framework anti-patterns because they weaken separation between protocol and business logic.

## Best Practices

1. Keep request/response mapping explicit.
2. Prefer endpoint middleware for reusable runtime policies.
3. Use JSON helpers for common HTTP cases instead of hand-writing boilerplate.
4. Keep transport code small and easy to replace.
5. Use transport hooks for metadata and observability, not for business workflows.

## Stability Notes

The published `v2.0.0` transport API remains a historical stable baseline. The
approved `v2.1.0` SemVer exception resets the contract to the reviewed API
snapshot. After `v2.1.0`, compatibility covers documented behavior, not
internal execution details such as exact writer interception or internal
request lifecycle structure.

## Related Docs

- [README.md](../README.md)
- [ARCHITECTURE.md](../ARCHITECTURE.md)
- [PRODUCTION.md](../PRODUCTION.md)
