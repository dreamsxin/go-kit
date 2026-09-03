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

## Dual-Protocol Binding

One business endpoint can serve HTTP and gRPC without duplicated assembly.
`transport.Binding[Req, Resp]` carries the middleware-built endpoint once;
each protocol then owns only its wire codec:

```go
binding := transport.Binding[HelloRequest, HelloResponse]{
    Endpoint: endpoint.TypedEndpoint[HelloRequest, HelloResponse](sayHello).Wrap(),
}

// HTTP: strict typed JSON in one call.
kit.HandleJSONTyped(httpComponent, "POST /hello", binding.TypedEndpoint())

// gRPC: the protobuf-facing codec maps wire messages to the domain types.
srv := grpcserver.NewServer(
    binding.Endpoint,
    func(_ context.Context, m any) (any, error) {
        r := m.(*pb.HelloRequest)
        return HelloRequest{Name: r.Name}, nil
    },
    func(_ context.Context, resp any) (any, error) {
        r := resp.(HelloResponse)
        return &pb.HelloReply{Message: r.Message}, nil
    },
)
pb.RegisterGreeterServer(grpcComponent.Server(), srv)
```

The HTTP side needs no extra work because `Binding.TypedEndpoint()` returns
the typed endpoint the JSON servers accept. The gRPC side is one
`grpcserver.NewServer` call with the two protobuf mapping functions. The same
`endpoint` middleware chain (timeout, metrics, circuit breaker, ...) runs on
both protocols, because the binding carries the already-composed endpoint.

The full runnable test (a real HTTP server plus a real gRPC server over
bufconn, driven by one binding) is `integrations/grpc/dual_protocol_test.go`.

## Custom Body Formats

HTTP routes are not JSON-only. Two pure functions become the transport codec;
the error encoder shares the content type so error responses match the format:

```go
decode, encode := server.RawBodyCodec(
    unmarshalProto,   // func([]byte) (any, error)
    marshalProto,     // func(any) ([]byte, error)
    "application/x-protobuf",
)
svc.Handle("POST /shout", server.NewServer(ep, decode, encode,
    server.ServerErrorEncoder(server.TextErrorEncoder("application/x-protobuf")),
))
```

`RawBodyCodec` bounds the body (1 MiB by default, configurable) and preserves
`StatusCoder`/`Headerer` on the response. `TextErrorEncoder` keeps 4xx
messages public and 5xx opaque, in the route's format instead of JSON.
`RawBodyCodecWithMaxBytes` takes an explicit limit.

> [!NOTE]
> apperror is optional here. A custom route can classify nothing and let its
> own error encoder decide statuses and bodies; the framework does not force
> any error model. apperror exists to make the common HTTP/gRPC mappings
> automatic when you want them.

The runnable walkthrough is [examples/customcodec](../examples/README.md).

## Composition And Nesting

Components compose in two clearly separated styles.

**Accumulating** - pass as many as you need; they stack in the order added:

- `ServerBefore` / `ServerAfter` / `ServerFinalizer` hooks (and the client
  `Before` / `After` / finalizer equivalents) run in registration order;
- endpoint middleware (`endpoint.Chain`, `Builder.Use`, `kit.WithEndpointMiddleware`)
  nests arbitrarily: the first middleware is the outermost, and a wrapped
  endpoint can itself be a chain (a `Fallback` endpoint, for example, can be a
  fully middleware-built endpoint);
- `security/http.Chain` and `kit.WithHTTPMiddleware` compose standard
  `http.Handler` middleware the same way;
- `kit.WithJSONServerOptions` may be passed several times; its options are
  appended to every JSON route.

**Replacing** - exactly one wins per route, the last one set:

- the success response encoder (`server.ServerResponseEncoder`, default
  `EncodeJSONResponse`) — note it applies only to the **JSON entry points**
  (`NewJSONServer`, `NewJSONEndpoint`, and their typed and strict variants). A
  server built with `NewServer` takes its encode function as a constructor
  argument, so passing `ServerResponseEncoder` to it is silently overridden;
- the error encoder (`server.ServerErrorEncoder`);
- the request body decoder (the constructor `DecodeRequestFunc`).

Multiple format conversions still compose, but inside the one installed
function: an envelope encoder can marshal, post-process, or delegate itself.

**Combining request parsers.** A route has one body decoder, but body, path,
query, and multipart inputs combine inside it. `DecodeQueryRequest` fills the
struct from **both** path and query values, with path winning on a name clash,
so it is all a GET-style route needs:

```go
func decodeList(ctx context.Context, r *http.Request) (any, error) {
    var req ListOrdersRequest          // form/json tags map query and path fields
    if err := transporthttp.DecodeQueryRequest(r, &req); err != nil {
        return nil, err                // query and path both land here
    }
    page, err := transporthttp.ParsePage(r)
    if err != nil {
        return nil, err
    }
    req.Page = page
    return req, nil
}
```

`DecodePathRequest` is the *body* companion: run it after JSON decoding so path
values override whatever the body carried.

File uploads combine the same way: decode form fields with
`server.ParseMultipartForm` inside the decoder, then read the file part.

**Startup-time struct validation.** Reflection-based query decoding discovers
unsupported field types at the first request. Validate the struct once at
assembly so a bad tag or unsupported type fails fast:

```go
if err := transporthttp.ValidateQueryStruct[ListOrdersRequest](); err != nil {
    return err
}
```

**Envelopes without rewriting the encoder.** `server.WrapJSONResponse` wraps
the response value while preserving the original response's `StatusCoder` and
`Headerer` behavior:

```go
kit.NewHTTP(":8080", kit.WithJSONServerOptions(
    server.ServerResponseEncoder(server.WrapJSONResponse(func(response any) any {
        return envelope{Code: 0, Message: "ok", Data: response}
    })),
))
```

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

- `server.NewServer` — the general server; you supply decode and encode
- `server.NewJSONServer` / `server.NewTypedJSONServer` — handler in, JSON route out
- `server.NewJSONEndpoint` — same, for an endpoint you already built
- `server.NewJSONServerWithBodyLimit` / `server.NewTypedJSONServerWithBodyLimit` /
  `server.NewJSONEndpointWithBodyLimit` — the same three with an explicit body cap
- `server.NewJSONServerWithMiddleware` / `server.NewTypedJSONServerWithMiddleware`
- `server.NewJSONEndpointWithDecodeOptions` — the escape hatch for changing
  strictness itself
- `server.DecodeJSONRequest` / `server.DecodeJSONRequestWithOptions` /
  `server.DecodeJSONBody`
- `server.StrictJSONDecodeOptions`, `server.JSONDecodeOptions`,
  `server.DefaultMaxJSONBodyBytes`
- `server.EncodeJSONResponse`, `server.WrapJSONResponse`
- `server.JSONErrorEncoder`, `server.DefaultErrorEncoder`,
  `server.JSONErrorEncoderWithKindMapper`, `server.TextErrorEncoder`
- `server.HTTPStatusForError` / `server.HTTPStatusForErrorKind` — the status an
  error maps to, for relaying an upstream failure or writing a custom encoder
- `server.RawBodyCodec` / `server.RawBodyCodecWithMaxBytes` for non-JSON bodies
- `server.NopRequestDecoder` / `server.NopResponseEncoder`
- `server.NewSSEServer` / `server.NewSSEServerTyped` for Server-Sent Events streams
- `server.ParseMultipartForm` for bounded multipart/form-data uploads
- `server.WriteAttachment` for file downloads
- `server.AccessLogMiddleware` for a standard-library `slog` access log

Every JSON entry point decodes **strictly**: unknown object fields, a second
JSON value, and bodies over `DefaultMaxJSONBodyBytes` are rejected with 400.
The `WithBodyLimit` variants change only the size cap;
`NewJSONEndpointWithDecodeOptions` is the only way to relax strictness.

`ParseMultipartForm` enforces a total body cap, a per-file cap, and the
in-memory threshold before parts spill to temporary files; limit violations
classify as 413 and malformed requests as 415/400 through the standard error
encoders. `WriteAttachment` sets a sanitized `Content-Disposition` (RFC 2231
for non-ASCII names) and derives the content type from the filename.

### Server-Sent Events streams

`server.NewSSEServer` and `NewSSEServerTyped` adapt a streaming handler to
`http.Handler`: the `SSEStreamHandler` receives the decoded request and an
`SSEStream` event writer (`Data`, `Event`, `EventJSON`, `Comment`, `Retry`).
Hook semantics:

- `ServerBefore` runs before the stream starts;
- the decode function runs before any SSE headers are written, so decode
  failures map to regular error responses (for example 400) through the
  ErrorEncoder;
- `ServerErrorHandler` observes decode failures and errors returned by the
  stream handler after streaming began;
- `ServerFinalizer` always runs when the stream ends;
- `ServerAfter` and `ServerResponseEncoder` do not apply to streams and are
  ignored.

Composed with endpoint middleware, one stream counts as one request: metrics
record one call, a timeout middleware bounds the total stream duration
(long-lived streams should avoid or relax global deadlines), and
authentication or validation runs before the stream starts. Errors returned
after streaming began cannot reach the client; emit a terminal event when
clients need to learn about a failure.

Primary extension points:

- `ServerBefore`
- `ServerAfter`
- `ServerFinalizer`
- `ServerErrorHandler`
- `ServerErrorEncoder`
- `ServerResponseEncoder` (JSON entry points only)

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

Prefer the fully typed helpers when the response has a concrete type. All JSON
helpers are strict: they reject unknown object fields, a second JSON value, and
bodies larger than the default byte limit. Use a `WithBodyLimit` helper when a
route needs a different cap:

```go
handler := server.NewJSONEndpointWithBodyLimit[HelloReq](
    ep,
    64<<10,
    server.ServerErrorEncoder(server.JSONErrorEncoder),
)
```

Decode errors returned by JSON request decoders carry HTTP 400 status metadata
for `JSONErrorEncoder`.

`JSONErrorEncoder` writes `code`, `message`, and optional `request_id` fields.
Prefer `apperror.New` or `apperror.Wrap` in application code so the failure
classification remains independent of HTTP. The HTTP transport maps application
kinds to status codes and uses their stable code and public message. Low-level
HTTP integrations may implement
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
- `client.DecodeJSONResponse`
- `client.DecodeJSONResponseWithMaxBodyBytes`

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

`DecodeJSONResponse` is the default decoder exported as a building block, so a
custom client composed with `NewExplicitClient` (or a custom wrapper around it)
reuses the same status handling and body limit instead of reimplementing them.

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
