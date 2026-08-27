package slogadapter

import (
	"context"
	"log/slog"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

func TestNewTelemetryRejectsEmptyOperation(t *testing.T) {
	if _, err := NewTelemetry(TelemetryConfig{}); err == nil {
		t.Fatal("expected empty operation error")
	}
}

func TestTelemetryChainCorrelatesLogsMetricsAndTrace(t *testing.T) {
	handler := &captureHandler{}
	logger := slog.New(handler)

	telemetry, err := NewTelemetry(TelemetryConfig{
		Operation: "hello",
		Logger:    logger,
	})
	if err != nil {
		t.Fatalf("NewTelemetry: %v", err)
	}

	var seenTraceID endpoint.TraceID
	var seenRequestID string
	base := endpoint.Endpoint(func(ctx context.Context, _ any) (any, error) {
		seenTraceID = endpoint.TraceIDFromContext(ctx)
		seenRequestID = endpoint.RequestIDFromContext(ctx)
		return "ok", nil
	})
	ep := telemetry.Apply(endpoint.NewBuilder(base)).Build()

	if _, err := ep(context.Background(), nil); err != nil {
		t.Fatalf("endpoint: %v", err)
	}

	if seenTraceID == "" || seenRequestID == "" {
		t.Fatalf("correlation IDs = %q/%q, want both set", seenTraceID, seenRequestID)
	}

	snapshot := telemetry.Metrics.Snapshot()
	if snapshot.RequestCount != 1 || snapshot.SuccessCount != 1 {
		t.Fatalf("metrics snapshot = %+v, want one successful request", snapshot)
	}

	attrs := recordAttrs(handler.latest())
	if attrs["operation"] != "hello" || attrs["success"] != true {
		t.Fatalf("log attrs = %#v", attrs)
	}
	if attrs["trace_id"] != string(seenTraceID) {
		t.Fatalf("log trace_id = %v, want %q", attrs["trace_id"], seenTraceID)
	}
}

func TestTelemetryApplyLabels(t *testing.T) {
	telemetry, err := NewTelemetry(TelemetryConfig{Operation: "op"})
	if err != nil {
		t.Fatalf("NewTelemetry: %v", err)
	}
	builder := endpoint.NewBuilder(endpoint.Nop)
	telemetry.Apply(builder)

	labels := builder.Describe()
	want := []string{"telemetry:tracing", "telemetry:metrics", "telemetry:logging"}
	if len(labels) != len(want) {
		t.Fatalf("labels = %v, want %v", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("labels = %v, want %v", labels, want)
		}
	}
}
