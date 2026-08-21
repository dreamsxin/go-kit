# gRPC 集成

[English](README.md) | 简体中文

`integrations/grpc` 把 endpoint 适配到 gRPC。它是独立模块，因此纯 HTTP 服务不会引入 gRPC 依赖。

## 安装

```bash
go get github.com/dreamsxin/go-kit/v2/integrations/grpc@v0.2.2
```

## 服务端

将 endpoint 包装为 gRPC 服务方法。元数据流入请求 context；错误编码器把应用错误映射为 gRPC 状态码。

```go
server := grpcserver.NewServer(
	ep,
	grpcserver.DecodeProto[pb.HelloRequest],
	grpcserver.EncodeProto[pb.HelloResponse],
	grpcserver.ServerErrorEncoder(grpcserver.DefaultErrorEncoder),
	grpcserver.ServerBefore(grpcserver.PopulateRequestContext),
)
```

用生成的 protobuf 服务注册方法注册该服务：

```go
pb.RegisterGreeterServer(grpcComponent.Server(), server)
```

通过 `kit/grpc` 的生命周期组件把 gRPC 监听器接入 kit 服务。

## 客户端

构建调用远程 gRPC 服务的 endpoint。客户端处理元数据、超时与非成功状态。

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

`grpcclient.NewClient` 接受拦截器，用于认证、追踪与元数据传播。

## 重试分类

`integrations/grpc.Retryable` 为 `sd/retry` 分类瞬态 gRPC 状态。在组合
gRPC 客户端的重试时，把它作为可重试分类器传入：

```go
call, closer, err := sdclient.NewEndpoint(instancer, factory, logger,
	sdclient.WithRetryable(grpcclient.Retryable),
)
```

未知错误视为永久错误。默认分类器只重试显式 `Retryable() == true` 的错误
和已知的瞬态 gRPC 代码。
