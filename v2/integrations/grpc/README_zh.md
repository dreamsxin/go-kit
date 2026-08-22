# gRPC 集成

[English](README.md) | 简体中文

`integrations/grpc` 把 endpoint 适配到 gRPC。它是独立模块，因此纯 HTTP 服务不会引入 gRPC 依赖。

## 安装

```bash
go get github.com/dreamsxin/go-kit/v2/integrations/grpc@v0.2.4
```

## 服务端

将 endpoint 包装为 gRPC 服务方法。编解码函数在 protobuf 消息与领域类型之间转换；钩子处理元数据与错误。

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
	grpcserver.ServerBefore(myMetadataHook), // 你的元数据钩子
)
```

通过 `kit/grpc` 生命周期组件把 gRPC 监听器接入 kit 服务：

```go
grpcComponent, err := kitgrpc.New(":8081")
if err != nil {
	return err
}
pb.RegisterGreeterServer(grpcComponent.Server(), srv)

svc, err := kit.New(":8080", kit.WithLifecycle(grpcComponent))
```

错误经 `DefaultErrorEncoder` 映射为 gRPC 状态码，它分类 `apperror` 种类
（NotFound、InvalidArgument、Unauthenticated 等）。

## 客户端

构建调用远程 gRPC 方法的 endpoint：

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

`ClientBefore`/`ClientAfter` 钩子管理元数据；`ClientFinalizer` 始终执行，
用于可观测性。

## 重试分类

`grpc.Retryable` 为 `sd/retry` 分类瞬态 gRPC 状态（Unavailable、
ResourceExhausted、Aborted）。在组合 gRPC 客户端的重试时，把它作为
可重试分类器传入：

```go
call, closer, err := sdclient.NewEndpoint(instancer, factory, logger,
	sdclient.WithRetryable(grpc.Retryable),
)
```

未知错误视为永久错误；只有已分类的瞬态错误会被重试。

服务端、客户端与钩子的可运行参照见
[examples/transport](../../examples/README_zh.md)。
