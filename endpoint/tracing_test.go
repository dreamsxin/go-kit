package endpoint_test

import (
	"context"
	"testing"

	"github.com/dreamsxin/go-kit/endpoint"
)

func TestTracingMiddleware_GeneratesIDs(t *testing.T) {
	var gotTrace endpoint.TraceID
	var gotReq string

	ep := endpoint.TracingMiddleware()(func(ctx context.Context, _ any) (any, error) {
		gotTrace = endpoint.TraceIDFromContext(ctx)
		gotReq = endpoint.RequestIDFromContext(ctx)
		return nil, nil
	})

	ep(context.Background(), nil) //nolint:errcheck

	if gotTrace == "" {
		t.Error("trace ID should be generated")
	}
	if gotReq == "" {
		t.Error("request ID should be generated")
	}
}

func TestTracingMiddleware_PreservesExistingTraceID(t *testing.T) {
	const existing = endpoint.TraceID("my-trace-123")
	ctx := endpoint.WithTraceID(context.Background(), existing)

	var got endpoint.TraceID
	ep := endpoint.TracingMiddleware()(func(ctx context.Context, _ any) (any, error) {
		got = endpoint.TraceIDFromContext(ctx)
		return nil, nil
	})

	ep(ctx, nil) //nolint:errcheck

	if got != existing {
		t.Errorf("trace ID: want %q, got %q", existing, got)
	}
}

func TestWithTraceID_RoundTrip(t *testing.T) {
	ctx := endpoint.WithTraceID(context.Background(), "abc")
	if got := endpoint.TraceIDFromContext(ctx); got != "abc" {
		t.Errorf("want %q, got %q", "abc", got)
	}
}

func TestWithRequestID_RoundTrip(t *testing.T) {
	ctx := endpoint.WithRequestID(context.Background(), "req-1")
	if got := endpoint.RequestIDFromContext(ctx); got != "req-1" {
		t.Errorf("want %q, got %q", "req-1", got)
	}
}

func TestBuilder_WithTracing(t *testing.T) {
	var gotTrace endpoint.TraceID
	base := endpoint.Endpoint(func(ctx context.Context, _ any) (any, error) {
		gotTrace = endpoint.TraceIDFromContext(ctx)
		return nil, nil
	})

	ep := endpoint.NewBuilder(base).WithTracing().Build()
	ep(context.Background(), nil) //nolint:errcheck

	if gotTrace == "" {
		t.Error("Builder.WithTracing should inject trace ID")
	}
}

func TestBuilder_WithBackpressure(t *testing.T) {
	ep := endpoint.NewBuilder(endpoint.Nop).WithBackpressure(1).Build()
	// First call should succeed
	if _, err := ep(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
