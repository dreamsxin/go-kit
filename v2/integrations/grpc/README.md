# gRPC Integration

English | [简体中文](README_zh.md)

`integrations/grpc` adapts endpoints to gRPC. It is an independent module so
HTTP-only services do not pull gRPC dependencies.

## Install

```bash
go get github.com/dreamsxin/go-kit/v2/integrations/grpc@latest
```

## Server

Wrap an endpoint as a gRPC service method. Decode and encode functions map
between protobuf messages and domain types; hooks shape metadata and errors.

```go
srv := grpcserver.NewServer(
	sayHelloEndpoint,
	func(_ context.Context, req any) (any, error) {
		r := req.(*pb.HelloRequest)
		return HelloRequest{Name: r.Name}, nil
	},
	func(_ context.Context, resp any) (any, error) {
		r := resp.(HelloResponse)
		return &pb.HelloReply{Message: r.Message}, nil
	},
	grpcserver.ServerBefore(myMetadataHook), // your metadata hooks
)
```

Attach the gRPC listener to a kit service through the `kit/grpc` lifecycle
component:

```go
grpcComponent, err := kitgrpc.New(":8081")
if err != nil {
	return err
}
pb.RegisterGreeterServer(grpcComponent.Server(), srv)

host, err := kit.NewHost(kit.WithLifecycle(httpComponent, grpcComponent))
```

Errors are mapped to gRPC status codes through `DefaultErrorEncoder`, which
classifies `apperror` kinds (NotFound, InvalidArgument, Unauthenticated, ...).
The stable application code travels in a `google.rpc.ErrorInfo` detail and a
retry hint in `google.rpc.RetryInfo`, so neither is lost the way a status code
alone would lose them. `NewErrorEncoder` configures both concerns:

```go
grpcserver.ServerErrorEncoder(grpcserver.NewErrorEncoder(
	grpcserver.WithErrorDomain("greeter.example.com"),
	grpcserver.WithKindMapper(func(kind string) codes.Code {
		if kind == "payment_failed" {
			return codes.FailedPrecondition
		}
		return codes.Code(99) // invalid: fall back to the built-in mapping
	}),
))
```

## Client

Build an endpoint that calls a remote gRPC method:

```go
call := grpcclient.NewClient(
	conn,
	"hello.Greeter",
	"SayHello",
	func(_ context.Context, req any) (any, error) {
		r := req.(HelloRequest)
		return &pb.HelloRequest{Name: r.Name}, nil
	},
	func(_ context.Context, resp any) (any, error) {
		r := resp.(*pb.HelloReply)
		return HelloResponse{Message: r.Message}, nil
	},
	&pb.HelloReply{},
).Endpoint()
```

`ClientBefore`/`ClientAfter` hooks manage metadata; `ClientFinalizer` always
runs for observability.

## Error Classification And Retry

The client wraps every failed `Invoke` in `grpc.StatusError`. It stays a gRPC
error — `status.FromError` and `status.Code` keep working — and adds the
framework contracts: `ErrorKindName` maps the code to a transport-neutral kind
(`grpc.KindNameForCode`, the inverse of `grpcserver.CodeForErrorKind`), `Retryable`
reports whether the code is transient, `ErrorCode`/`ErrorDomain` read the
`google.rpc.ErrorInfo` detail so the upstream application code is not lost, and
`RetryAfter` reads the `google.rpc.RetryInfo` detail. A gRPC client endpoint
therefore composes with the core middleware with no extra wiring:

```go
ep := endpoint.NewBuilder(call.Endpoint()).
    WithCircuitBreaker(breaker).
    WithRetry(3). // Unavailable/ResourceExhausted/Aborted, honoring RetryInfo
    Build()
```

Translate before relaying: a dependency's `NotFound` would otherwise make your
own handler answer 404 —
`apperror.WrapCause(apperror.KindUnavailable, "upstream.greeter", err)`.

`grpc.Retryable` classifies transient gRPC statuses (Unavailable,
ResourceExhausted, Aborted) for `sd/retry`. Pass it as the retryable classifier
when composing retry over a gRPC client:

```go
call, closer, err := sdclient.NewEndpoint(instancer, factory, logger,
	sdclient.WithRetryable(grpc.Retryable),
)
```

Unknown errors are permanent; only classified transient errors are retried.

The runnable reference for servers, clients, and hooks is
[examples/transport](../../examples/README.md).
