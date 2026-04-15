package userservice

import (
	"context"

	"github.com/dreamsxin/go-kit/endpoint"
	grpcclient "github.com/dreamsxin/go-kit/transport/grpc/client"
	grpcserver "github.com/dreamsxin/go-kit/transport/grpc/server"
	"google.golang.org/grpc"
	idl "example.com/gen_proto_component_flow/pb"
	genendpoint "example.com/gen_proto_component_flow/endpoint/userservice"
)

// grpcServer implements the generated gRPC server contract.
type grpcServer struct {
	idl.UnimplementedUserServiceServer
	getuser grpcserver.Handler
	createuser grpcserver.Handler
}


func (s *grpcServer) GetUser(ctx context.Context, req *idl.GetUserRequest) (*idl.GetUserResponse, error) {
	_, resp, err := s.getuser.ServeGRPC(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.(*idl.GetUserResponse), nil
}

func (s *grpcServer) CreateUser(ctx context.Context, req *idl.CreateUserRequest) (*idl.CreateUserResponse, error) {
	_, resp, err := s.createuser.ServeGRPC(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.(*idl.CreateUserResponse), nil
}


func NewGRPCServer(endpoints genendpoint.UserServiceEndpoints) *grpcServer {
	return &grpcServer{
		getuser: grpcserver.NewServer(
			endpoints.GetUserEndpoint,
			decodeGRPCGetUserRequest,
			encodeGRPCGetUserResponse,
		),
		createuser: grpcserver.NewServer(
			endpoints.CreateUserEndpoint,
			decodeGRPCCreateUserRequest,
			encodeGRPCCreateUserResponse,
		),
	}
}

func RegisterGRPCServer(s *grpc.Server, endpoints genendpoint.UserServiceEndpoints) {
	idl.RegisterUserServiceServer(s, NewGRPCServer(endpoints))
}


func NewGRPCGetUserClient(conn *grpc.ClientConn, options ...grpcclient.ClientOption) endpoint.Endpoint {
	return grpcclient.NewClient(
		conn,
		"userservice.UserService",
		"GetUser",
		EncodeGRPCGetUserRequest,
		DecodeGRPCGetUserResponse,
		&idl.GetUserResponse{},
		options...,
	).Endpoint()
}

func NewGRPCCreateUserClient(conn *grpc.ClientConn, options ...grpcclient.ClientOption) endpoint.Endpoint {
	return grpcclient.NewClient(
		conn,
		"userservice.UserService",
		"CreateUser",
		EncodeGRPCCreateUserRequest,
		DecodeGRPCCreateUserResponse,
		&idl.CreateUserResponse{},
		options...,
	).Endpoint()
}



func decodeGRPCGetUserRequest(_ context.Context, grpcReq any) (any, error) {
	req := grpcReq.(*idl.GetUserRequest)
	return *req, nil
}

func encodeGRPCGetUserResponse(_ context.Context, response any) (any, error) {
	resp := response.(idl.GetUserResponse)
	return &resp, nil
}

func EncodeGRPCGetUserRequest(_ context.Context, request any) (any, error) {
	req := request.(idl.GetUserRequest)
	return &req, nil
}

func DecodeGRPCGetUserResponse(_ context.Context, grpcReply any) (any, error) {
	resp := grpcReply.(*idl.GetUserResponse)
	return *resp, nil
}

func decodeGRPCCreateUserRequest(_ context.Context, grpcReq any) (any, error) {
	req := grpcReq.(*idl.CreateUserRequest)
	return *req, nil
}

func encodeGRPCCreateUserResponse(_ context.Context, response any) (any, error) {
	resp := response.(idl.CreateUserResponse)
	return &resp, nil
}

func EncodeGRPCCreateUserRequest(_ context.Context, request any) (any, error) {
	req := request.(idl.CreateUserRequest)
	return &req, nil
}

func DecodeGRPCCreateUserResponse(_ context.Context, grpcReply any) (any, error) {
	resp := grpcReply.(*idl.CreateUserResponse)
	return *resp, nil
}

