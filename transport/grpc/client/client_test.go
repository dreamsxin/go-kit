package client

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"

	transportgrpc "github.com/dreamsxin/go-kit/transport/grpc"
)

func TestNewClient_PanicsOnNilEssentialParameters(t *testing.T) {
	tests := []struct {
		name      string
		cc        *grpc.ClientConn
		enc       EncodeRequestFunc
		dec       DecodeResponseFunc
		grpcReply any
	}{
		{
			name:      "nil connection",
			enc:       func(context.Context, interface{}) (interface{}, error) { return nil, nil },
			dec:       func(context.Context, interface{}) (interface{}, error) { return nil, nil },
			grpcReply: &struct{}{},
		},
		{
			name:      "nil encoder",
			cc:        &grpc.ClientConn{},
			dec:       func(context.Context, interface{}) (interface{}, error) { return nil, nil },
			grpcReply: &struct{}{},
		},
		{
			name:      "nil decoder",
			cc:        &grpc.ClientConn{},
			enc:       func(context.Context, interface{}) (interface{}, error) { return nil, nil },
			grpcReply: &struct{}{},
		},
		{
			name: "nil grpc reply",
			cc:   &grpc.ClientConn{},
			enc:  func(context.Context, interface{}) (interface{}, error) { return nil, nil },
			dec:  func(context.Context, interface{}) (interface{}, error) { return nil, nil },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected panic for nil essential parameter")
				}
			}()
			NewClient(tt.cc, "Svc", "Method", tt.enc, tt.dec, tt.grpcReply)
		})
	}
}

func TestEndpoint_PropagatesResponseMetadataIntoContext(t *testing.T) {
	const bufSize = 1 << 20

	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	srv.RegisterService(&grpc.ServiceDesc{
		ServiceName: "test.TestService",
		HandlerType: (*interface{})(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "Ping",
				Handler: func(_ interface{}, ctx context.Context, dec func(interface{}) error, _ grpc.UnaryServerInterceptor) (interface{}, error) {
					var req emptypb.Empty
					if err := dec(&req); err != nil {
						return nil, err
					}
					if err := grpc.SendHeader(ctx, metadata.Pairs("x-test-header", "header-value")); err != nil {
						return nil, err
					}
					if err := grpc.SetTrailer(ctx, metadata.Pairs("x-test-trailer", "trailer-value")); err != nil {
						return nil, err
					}
					return &emptypb.Empty{}, nil
				},
			},
		},
	}, struct{}{})
	go srv.Serve(lis) //nolint:errcheck
	defer srv.Stop()
	defer lis.Close()

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
	defer conn.Close()

	var finalizerHeader, finalizerTrailer metadata.MD
	ep := NewClient(
		conn,
		"test.TestService",
		"Ping",
		func(context.Context, interface{}) (interface{}, error) { return &emptypb.Empty{}, nil },
		func(ctx context.Context, reply interface{}) (interface{}, error) {
			if _, ok := reply.(*emptypb.Empty); !ok {
				t.Fatalf("reply type = %T, want *emptypb.Empty", reply)
			}
			header, ok := ctx.Value(transportgrpc.ContextKeyResponseHeaders).(metadata.MD)
			if !ok {
				t.Fatal("response headers missing from context")
			}
			trailer, ok := ctx.Value(transportgrpc.ContextKeyResponseTrailers).(metadata.MD)
			if !ok {
				t.Fatal("response trailers missing from context")
			}
			if got := header.Get("x-test-header"); len(got) != 1 || got[0] != "header-value" {
				t.Fatalf("header = %v, want header-value", got)
			}
			if got := trailer.Get("x-test-trailer"); len(got) != 1 || got[0] != "trailer-value" {
				t.Fatalf("trailer = %v, want trailer-value", got)
			}
			return "ok", nil
		},
		&emptypb.Empty{},
		ClientFinalizer(func(ctx context.Context, _ error) {
			finalizerHeader, _ = ctx.Value(transportgrpc.ContextKeyResponseHeaders).(metadata.MD)
			finalizerTrailer, _ = ctx.Value(transportgrpc.ContextKeyResponseTrailers).(metadata.MD)
		}),
	).Endpoint()

	resp, err := ep(context.Background(), struct{}{})
	if err != nil {
		t.Fatalf("Endpoint: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("response = %v, want ok", resp)
	}
	if got := finalizerHeader.Get("x-test-header"); len(got) != 1 || got[0] != "header-value" {
		t.Fatalf("finalizer header = %v, want header-value", got)
	}
	if got := finalizerTrailer.Get("x-test-trailer"); len(got) != 1 || got[0] != "trailer-value" {
		t.Fatalf("finalizer trailer = %v, want trailer-value", got)
	}
}

func TestEndpoint_NilHooks_DoNotPanicAtRequestTime(t *testing.T) {
	const bufSize = 1 << 20

	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	srv.RegisterService(&grpc.ServiceDesc{
		ServiceName: "test.TestService",
		HandlerType: (*interface{})(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "Ping",
				Handler: func(_ interface{}, ctx context.Context, dec func(interface{}) error, _ grpc.UnaryServerInterceptor) (interface{}, error) {
					var req emptypb.Empty
					if err := dec(&req); err != nil {
						return nil, err
					}
					return &emptypb.Empty{}, nil
				},
			},
		},
	}, struct{}{})
	go srv.Serve(lis) //nolint:errcheck
	defer srv.Stop()
	defer lis.Close()

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
	defer conn.Close()

	ep := NewClient(
		conn,
		"test.TestService",
		"Ping",
		func(context.Context, interface{}) (interface{}, error) { return &emptypb.Empty{}, nil },
		func(context.Context, interface{}) (interface{}, error) { return "ok", nil },
		&emptypb.Empty{},
		ClientBefore(nil),
		ClientAfter(nil),
		ClientFinalizer(nil),
	).Endpoint()

	resp, err := ep(context.Background(), struct{}{})
	if err != nil {
		t.Fatalf("Endpoint: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("response = %v, want ok", resp)
	}
}
