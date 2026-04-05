package transport

import (
	"context"

	"google.golang.org/grpc"
	grpcserver "github.com/dreamsxin/go-kit/transport/grpc/server"
	grpcclient "github.com/dreamsxin/go-kit/transport/grpc/client"
	"github.com/dreamsxin/go-kit/endpoint"
	idl "github.com/dreamsxin/go-kit/examples/microgen_skill/pb"
	genendpoint "github.com/dreamsxin/go-kit/examples/microgen_skill/endpoint"
)

// grpcServer 实现了 idl.GreeterServer 接口
type grpcServer struct {
	sayhello grpcserver.Handler
	getstatus grpcserver.Handler
}


func (s *grpcServer) SayHello(ctx context.Context, req *idl.HelloRequest) (*idl.HelloResponse, error) {
	_, resp, err := s.sayhello.ServeGRPC(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.(*idl.HelloResponse), nil
}

func (s *grpcServer) GetStatus(ctx context.Context, req *idl.Empty) (*idl.StatusResponse, error) {
	_, resp, err := s.getstatus.ServeGRPC(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.(*idl.StatusResponse), nil
}


// RegisterGRPCServer 将服务端点注册到 gRPC Server
func RegisterGRPCServer(s *grpc.Server, endpoints genendpoint.GreeterEndpoints) {
	idl.RegisterGreeterServer(s, &grpcServer{
		sayhello: grpcserver.NewServer(
			endpoints.SayHelloEndpoint,
			DecodeSayHelloGRPCRequest,
			EncodeSayHelloGRPCResponse,
		),
		getstatus: grpcserver.NewServer(
			endpoints.GetStatusEndpoint,
			DecodeGetStatusGRPCRequest,
			EncodeGetStatusGRPCResponse,
		),
	})
}


// NewGRPCSayHelloClient 返回一个用于 SayHello 的 gRPC 客户端。
func NewGRPCSayHelloClient(conn *grpc.ClientConn, options ...grpcclient.ClientOption) endpoint.Endpoint {
	return grpcclient.NewClient(
		conn,
		"greeter.Greeter",
		"SayHello",
		EncodeGRPCSayHelloRequest,
		DecodeGRPCSayHelloResponse,
		&idl.HelloResponse{},
		options...,
	).Endpoint()
}

// NewGRPCGetStatusClient 返回一个用于 GetStatus 的 gRPC 客户端。
func NewGRPCGetStatusClient(conn *grpc.ClientConn, options ...grpcclient.ClientOption) endpoint.Endpoint {
	return grpcclient.NewClient(
		conn,
		"greeter.Greeter",
		"GetStatus",
		EncodeGRPCGetStatusRequest,
		DecodeGRPCGetStatusResponse,
		&idl.StatusResponse{},
		options...,
	).Endpoint()
}



// DecodeSayHelloGRPCRequest 从 gRPC 请求中解码。
func DecodeSayHelloGRPCRequest(_ context.Context, grpcReq any) (any, error) {
	req := grpcReq.(*idl.HelloRequest)
	return *req, nil
}

// EncodeSayHelloGRPCResponse 编码为 gRPC 响应。
func EncodeSayHelloGRPCResponse(_ context.Context, response any) (any, error) {
	resp := response.(idl.HelloResponse)
	return &resp, nil
}

// EncodeGRPCSayHelloRequest 编码为 gRPC 请求 (客户端使用)。
func EncodeGRPCSayHelloRequest(_ context.Context, request any) (any, error) {
	req := request.(idl.HelloRequest)
	return &req, nil
}

// DecodeGRPCSayHelloResponse 从 gRPC 响应中解码 (客户端使用)。
func DecodeGRPCSayHelloResponse(_ context.Context, grpcReply any) (any, error) {
	resp := grpcReply.(*idl.HelloResponse)
	return *resp, nil
}

// DecodeGetStatusGRPCRequest 从 gRPC 请求中解码。
func DecodeGetStatusGRPCRequest(_ context.Context, grpcReq any) (any, error) {
	req := grpcReq.(*idl.Empty)
	return *req, nil
}

// EncodeGetStatusGRPCResponse 编码为 gRPC 响应。
func EncodeGetStatusGRPCResponse(_ context.Context, response any) (any, error) {
	resp := response.(idl.StatusResponse)
	return &resp, nil
}

// EncodeGRPCGetStatusRequest 编码为 gRPC 请求 (客户端使用)。
func EncodeGRPCGetStatusRequest(_ context.Context, request any) (any, error) {
	req := request.(idl.Empty)
	return &req, nil
}

// DecodeGRPCGetStatusResponse 从 gRPC 响应中解码 (客户端使用)。
func DecodeGRPCGetStatusResponse(_ context.Context, grpcReply any) (any, error) {
	resp := grpcReply.(*idl.StatusResponse)
	return *resp, nil
}

