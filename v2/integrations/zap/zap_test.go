package zapadapter

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap/zaptest/observer"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"go.uber.org/zap"
)

func TestLoggingMiddlewareSuccess(t *testing.T) {
	middleware := LoggingMiddleware(zap.NewNop(), "testOp")
	response, err := middleware(func(context.Context, any) (any, error) {
		return "ok", nil
	})(context.Background(), nil)
	if err != nil || response != "ok" {
		t.Fatalf("response=%v err=%v", response, err)
	}
}

func TestLoggingMiddlewareError(t *testing.T) {
	sentinel := errors.New("failed")
	middleware := LoggingMiddleware(zap.NewNop(), "testOp")
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

func TestLoggingMiddlewareRecordsCorrelationAndLevel(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	middleware := LoggingMiddleware(logger, "testOp")

	ctx := endpoint.WithRequestID(
		endpoint.ContinueIncomingTrace(
			endpoint.WithIncomingTraceContext(context.Background(), endpoint.TraceContext{
				TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
				SpanID:  "00f067aa0ba902b7",
				Flags:   "01",
			})),
		"req-1",
	)

	failure := errors.New("boom")
	if _, err := middleware(func(context.Context, any) (any, error) {
		return nil, failure
	})(ctx, nil); !errors.Is(err, failure) {
		t.Fatalf("error = %v", err)
	}

	entries := observed.FilterMessage("endpoint call failed").AllUntimed()
	if len(entries) != 1 {
		t.Fatalf("failure records = %d, want 1", len(entries))
	}
	if entries[0].Level != zap.ErrorLevel {
		t.Fatalf("level = %v, want Error", entries[0].Level)
	}
	fields := entries[0].ContextMap()
	for key, want := range map[string]string{
		"operation":  "testOp",
		"trace_id":   "4bf92f3577b34da6a3ce929d0e0e4736",
		"request_id": "req-1",
	} {
		if got, _ := fields[key].(string); got != want {
			t.Errorf("%s = %v, want %q", key, fields[key], want)
		}
	}
	// spanOwned is set by ContinueIncomingTrace, so the child span is reported.
	if span, _ := fields["span_id"].(string); span == "" || span == "00f067aa0ba902b7" {
		t.Errorf("span_id = %v, want the minted child span", fields["span_id"])
	}
	if got, _ := fields["success"].(bool); got {
		t.Errorf("success = %v, want false", fields["success"])
	}
}
