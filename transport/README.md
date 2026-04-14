# transport

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

The transport layer is split into four main public areas:

- `transport/http/server`
- `transport/http/client`
- `transport/grpc/server`
- `transport/grpc/client`

Common helpers also live under:

- `transport/error_handler.go`
- `transport/http`
- `transport/grpc`

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
- `server.NewJSONServer`
- `server.NewJSONEndpoint`
- `server.NewJSONServerWithMiddleware`
- `server.DecodeJSONRequest`
- `server.EncodeJSONResponse`
- `server.JSONErrorEncoder`

Primary extension points:

- `ServerBefore`
- `ServerAfter`
- `ServerFinalizer`
- `ServerErrorEncoder`
- `ServerErrorHandler`

Typical flow:

1. `ServerBefore` hooks populate context from the request.
2. A decode function maps HTTP input into a domain request.
3. The endpoint is invoked.
4. `ServerAfter` hooks inspect or enrich the response path.
5. An encode function writes the response.
6. Finalizers run regardless of success or failure.

Minimal example:

```go
handler := server.NewJSONServer[HelloReq](
    func(ctx context.Context, req HelloReq) (any, error) {
        return ep(ctx, req)
    },
    server.ServerErrorEncoder(server.JSONErrorEncoder),
)

http.Handle("/hello", handler)
```

## HTTP Client

Use `transport/http/client` when calling HTTP APIs through endpoint-style abstractions.

Recommended entry points:

- `client.NewClient`
- `client.NewJSONClient`
- `client.NewJSONClientWithRetry`
- `client.EncodeJSONRequest`

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

Use `transport/grpc/server` when exposing gRPC APIs.

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

## gRPC Client

Use `transport/grpc/client` when making gRPC calls through framework abstractions.

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

- gRPC client response headers and trailers are exposed in context for decode/finalizer-time inspection via `transport/grpc` context keys.

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

The transport layer has two different maturity levels:

- `transport/http/server` and `transport/http/client` are part of the stable public surface.
- `transport/grpc/server` and `transport/grpc/client` are public but still more evolvable.

Do not depend on undocumented internal execution details such as exact writer interception or internal request lifecycle structure.

## Related Docs

- [README.md](../README.md)
- [FRAMEWORK_BOUNDARIES.md](../FRAMEWORK_BOUNDARIES.md)
- [STABILITY.md](../STABILITY.md)
- [PACKAGE_SURFACES.md](../PACKAGE_SURFACES.md)
- [ANTI_PATTERNS.md](../ANTI_PATTERNS.md)
