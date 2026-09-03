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

// A response carrying its own error is a failure here too: the gRPC server must
// encode it as a status instead of shipping a half-filled message.
func TestServeGRPC_FailerResponseIsEncodedAsAStatus(t *testing.T) {
	s := NewServer(
		func(context.Context, any) (any, error) {
			return grpcFailerResponse{err: classifiedError{kind: "not_found", message: "no such user"}}, nil
		},
		func(context.Context, interface{}) (interface{}, error) { return nil, nil },
		func(context.Context, interface{}) (interface{}, error) {
			t.Fatal("response encoder must not run for a failed response")
			return nil, nil
		},
	)

	_, _, err := s.ServeGRPC(context.Background(), nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("not a status error: %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("code = %v, want NotFound", st.Code())
	}
	if st.Message() != "no such user" {
		t.Errorf("message = %q, want the public message", st.Message())
	}
}

type grpcFailerResponse struct {
	err error
}

func (r grpcFailerResponse) Failed() error { return r.err }

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

type customKindError struct{ kind string }

func (e customKindError) Error() string         { return e.kind }
func (e customKindError) ErrorKindName() string { return e.kind }
func (e customKindError) PublicMessage() string { return "public: " + e.kind }

func TestErrorEncoderWithKindMapper(t *testing.T) {
	encoder := ErrorEncoderWithKindMapper(func(k string) codes.Code {
		if k == "payment_failed" {
			return codes.FailedPrecondition
		}
		return codes.Code(99)
	})

	err := encoder(context.Background(), customKindError{kind: "payment_failed"})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("want status error, got %v", err)
	}
	if st.Code() != codes.FailedPrecondition {
		t.Errorf("code: got %v, want FailedPrecondition", st.Code())
	}
	if st.Message() != "public: payment_failed" {
		t.Errorf("message: got %q", st.Message())
	}

	// Unknown kinds fall back to the built-in mapping.
	err = encoder(context.Background(), customKindError{kind: "not_found"})
	st, _ = status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("built-in kind: got %v, want NotFound", st.Code())
	}

	// Unclassified errors stay internal.
	err = encoder(context.Background(), errors.New("plain"))
	st, _ = status.FromError(err)
	if st.Code() != codes.Internal {
		t.Errorf("unclassified: got %v, want Internal", st.Code())
	}
}

func TestErrorEncoderWithKindMapper_Nil(t *testing.T) {
	encoder := ErrorEncoderWithKindMapper(nil)
	err := encoder(context.Background(), customKindError{kind: "not_found"})
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("nil mapper should fall back, got %v", st.Code())
	}
}
