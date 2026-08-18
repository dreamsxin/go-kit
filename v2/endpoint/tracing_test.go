package endpoint_test

import (
	"context"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
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

func TestParseTraceparent(t *testing.T) {
	valid := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	tc, ok := endpoint.ParseTraceparent(valid)
	if !ok {
		t.Fatalf("valid header rejected: %s", valid)
	}
	if tc.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("trace ID: got %q", tc.TraceID)
	}
	if tc.SpanID != "00f067aa0ba902b7" {
		t.Errorf("span ID: got %q", tc.SpanID)
	}
	if tc.Flags != "01" {
		t.Errorf("flags: got %q", tc.Flags)
	}
	if got := tc.String(); got != valid {
		t.Errorf("String round trip: want %q, got %q", valid, got)
	}

	invalid := []string{
		"",
		"00",
		"00-4bf92f3577b34da6a3ce929d0e0e4736",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01-extra", // extra field
		"ff-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",       // reserved version
		"0-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",        // short version
		"00-4BF92F3577B34DA6A3CE929D0E0E4736-00f067aa0ba902b7-01",       // uppercase
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01",       // zero trace
		"00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01",       // zero span
		"00-4bf92f3577b34da6a3ce929d0e0e47-00f067aa0ba902b7-01",         // short trace
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902-01",         // short span
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-0",        // short flags
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-0x",       // bad flags
		"xx-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",       // bad version
	}
	for _, header := range invalid {
		if _, ok := endpoint.ParseTraceparent(header); ok {
			t.Errorf("invalid header accepted: %q", header)
		}
	}

	// Future versions with a matching shape are accepted per the specification.
	future := "42-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	if _, ok := endpoint.ParseTraceparent(future); !ok {
		t.Errorf("future version rejected: %q", future)
	}
}

func TestNewTraceContextIsW3CConformant(t *testing.T) {
	tc := endpoint.NewTraceContext()
	if !tc.Valid() {
		t.Fatalf("minted context invalid: %+v", tc)
	}
	if tc.TraceID == endpoint.NewTraceContext().TraceID {
		t.Error("trace IDs should be unique")
	}
	if _, ok := endpoint.ParseTraceparent(tc.String()); !ok {
		t.Errorf("String output not parseable: %q", tc.String())
	}
}

func TestTracingMiddleware_JoinsIncomingTraceContext(t *testing.T) {
	incoming := endpoint.TraceContext{
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:  "00f067aa0ba902b7",
		Flags:   "01",
	}
	ctx := endpoint.WithTraceContext(context.Background(), incoming)

	var got endpoint.TraceContext
	var gotTraceID endpoint.TraceID
	ep := endpoint.TracingMiddleware()(func(ctx context.Context, _ any) (any, error) {
		got = endpoint.TraceContextFromContext(ctx)
		gotTraceID = endpoint.TraceIDFromContext(ctx)
		return nil, nil
	})

	ep(ctx, nil) //nolint:errcheck

	if got.TraceID != incoming.TraceID {
		t.Errorf("trace ID: want %q, got %q", incoming.TraceID, got.TraceID)
	}
	if got.SpanID == incoming.SpanID {
		t.Error("span ID should be minted per operation")
	}
	if !got.Valid() {
		t.Errorf("propagated context invalid: %+v", got)
	}
	if got.Flags != "01" {
		t.Errorf("flags should follow the incoming context: got %q", got.Flags)
	}
	if gotTraceID != endpoint.TraceID(incoming.TraceID) {
		t.Errorf("TraceIDFromContext should mirror the trace context: got %q", gotTraceID)
	}
}

func TestTracingMiddleware_BridgesLegacyTraceID(t *testing.T) {
	const legacy = endpoint.TraceID("4bf92f3577b34da6a3ce929d0e0e4736")
	ctx := endpoint.WithTraceID(context.Background(), legacy)

	var got endpoint.TraceContext
	ep := endpoint.TracingMiddleware()(func(ctx context.Context, _ any) (any, error) {
		got = endpoint.TraceContextFromContext(ctx)
		return nil, nil
	})

	ep(ctx, nil) //nolint:errcheck

	if !got.Valid() || got.TraceID != string(legacy) {
		t.Errorf("legacy trace ID should bridge into a trace context: %+v", got)
	}
}
