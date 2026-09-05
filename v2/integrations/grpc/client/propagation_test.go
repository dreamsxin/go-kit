package client

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	transportgrpc "github.com/dreamsxin/go-kit/v2/integrations/grpc"
)

// TestClientInjectsTraceparentByDefault: propagation that has to be wired up is
// propagation that breaks at the first hop somebody forgot.
func TestClientInjectsTraceparentByDefault(t *testing.T) {
	const (
		traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
		spanID  = "00f067aa0ba902b7"
	)
	connection, err := grpc.NewClient("127.0.0.1:0", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("NewClient connection: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })

	client := NewClient(
		connection,
		"UserService", "GetUser",
		func(context.Context, any) (any, error) { return nil, nil },
		func(context.Context, any) (any, error) { return nil, nil },
		&metadata.MD{},
	)

	ctx := endpoint.WithTraceContext(context.Background(), endpoint.TraceContext{
		TraceID: traceID,
		SpanID:  spanID,
		Flags:   "01",
	})
	md := metadata.MD{}
	for _, before := range client.before {
		ctx = before(ctx, &md)
	}

	want := "00-" + traceID + "-" + spanID + "-01"
	if got := md.Get(transportgrpc.TraceparentKey); len(got) != 1 || got[0] != want {
		t.Fatalf("outgoing metadata = %v, want [%s]", got, want)
	}
}
