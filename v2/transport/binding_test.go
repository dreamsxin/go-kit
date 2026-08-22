package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/transport"
	"github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// fake protobuf message stand-ins: the binding only needs "some wire type".
type pbHelloRequest struct{ Name string }
type pbHelloReply struct{ Message string }

type helloRequest struct{ Name string }
type helloResponse struct{ Message string }

func sayHello(_ context.Context, req helloRequest) (helloResponse, error) {
	if req.Name == "" {
		return helloResponse{}, apperror.New(apperror.KindInvalidArgument, "hello.name_required", "name is required")
	}
	return helloResponse{Message: "Hello, " + req.Name + "!"}, nil
}

func newHelloBinding() transport.Binding[helloRequest, helloResponse] {
	return transport.Binding[helloRequest, helloResponse]{
		Endpoint: endpoint.TypedEndpoint[helloRequest, helloResponse](sayHello).Wrap(),
	}
}

func TestBinding_TypedEndpointServesHTTP(t *testing.T) {
	binding := newHelloBinding()
	h := server.NewTypedJSONServer(binding.TypedEndpoint())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/hello", strings.NewReader(`{"Name":"kit"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	var body helloResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Message != "Hello, kit!" {
		t.Errorf("response: got %+v", body)
	}
}

func TestBinding_ServeGRPCSatisfiesHandlerShape(t *testing.T) {
	binding := newHelloBinding()
	grpcServer := binding.GRPCServer(
		func(_ context.Context, pb any) (helloRequest, error) {
			m := pb.(*pbHelloRequest)
			return helloRequest{Name: m.Name}, nil
		},
		func(_ context.Context, resp helloResponse) (any, error) {
			return &pbHelloReply{Message: resp.Message}, nil
		},
	)

	// The returned value satisfies the integrations/grpc/server.Handler
	// contract shape: ServeGRPC(ctx, any) (context.Context, any, error).
	var handler interface {
		ServeGRPC(context.Context, interface{}) (context.Context, interface{}, error)
	} = grpcServer

	_, resp, err := handler.ServeGRPC(context.Background(), &pbHelloRequest{Name: "kit"})
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.(*pbHelloReply).Message; got != "Hello, kit!" {
		t.Errorf("gRPC response: got %q", got)
	}
}

func TestBinding_GRPCDecodeErrorSurfaces(t *testing.T) {
	binding := newHelloBinding()
	grpcServer := binding.GRPCServer(
		func(_ context.Context, pb any) (helloRequest, error) {
			return helloRequest{}, errors.New("bad protobuf message")
		},
		func(_ context.Context, resp helloResponse) (any, error) {
			return &pbHelloReply{Message: resp.Message}, nil
		},
	)

	_, _, err := grpcServer.ServeGRPC(context.Background(), &pbHelloRequest{})
	if err == nil || err.Error() != "bad protobuf message" {
		t.Fatalf("decode error should surface, got %v", err)
	}
}

func TestBinding_EndpointErrorSurfaces(t *testing.T) {
	binding := newHelloBinding()
	grpcServer := binding.GRPCServer(
		func(_ context.Context, pb any) (helloRequest, error) {
			m := pb.(*pbHelloRequest)
			return helloRequest{Name: m.Name}, nil
		},
		func(_ context.Context, resp helloResponse) (any, error) {
			return &pbHelloReply{Message: resp.Message}, nil
		},
	)

	_, _, err := grpcServer.ServeGRPC(context.Background(), &pbHelloRequest{Name: ""})
	var appErr *apperror.Error
	if !errors.As(err, &appErr) || appErr.ErrorKind() != apperror.KindInvalidArgument {
		t.Fatalf("endpoint apperror should surface, got %v", err)
	}
}
