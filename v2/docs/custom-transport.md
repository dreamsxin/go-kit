# Custom Transport Protocols

English | [简体中文](custom-transport_zh.md)

Use this chapter when the wire protocol is not the built-in JSON HTTP or gRPC
path. The rule is simple: a transport translates bytes and protocol metadata;
the endpoint and service stay unchanged.

## Choose The Smallest Adapter

| Situation | Use |
| --- | --- |
| a different body format over HTTP | `server.RawBodyCodec` with `server.NewServer` |
| a custom HTTP error format | `server.ServerErrorEncoder` |
| a new socket, queue, or RPC protocol | a small adapter around `endpoint.Endpoint` |
| long-lived messages | a stream adapter with context cancellation and explicit close |

For HTTP codecs, start with the runnable [customcodec example](../examples/customcodec/main.go).
For common HTTP and gRPC entry points, use the [transport reference](../transport/README.md).

## Custom HTTP Body Format

Two functions are enough when the protocol is still HTTP:

```go
decode, encode := server.RawBodyCodec(
    func(body []byte) (any, error) { return decodeMessage(body) },
    func(response any) ([]byte, error) { return encodeMessage(response) },
    "application/x-message",
)

handler := server.NewServer(
    ep,
    decode,
    encode,
    server.ServerErrorEncoder(server.TextErrorEncoder("application/x-message")),
)
```

`RawBodyCodec` applies the default request-body limit. Use
`RawBodyCodecWithMaxBytes` when the wire contract needs another limit. Keep the
error encoder in the same media type; errors still use the normal status and
classification rules.

## A New Protocol Adapter

An adapter has four steps:

1. derive a request context from the connection or message;
2. decode one wire request into a domain request;
3. call the endpoint exactly once for that message;
4. encode the response or map the error to the protocol's error shape.

The core can be this small:

```go
type Adapter struct {
    Endpoint endpoint.Endpoint
    Decode   func(context.Context, []byte) (any, error)
    Encode   func(context.Context, any) ([]byte, error)
    Error    func(context.Context, error) error
}

func (a Adapter) Handle(ctx context.Context, wire []byte) ([]byte, error) {
    request, err := a.Decode(ctx, wire)
    if err != nil {
        return nil, a.Error(ctx, err)
    }
    response, err := a.Endpoint(ctx, request)
    if err != nil {
        return nil, a.Error(ctx, err)
    }
    return a.Encode(ctx, response)
}
```

The adapter owns framing, maximum message size, protocol status, headers or
trailers, and connection shutdown. The endpoint owns timeout, validation,
authentication middleware, metrics, retry policy, and business errors. Do not
move service decisions into `Decode`, `Encode`, or the protocol error mapper.

## Error And Context Rules

- Preserve the original error for logging and `errors.Is`/`errors.As`.
- Map `apperror.Kind` to the protocol's status or error code in one place.
- Use `PublicMessage` when the protocol has a client-visible message; keep
  `Error()` for diagnostics.
- Propagate cancellation from the connection to the endpoint. A stream must
  stop reading and writing after its context ends.
- Bound every frame, request, response, and queued message. A custom decoder is
  an untrusted input boundary.

If the protocol has a response-side failure field, decide whether it is data or
an error. Use `endpoint.Failer` only for response types whose signature cannot
return an error; otherwise return the error normally.

## Streaming And Lifecycle

For a long-lived protocol, define when one `Done` event completes: after one
message, after a connection closes, or after a stream terminates. The generic
endpoint layer cannot infer that boundary. The adapter must also implement a
bounded close path and ensure no goroutine remains blocked on a connection or
queue after cancellation.

Test the adapter at three levels:

- pure decode/encode tests for malformed and oversized frames;
- endpoint tests for cancellation, error classification, and `Done` timing;
- a protocol smoke test using an in-memory connection or test server.

The adapter should be replaceable without changing the service or endpoint
packages. That is the boundary this chapter is protecting.
