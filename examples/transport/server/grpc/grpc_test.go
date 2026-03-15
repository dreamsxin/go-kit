// Package grpc tests the gRPC server transport binding.
// It uses google.golang.org/grpc/test/bufconn so no real network port is opened.
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

const bufSize = 1 << 20

func startServer(t *testing.T) (dialer func(context.Context, string) (net.Conn, error), cleanup func()) {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	pb.RegisterTestServer(srv, test.NewBinding(test.NewService()))
	go func() { _ = srv.Serve(lis) }()
	return func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}, func() {
			srv.GracefulStop()
			lis.Close()
		}
}

func dialPb(t *testing.T, dialer func(context.Context, string) (net.Conn, error)) pb.TestClient {
	t.Helper()
	conn, err := grpc.DialContext( //nolint:staticcheck
		context.Background(), "bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return pb.NewTestClient(conn)
}

// TestGRPCServer_BasicCall verifies the server can handle a simple unary RPC.
func TestGRPCServer_BasicCall(t *testing.T) {
	dialer, cleanup := startServer(t)
	defer cleanup()

	client := dialPb(t, dialer)
	resp, err := client.Test(context.Background(), &pb.TestRequest{A: "hello", B: 7})
	if err != nil {
		t.Fatalf("Test RPC: %v", err)
	}
	want := fmt.Sprintf("%s = %d", "hello", 7)
	if resp.V != want {
		t.Errorf("want %q, got %q", want, resp.V)
	}
}

// TestGRPCServer_MultipleCalls verifies the server handles multiple sequential calls.
func TestGRPCServer_MultipleCalls(t *testing.T) {
	dialer, cleanup := startServer(t)
	defer cleanup()

	client := dialPb(t, dialer)

	cases := []struct {
		a    string
		b    int64
		want string
	}{
		{"alpha", 1, "alpha = 1"},
		{"beta", 0, "beta = 0"},
		{"gamma", -5, "gamma = -5"},
	}

	for _, tc := range cases {
		resp, err := client.Test(context.Background(), &pb.TestRequest{A: tc.a, B: tc.b})
		if err != nil {
			t.Errorf("Test(%q, %d): %v", tc.a, tc.b, err)
			continue
		}
		if resp.V != tc.want {
			t.Errorf("Test(%q, %d): want %q, got %q", tc.a, tc.b, tc.want, resp.V)
		}
	}
}

// TestGRPCServer_ConcurrentCalls verifies the server is safe for concurrent requests.
func TestGRPCServer_ConcurrentCalls(t *testing.T) {
	dialer, cleanup := startServer(t)
	defer cleanup()

	client := dialPb(t, dialer)

	const n = 20
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			a := fmt.Sprintf("key%d", i)
			b := int64(i)
			resp, err := client.Test(context.Background(), &pb.TestRequest{A: a, B: b})
			if err != nil {
				errs <- err
				return
			}
			want := fmt.Sprintf("%s = %d", a, b)
			if resp.V != want {
				errs <- fmt.Errorf("want %q, got %q", want, resp.V)
				return
			}
			errs <- nil
		}(i)
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
}
