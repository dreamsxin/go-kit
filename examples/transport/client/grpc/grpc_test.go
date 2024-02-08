package grpc

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	test "github.com/dreamsxin/go-kit/examples/transport/_grpc_test"
)

const (
	hostPort string = "localhost:8002"
)

func TestGRPCClient(t *testing.T) {

	// 建立 grpc 连接
	cc, err := grpc.Dial(hostPort, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("unable to Dial: %+v", err)
	}

	// 实例化 client
	client := NewClient(cc)

	var (
		a   = "the answer to life the universe and everything"
		b   = int64(42)
		cID = "request-1"
		ctx = test.SetCorrelationID(context.Background(), cID)
	)

	responseCTX, v, err := client.Test(ctx, a, b)
	if err != nil {
		t.Fatalf("unable to Test: %+v", err)
	}
	if want, have := fmt.Sprintf("%s = %d", a, b), v; want != have {
		t.Fatalf("want %q, have %q", want, have)
	}

	if want, have := cID, test.GetConsumedCorrelationID(responseCTX); want != have {
		t.Fatalf("want %q, have %q", want, have)
	}
}
