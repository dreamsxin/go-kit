package server

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

type classifiedError struct {
	kind    string
	message string
}

func (e classifiedError) Error() string         { return e.message }
func (e classifiedError) ErrorKindName() string { return e.kind }
func (e classifiedError) PublicMessage() string { return e.message }

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
	if status.Code(err) != codes.Internal || status.Convert(err).Message() != "internal error" {
		t.Fatalf("ServeGRPC() error = %v, want redacted Internal", err)
	}
}

func TestServeGRPC_EndpointError_DoesNotPanicWithoutExplicitErrorHandler(t *testing.T) {
	s := NewServer(
		func(context.Context, any) (any, error) { return nil, errors.New("endpoint failed") },
		func(context.Context, interface{}) (interface{}, error) { return "req", nil },
		func(context.Context, interface{}) (interface{}, error) { return nil, nil },
	)

	_, _, err := s.ServeGRPC(context.Background(), struct{}{})
	if status.Code(err) != codes.Internal || status.Convert(err).Message() != "internal error" {
		t.Fatalf("ServeGRPC() error = %v, want redacted Internal", err)
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
	if status.Code(err) != codes.Internal || status.Convert(err).Message() != "internal error" {
		t.Fatalf("ServeGRPC() error = %v, want redacted Internal", err)
	}
}

func TestServeGRPC_MapsApplicationError(t *testing.T) {
	s := NewServer(
		func(context.Context, any) (any, error) {
			return nil, classifiedError{kind: "not_found", message: "user not found"}
		},
		func(context.Context, interface{}) (interface{}, error) { return "req", nil },
		func(context.Context, interface{}) (interface{}, error) { return nil, nil },
	)

	_, _, err := s.ServeGRPC(context.Background(), struct{}{})
	if status.Code(err) != codes.NotFound || status.Convert(err).Message() != "user not found" {
		t.Fatalf("ServeGRPC() error = %v, want NotFound", err)
	}
}

func TestServeGRPC_CustomErrorEncoder(t *testing.T) {
	s := NewServer(
		func(context.Context, any) (any, error) { return nil, errors.New("endpoint failed") },
		func(context.Context, interface{}) (interface{}, error) { return "req", nil },
		func(context.Context, interface{}) (interface{}, error) { return nil, nil },
		ServerErrorEncoder(func(_ context.Context, err error) error { return err }),
	)

	_, _, err := s.ServeGRPC(context.Background(), struct{}{})
	if err == nil || err.Error() != "endpoint failed" {
		t.Fatalf("ServeGRPC() error = %v, want original error", err)
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
