package test

import (
	"context"

	"github.com/dreamsxin/go-kit/examples/transport/_grpc_test/pb"
)

func EncodeRequest(ctx context.Context, req interface{}) (interface{}, error) {
	r := req.(TestRequest)
	return &pb.TestRequest{A: r.A, B: r.B}, nil
}

func DecodeRequest(ctx context.Context, req interface{}) (interface{}, error) {
	r := req.(*pb.TestRequest)
	return TestRequest{A: r.A, B: r.B}, nil
}

func EncodeResponse(ctx context.Context, resp interface{}) (interface{}, error) {
	r := resp.(*TestResponse)
	return &pb.TestResponse{V: r.V}, nil
}

func DecodeResponse(ctx context.Context, resp interface{}) (interface{}, error) {
	r := resp.(*pb.TestResponse)
	return &TestResponse{V: r.V, Ctx: ctx}, nil
}
