package grpc_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/test/bufconn"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/endpoint"
	grpcclient "github.com/dreamsxin/go-kit/v2/integrations/grpc/client"
	grpcserver "github.com/dreamsxin/go-kit/v2/integrations/grpc/server"
	"github.com/dreamsxin/go-kit/v2/transport"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// jsonCodec registers a JSON codec so the test can run without protoc output.
type jsonCodec struct{}

func (jsonCodec) Marshal(v any) ([]byte, error)      { return json.Marshal(v) }
func (jsonCodec) Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
func (jsonCodec) Name() string                       { return "json" }

func init() {
	encoding.RegisterCodec(jsonCodec{})
}

// pbRequest/pbReply stand in for generated protobuf messages; the JSON codec
// carries them over the wire.
type pbRequest struct {
	Name string `json:"name"`
}
type pbReply struct {
	Message string `json:"message"`
}

type helloRequest struct{ Name string }
type helloResponse struct{ Message string }

func sayHello(_ context.Context, req helloRequest) (helloResponse, error) {
	if req.Name == "" {
		return helloResponse{}, apperror.New(apperror.KindInvalidArgument, "hello.name_required", "name is required")
	}
	return helloResponse{Message: "Hello, " + req.Name + "!"}, nil
}

// TestDualProtocolBinding proves one Binding serves HTTP and gRPC without
// duplicated business logic: the domain types and endpoint are defined once,
// each protocol owns its wire codec.
func TestDualProtocolBinding(t *testing.T) {
	binding := transport.Binding[helloRequest, helloResponse]{
		Endpoint: endpoint.TypedEndpoint[helloRequest, helloResponse](sayHello).Wrap(),
	}

	// HTTP side: typed JSON through the server transport.
	httpHandler := httpserver.NewTypedJSONServer(binding.TypedEndpoint())
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpHandler.ServeHTTP(w, r)
	}))
	defer httpSrv.Close()

	resp, err := http.Post(httpSrv.URL, "application/json", strings.NewReader(`{"Name":"dual"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HTTP status: got %d, want 200", resp.StatusCode)
	}
	var httpBody helloResponse
	if err := json.NewDecoder(resp.Body).Decode(&httpBody); err != nil {
		t.Fatal(err)
	}
	if httpBody.Message != "Hello, dual!" {
		t.Errorf("HTTP response: got %+v", httpBody)
	}

	// gRPC side: the same binding through the gRPC transport over bufconn.
	grpcHandler := grpcserver.NewServer(
		binding.Endpoint,
		func(_ context.Context, req any) (any, error) {
			m := req.(*pbRequest)
			return helloRequest{Name: m.Name}, nil
		},
		func(_ context.Context, resp any) (any, error) {
			r := resp.(helloResponse)
			return &pbReply{Message: r.Message}, nil
		},
	)

	listener := bufconn.Listen(1024 * 1024)
	grpcSrv := grpc.NewServer()
	grpcSrv.RegisterService(&grpc.ServiceDesc{
		ServiceName: "hello.Greeter",
		HandlerType: (*greeterService)(nil),
		Methods: []grpc.MethodDesc{
			{MethodName: "SayHello", Handler: greeterHandler(grpcHandler)},
		},
	}, &greeterBinding{handler: grpcHandler})
	go func() { _ = grpcSrv.Serve(listener) }()
	defer grpcSrv.Stop()

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(jsonCodec{})),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	call := grpcclient.NewClient(
		conn,
		"hello.Greeter",
		"SayHello",
		func(_ context.Context, req any) (any, error) {
			r := req.(helloRequest)
			return &pbRequest{Name: r.Name}, nil
		},
		func(_ context.Context, resp any) (any, error) {
			r := resp.(*pbReply)
			return helloResponse{Message: r.Message}, nil
		},
		&pbReply{},
	).Endpoint()

	out, err := call(context.Background(), helloRequest{Name: "dual"})
	if err != nil {
		t.Fatal(err)
	}
	if got := out.(helloResponse).Message; got != "Hello, dual!" {
		t.Errorf("gRPC response: got %q", got)
	}
}

// greeterBinding wires the transport handler into the generated-style
// registration shape.
type greeterBinding struct {
	handler grpcserver.Handler
}

type greeterService interface {
	SayHello(context.Context, *pbRequest) (*pbReply, error)
}

func greeterHandler(h grpcserver.Handler) grpc.MethodHandler {
	return func(_ any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
		req := &pbRequest{}
		if err := dec(req); err != nil {
			return nil, err
		}
		if interceptor != nil {
			return interceptor(ctx, req, &grpc.UnaryServerInfo{FullMethod: "/hello.Greeter/SayHello"},
				func(ctx context.Context, r any) (any, error) {
					_, resp, err := h.ServeGRPC(ctx, r)
					return resp, err
				})
		}
		_, resp, err := h.ServeGRPC(ctx, req)
		return resp, err
	}
}

func (b *greeterBinding) SayHello(ctx context.Context, req *pbRequest) (*pbReply, error) {
	_, resp, err := b.handler.ServeGRPC(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.(*pbReply), nil
}
