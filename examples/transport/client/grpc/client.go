package grpc

import (
	"context"

	"google.golang.org/grpc"

	"github.com/dreamsxin/go-kit/endpoint"
	test "github.com/dreamsxin/go-kit/examples/transport/_grpc_test"
	"github.com/dreamsxin/go-kit/examples/transport/_grpc_test/pb"
	grpctransport "github.com/dreamsxin/go-kit/transport/grpc/client"
)

// 绑定多个端点
type clientBinding struct {
	test endpoint.Endpoint
}

func (c *clientBinding) Test(ctx context.Context, a string, b int64) (context.Context, string, error) {
	response, err := c.test(ctx, test.TestRequest{A: a, B: b})
	if err != nil {
		return nil, "", err
	}
	r := response.(*test.TestResponse)
	return r.Ctx, r.V, nil
}

func NewClient(cc *grpc.ClientConn) test.Service {
	return &clientBinding{
		test: grpctransport.NewClient(
			cc,
			"pb.Test",
			"Test",
			test.EncodeRequest,
			test.DecodeResponse,
			&pb.TestResponse{},
			grpctransport.ClientBefore(
				test.InjectCorrelationID,
			),
			grpctransport.ClientBefore(
				test.DisplayClientRequestHeaders,
			),
			grpctransport.ClientAfter(
				test.DisplayClientResponseHeaders,
				test.DisplayClientResponseTrailers,
			),
			grpctransport.ClientAfter(
				test.ExtractConsumedCorrelationID,
			),
		).Endpoint(),
	}
}
