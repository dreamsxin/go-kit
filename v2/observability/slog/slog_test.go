package slogadapter

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, record.Clone())
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func (h *captureHandler) latest() slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.records[len(h.records)-1]
}

func recordAttrs(record slog.Record) map[string]any {
	attrs := make(map[string]any)
	record.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.Any()
		return true
	})
	return attrs
}

func TestLoggingMiddlewareRecordsBoundedContext(t *testing.T) {
	handler := &captureHandler{}
	logger := slog.New(handler)
	wantErr := errors.New("boom")
	middleware := LoggingMiddleware(logger, "GetUser", WithAttrs(func(context.Context) []slog.Attr {
		return []slog.Attr{slog.String("component", "test")}
	}))
	endpointFn := middleware(func(ctx context.Context, _ any) (any, error) {
		if endpoint.TraceIDFromContext(ctx) != "trace-1" || endpoint.RequestIDFromContext(ctx) != "request-1" {
			t.Fatal("correlation IDs were not preserved")
		}
		return nil, wantErr
	})

	ctx := endpoint.WithRequestID(endpoint.WithTraceID(context.Background(), "trace-1"), "request-1")
	if _, err := endpointFn(ctx, struct{}{}); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}

	record := handler.latest()
	if record.Message != "endpoint call failed" {
		t.Fatalf("message = %q", record.Message)
	}
	attrs := recordAttrs(record)
	if attrs["operation"] != "GetUser" || attrs["success"] != false || attrs["component"] != "test" {
		t.Fatalf("attrs = %#v", attrs)
	}
	if attrs["trace_id"] != "trace-1" || attrs["request_id"] != "request-1" {
		t.Fatalf("correlation attrs = %#v", attrs)
	}
	if attrs["error"] != wantErr {
		t.Fatalf("error attr = %#v", attrs["error"])
	}
}

func TestLoggingMiddlewareUsesDefaultLoggerWhenNil(t *testing.T) {
	middleware := LoggingMiddleware(nil, "Health", WithLevel(slog.LevelDebug))
	if _, err := middleware(func(context.Context, any) (any, error) { return "ok", nil })(context.Background(), nil); err != nil {
		t.Fatalf("error = %v", err)
	}
}
