// Package grpc contains tests for the gRPC client binding.
// The actual end-to-end test (server + client together) lives in
// examples/transport/server/grpc to avoid import cycles.
// Here we only test the NewClient constructor and basic wiring.
package grpc

import (
	"context"
	"fmt"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	test "github.com/dreamsxin/go-kit/examples/transport/_grpc_test"
	"github.com/dreamsxin/go-kit/examples/transport/_grpc_test/pb"
)

func startTestServer(t *testing.T) (dialer func(context.Context, string) (net.Conn, error), cleanup func()) {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	pb.RegisterTestServer(srv, test.NewBinding(test.NewService()))
	go srv.Serve(lis) //nolint:errcheck
	return func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}, func() { srv.GracefulStop(); lis.Close() }
}

// TestNewClient verifies that NewClient correctly wires up the gRPC endpoint
// and can call the remote service.
func TestNewClient(t *testing.T) {
	dialer, cleanup := startTestServer(t)
	defer cleanup()

	conn, err := grpc.DialContext( //nolint:staticcheck
		context.Background(), "bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := NewClient(conn)
	a, b := "answer", int64(42)
	_, v, err := client.Test(context.Background(), a, b)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	want := fmt.Sprintf("%s = %d", a, b)
	if v != want {
		t.Errorf("want %q, got %q", want, v)
	}
}

// TestClientWithCorrelationID verifies that client Before/After hooks
// for correlation-id metadata work correctly.
func TestClientWithCorrelationID(t *testing.T) {
	dialer, cleanup := startTestServer(t)
	defer cleanup()

	conn, err := grpc.DialContext( //nolint:staticcheck
		context.Background(), "bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := NewClient(conn)
	cID := "test-cid-123"
	ctx := test.SetCorrelationID(context.Background(), cID)

	respCtx, _, err := client.Test(ctx, "ping", 0)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if got := test.GetConsumedCorrelationID(respCtx); got != cID {
		t.Errorf("correlation-id: want %q, got %q", cID, got)
	}
}
