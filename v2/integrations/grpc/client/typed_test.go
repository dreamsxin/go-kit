package client

import (
	"context"
	"net"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"

	transportgrpc "github.com/dreamsxin/go-kit/v2/integrations/grpc"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// The typed client tests speak to a hand-registered service the same way the
// untyped client's tests do: Ping answers an empty message and records the
// incoming traceparent; Missing answers NotFound.
type typedFixture struct {
	conn        *grpc.ClientConn
	traceparent string
}

type bufconnListener = bufconn.Listener

func newTypedFixture(t *testing.T) *typedFixture {
	t.Helper()
	fixture := &typedFixture{}

	lis := bufconnListen(t)
	srv := grpc.NewServer()
	srv.RegisterService(&grpc.ServiceDesc{
		ServiceName: "test.TypedService",
		HandlerType: (*interface{})(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "Ping",
				Handler: func(_ interface{}, ctx context.Context, dec func(interface{}) error, _ grpc.UnaryServerInterceptor) (interface{}, error) {
					req := &emptypb.Empty{}
					if err := dec(req); err != nil {
						return nil, err
					}
					if md, ok := metadata.FromIncomingContext(ctx); ok {
						if values := md.Get("traceparent"); len(values) > 0 {
							fixture.traceparent = values[0]
						}
					}
					return &emptypb.Empty{}, nil
				},
			},
			{
				MethodName: "Missing",
				Handler: func(_ interface{}, ctx context.Context, dec func(interface{}) error, _ grpc.UnaryServerInterceptor) (interface{}, error) {
					return nil, status.Error(codes.NotFound, "no such key")
				},
			},
		},
	}, struct{}{})
	go srv.Serve(lis) //nolint:errcheck
	t.Cleanup(srv.Stop)

	conn, err := grpc.DialContext( //nolint:staticcheck
		context.Background(),
		"bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	fixture.conn = conn
	return fixture
}

func bufconnListen(t *testing.T) *bufconnListener {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	t.Cleanup(func() { _ = lis.Close() })
	return lis
}

func TestTypedClientRoundTrip(t *testing.T) {
	fixture := newTypedFixture(t)
	client := NewTypedClient[*emptypb.Empty, *emptypb.Empty](
		fixture.conn, "test.TypedService", "Ping",
	)

	response, err := client.Invoke(context.Background(), &emptypb.Empty{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if response == nil {
		t.Fatal("Invoke returned a nil Resp")
	}
}

func TestTypedClientClassifiesTheStatus(t *testing.T) {
	fixture := newTypedFixture(t)
	client := NewTypedClient[*emptypb.Empty, *emptypb.Empty](
		fixture.conn, "test.TypedService", "Missing",
	)

	_, err := client.Invoke(context.Background(), &emptypb.Empty{})
	if err == nil {
		t.Fatal("Invoke returned no error for a NotFound")
	}
	classified, ok := err.(*transportgrpc.GRPCStatusError)
	if !ok {
		t.Fatalf("error = %T, want the classified wrapper", err)
	}
	if classified.ErrorKindName() != "not_found" {
		t.Fatalf("kind = %q, want not_found", classified.ErrorKindName())
	}
}

func TestTypedClientInjectsTraceparent(t *testing.T) {
	fixture := newTypedFixture(t)
	client := NewTypedClient[*emptypb.Empty, *emptypb.Empty](
		fixture.conn, "test.TypedService", "Ping",
	)

	traced := endpoint.WithTraceContext(context.Background(), endpoint.TraceContext{
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:  "00f067aa0ba902b7",
		Flags:   "01",
	})
	if _, err := client.Invoke(traced, &emptypb.Empty{}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.HasPrefix(fixture.traceparent, "00-") {
		t.Fatalf("server saw traceparent %q, want the injected context", fixture.traceparent)
	}
}
