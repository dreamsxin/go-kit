package server

import (
	"context"
	"errors"
	"testing"

	"github.com/dreamsxin/go-kit/endpoint"
)

func TestNewServer_PanicsOnNilEssentialParameters(t *testing.T) {
	tests := []struct {
		name string
		e    endpoint.Endpoint
		dec  DecodeRequestFunc
		enc  EncodeResponseFunc
	}{
		{
			name: "nil endpoint",
			dec:  func(context.Context, interface{}) (interface{}, error) { return nil, nil },
			enc:  func(context.Context, interface{}) (interface{}, error) { return nil, nil },
		},
		{
			name: "nil decoder",
			e:    func(context.Context, any) (any, error) { return nil, nil },
			enc:  func(context.Context, interface{}) (interface{}, error) { return nil, nil },
		},
		{
			name: "nil encoder",
			e:    func(context.Context, any) (any, error) { return nil, nil },
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
			NewServer(tt.e, tt.dec, tt.enc)
		})
	}
}

func TestServeGRPC_DecodeError_DoesNotPanicWithoutExplicitErrorHandler(t *testing.T) {
	s := NewServer(
		func(context.Context, any) (any, error) { return nil, nil },
		func(context.Context, interface{}) (interface{}, error) { return nil, errors.New("decode failed") },
		func(context.Context, interface{}) (interface{}, error) { return nil, nil },
	)

	_, _, err := s.ServeGRPC(context.Background(), struct{}{})
	if err == nil || err.Error() != "decode failed" {
		t.Fatalf("ServeGRPC() error = %v, want decode failed", err)
	}
}

func TestServeGRPC_EndpointError_DoesNotPanicWithoutExplicitErrorHandler(t *testing.T) {
	s := NewServer(
		func(context.Context, any) (any, error) { return nil, errors.New("endpoint failed") },
		func(context.Context, interface{}) (interface{}, error) { return "req", nil },
		func(context.Context, interface{}) (interface{}, error) { return nil, nil },
	)

	_, _, err := s.ServeGRPC(context.Background(), struct{}{})
	if err == nil || err.Error() != "endpoint failed" {
		t.Fatalf("ServeGRPC() error = %v, want endpoint failed", err)
	}
}

func TestServeGRPC_EndpointError_DoesNotPanicWithNilErrorHandlerOption(t *testing.T) {
	s := NewServer(
		func(context.Context, any) (any, error) { return nil, errors.New("endpoint failed") },
		func(context.Context, interface{}) (interface{}, error) { return "req", nil },
		func(context.Context, interface{}) (interface{}, error) { return nil, nil },
		ServerErrorHandler(nil),
	)

	_, _, err := s.ServeGRPC(context.Background(), struct{}{})
	if err == nil || err.Error() != "endpoint failed" {
		t.Fatalf("ServeGRPC() error = %v, want endpoint failed", err)
	}
}

func TestServeGRPC_NilHooks_DoNotPanicAtRequestTime(t *testing.T) {
	s := NewServer(
		func(context.Context, any) (any, error) { return "ok", nil },
		func(context.Context, interface{}) (interface{}, error) { return "req", nil },
		func(context.Context, interface{}) (interface{}, error) { return "resp", nil },
		ServerBefore(nil),
		ServerAfter(nil),
		ServerFinalizer(nil),
	)

	_, resp, err := s.ServeGRPC(context.Background(), struct{}{})
	if err != nil {
		t.Fatalf("ServeGRPC() error = %v", err)
	}
	if resp != "resp" {
		t.Fatalf("ServeGRPC() response = %v, want resp", resp)
	}
}
