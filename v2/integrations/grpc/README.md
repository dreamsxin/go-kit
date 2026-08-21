# gRPC Integration

English | [简体中文](README_zh.md)

`integrations/grpc` adapts endpoints to gRPC. It is an independent module so
HTTP-only services do not pull gRPC dependencies.

## Install

```bash
go get github.com/dreamsxin/go-kit/v2/integrations/grpc@v0.2.2
```

## Server

Wrap an endpoint as a gRPC service method. Metadata flows into the request
context; the error encoder maps application errors to gRPC status codes.

```go
server := grpcserver.NewServer(
	ep,
	grpcserver.DecodeProto[pb.HelloRequest],
	grpcserver.EncodeProto[pb.HelloResponse],
	grpcserver.ServerErrorEncoder(grpcserver.DefaultErrorEncoder),
	grpcserver.ServerBefore(grpcserver.PopulateRequestContext),
)
```

Register the server with the generated protobuf service registration:

```go
pb.RegisterGreeterServer(grpcComponent.Server(), server)
```

See `kit/grpc` for the lifecycle component that attaches the gRPC listener to
a kit service.

## Client

Build an endpoint that calls a remote gRPC service. The client handles
metadata, timeouts, and non-success statuses.

```go
client := grpcclient.NewClient(
	conn,
	"hello.Greeter",
	"SayHello",
	grpcclient.EncodeProto[pb.HelloRequest],
	grpcclient.DecodeProto[pb.HelloResponse],
)
ep := client.Endpoint()
```

`grpcclient.NewClient` accepts interceptors for authentication, tracing, and
metadata propagation.

## Retry Classification

`integrations/grpc.Retryable` classifies transient gRPC statuses for
`sd/retry`. Pass it as the retryable classifier when composing retry over a
gRPC client:

```go
call, closer, err := sdclient.NewEndpoint(instancer, factory, logger,
	sdclient.WithRetryable(grpcclient.Retryable),
)
```

Unknown errors are permanent. The default classifier retries only explicit
`Retryable() == true` errors and known transient gRPC codes.
