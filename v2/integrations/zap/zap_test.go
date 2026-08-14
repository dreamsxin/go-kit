package zapadapter

import (
	"context"
	"errors"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/log"
)

func TestLoggingMiddlewareSuccess(t *testing.T) {
	middleware := LoggingMiddleware(log.NewNopLogger(), "testOp")
	response, err := middleware(func(context.Context, any) (any, error) {
		return "ok", nil
	})(context.Background(), nil)
	if err != nil || response != "ok" {
		t.Fatalf("response=%v err=%v", response, err)
	}
}

func TestLoggingMiddlewareError(t *testing.T) {
	sentinel := errors.New("failed")
	middleware := LoggingMiddleware(log.NewNopLogger(), "testOp")
	_, err := middleware(func(context.Context, any) (any, error) {
		return nil, sentinel
	})(context.Background(), nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want sentinel", err)
	}
}

func TestLoggingMiddlewareNilLogger(t *testing.T) {
	if _, err := LoggingMiddleware(nil, "testOp")(endpoint.Nop)(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewErrorHandlerAcceptsNilLogger(t *testing.T) {
	handler := NewErrorHandler(nil)
	if handler == nil {
		t.Fatal("NewErrorHandler returned nil")
	}
	handler.Handle(context.Background(), errors.New("failed"))
}
