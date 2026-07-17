package server

import (
	"context"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	kitlog "github.com/dreamsxin/go-kit/v2/log"
	transportgrpc "github.com/dreamsxin/go-kit/v2/transport/grpc"
	"google.golang.org/grpc"
)

func TestInterceptor_InjectsMethodName(t *testing.T) {
	var capturedMethod string
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		capturedMethod, _ = ctx.Value(transportgrpc.ContextKeyRequestMethod).(string)
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/DoThing"}
	resp, err := Interceptor(context.Background(), nil, info, handler)
	if err != nil {
		t.Fatalf("Interceptor: %v", err)
	}
	if resp != "ok" {
		t.Errorf("resp: got %v, want ok", resp)
	}
	if capturedMethod != "/test.Service/DoThing" {
		t.Errorf("method: got %q, want /test.Service/DoThing", capturedMethod)
	}
}

func TestInterceptor_PropagatesHandlerError(t *testing.T) {
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, grpc.ErrServerStopped
	}
	info := &grpc.UnaryServerInfo{FullMethod: "/svc/M"}
	_, err := Interceptor(context.Background(), nil, info, handler)
	if err != grpc.ErrServerStopped {
		t.Errorf("err: got %v, want %v", err, grpc.ErrServerStopped)
	}
}

func TestServerErrorLogger_SetsHandler(t *testing.T) {
	logger := kitlog.NewNopLogger()
	s := NewServer(stubEndpoint, stubDec, stubEnc, ServerErrorLogger(logger))
	if s.errorHandler == nil {
		t.Error("expected errorHandler to be set")
	}
}

func TestServerErrorHandler_SetsHandler(t *testing.T) {
	called := false
	h := &mockErrorHandler{handle: func(ctx context.Context, err error) { called = true }}
	s := NewServer(stubEndpoint, stubDec, stubEnc, ServerErrorHandler(h))

	s.errorHandler.Handle(context.Background(), nil)
	if !called {
		t.Error("expected error handler to be called")
	}
}

func TestServerFinalizer_SkipsNil(t *testing.T) {
	s := NewServer(stubEndpoint, stubDec, stubEnc, ServerFinalizer(nil, nil))
	if len(s.finalizer) != 0 {
		t.Errorf("expected 0 finalizers after nil-only, got %d", len(s.finalizer))
	}
}

func TestServerBefore_SkipsNil(t *testing.T) {
	s := NewServer(stubEndpoint, stubDec, stubEnc, ServerBefore(nil))
	if len(s.before) != 0 {
		t.Errorf("expected 0 before hooks after nil-only, got %d", len(s.before))
	}
}

func TestServerAfter_SkipsNil(t *testing.T) {
	s := NewServer(stubEndpoint, stubDec, stubEnc, ServerAfter(nil))
	if len(s.after) != 0 {
		t.Errorf("expected 0 after hooks after nil-only, got %d", len(s.after))
	}
}

// ─── test helpers ────────────────────────────────────────────────────────────

type mockErrorHandler struct {
	handle func(ctx context.Context, err error)
}

func (m *mockErrorHandler) Handle(ctx context.Context, err error) {
	m.handle(ctx, err)
}

// Stub values to satisfy NewServer's nil-checks.
var (
	stubEndpoint = endpoint.Endpoint(func(_ context.Context, _ any) (any, error) { return nil, nil })
	stubDec      = func(_ context.Context, _ interface{}) (interface{}, error) { return nil, nil }
	stubEnc      = func(_ context.Context, _ interface{}) (interface{}, error) { return nil, nil }
)
