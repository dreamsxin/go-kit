package oteladapter

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

func TestTracingMiddlewareUsesApplicationTracer(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	wantErr := errors.New("failed")
	middleware := TracingMiddleware(provider.Tracer("test"), "GetUser", WithSpanAttributes(attribute.String("component", "test")))
	wrapped := middleware(func(ctx context.Context, _ any) (any, error) {
		if span := trace.SpanFromContext(ctx); !span.SpanContext().IsValid() {
			t.Fatal("endpoint did not receive an active span")
		}
		return nil, wantErr
	})
	if _, err := wrapped(context.Background(), nil); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	if spans[0].Name != "GetUser" || spans[0].Status.Code.String() != "Error" {
		t.Fatalf("span = %#v", spans[0])
	}
	if spans[0].Attributes[0].Value.AsString() != "test" {
		t.Fatalf("span attributes = %#v", spans[0].Attributes)
	}
}
func TestMetricsRecordSemanticConventionInstruments(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	metrics, err := NewMetrics(provider.Meter("test"), WithMetricAttributes(attribute.String("component", "users")))
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}
	wrapped := metrics.Middleware("GetUser")(func(context.Context, any) (any, error) {
		return "ok", nil
	})
	if _, err := wrapped(context.Background(), nil); err != nil {
		t.Fatalf("wrapped error = %v", err)
	}
	wantErr := &kindError{kind: "not_found"}
	errorWrapped := metrics.Middleware("GetUser")(func(context.Context, any) (any, error) {
		return nil, wantErr
	})
	if _, err := errorWrapped(context.Background(), nil); !errors.Is(err, wantErr) {
		t.Fatalf("error wrapped error = %v, want %v", err, wantErr)
	}

	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if count := metricDataCount(data, "go_kit.endpoint.requests"); count != 2 {
		t.Fatalf("request count = %d, want 2", count)
	}
	// A duration in milliseconds is not what the conventions ask for, and a
	// backend converting units by name would read it wrong.
	if unit := metricDataUnit(data, "go_kit.endpoint.duration"); unit != "s" {
		t.Fatalf("duration unit = %q, want %q", unit, "s")
	}
	// The failure is the same request series carrying error.type, so error
	// rate is a ratio over one instrument rather than two.
	if got := errorTypeValues(data, "go_kit.endpoint.requests"); len(got) != 1 || got[0] != "not_found" {
		t.Fatalf("error.type values = %v, want [not_found]", got)
	}
}

func TestMetricsReportUnclassifiedErrorsAsOther(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	metrics, err := NewMetrics(provider.Meter("test"))
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}
	wrapped := metrics.Middleware("GetUser")(func(context.Context, any) (any, error) {
		return nil, errors.New("failed")
	})
	if _, err := wrapped(context.Background(), nil); err == nil {
		t.Fatal("wrapped error = nil, want error")
	}

	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got := errorTypeValues(data, "go_kit.endpoint.requests"); len(got) != 1 || got[0] != "_OTHER" {
		t.Fatalf("error.type values = %v, want [_OTHER]", got)
	}
}

type kindError struct {
	kind string
}

func (e *kindError) Error() string { return e.kind }

func (e *kindError) ErrorKindName() string { return e.kind }

func metricDataUnit(data metricdata.ResourceMetrics, name string) string {
	for _, scope := range data.ScopeMetrics {
		for _, item := range scope.Metrics {
			if item.Name == name {
				return item.Unit
			}
		}
	}
	return ""
}

// errorTypeValues returns the error.type attribute of every data point that
// carries one, so a test can assert both the value and its absence.
func errorTypeValues(data metricdata.ResourceMetrics, name string) []string {
	var values []string
	for _, scope := range data.ScopeMetrics {
		for _, item := range scope.Metrics {
			if item.Name != name {
				continue
			}
			sum, ok := item.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, point := range sum.DataPoints {
				if value, found := point.Attributes.Value(attribute.Key("error.type")); found {
					values = append(values, value.AsString())
				}
			}
		}
	}
	return values
}

func metricDataCount(data metricdata.ResourceMetrics, name string) int64 {
	for _, scope := range data.ScopeMetrics {
		for _, item := range scope.Metrics {
			if item.Name != name {
				continue
			}
			if sum, ok := item.Data.(metricdata.Sum[int64]); ok {
				var total int64
				for _, point := range sum.DataPoints {
					total += point.Value
				}
				return total
			}
		}
	}
	return 0
}

// newTraceTestProvider assembles a synchronous in-memory trace pipeline, the
// smallest honest stand-in for a backend: exported spans are the truth a
// collector would have seen.
func newTraceTestProvider(t *testing.T) (*sdktrace.TracerProvider, *tracetest.SpanRecorder) {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	return provider, recorder
}

// TestTracingMiddlewareBridgesTheFrameworkTraceContext proves the trace context
// the framework's transports extracted parents the span: the exported span
// carries the traceparent's trace ID, names the caller's span as its parent,
// and inherits the sampled decision the header carried. Without the bridge the
// first span of every service was a fresh root — a tree the backend never saw.
//
// Stable marker: otel.traceparent-bridged (otel.go).
func TestTracingMiddlewareBridgesTheFrameworkTraceContext(t *testing.T) {
	provider, recorder := newTraceTestProvider(t)
	traceID := "11111111111111111111111111111111"
	parentSpanID := "2222222222222222"

	middleware := TracingMiddleware(provider.Tracer("test"), "GetUser")
	for _, tc := range []struct {
		flags   string
		sampled bool
	}{
		{flags: "01", sampled: true},
		{flags: "00", sampled: false},
	} {
		recorder.Reset()
		ctx := endpoint.WithTraceContext(context.Background(), endpoint.TraceContext{
			TraceID: traceID, SpanID: parentSpanID, Flags: tc.flags,
		})
		_, _ = middleware(func(context.Context, any) (any, error) { return nil, nil })(ctx, nil)

		spans := recorder.Ended()
		if tc.sampled && len(spans) != 1 {
			t.Fatalf("flags %s: spans = %d, want the sampled span", tc.flags, len(spans))
		}
		if !tc.sampled {
			if len(spans) != 0 {
				t.Fatalf("flags %s: spans = %d, want the parent's unsampled decision to hold", tc.flags, len(spans))
			}
			continue
		}
		sc := spans[0].SpanContext()
		if sc.TraceID().String() != traceID {
			t.Errorf("flags %s: trace ID = %s, want the header's %s", tc.flags, sc.TraceID(), traceID)
		}
		if spans[0].Parent().SpanID().String() != parentSpanID {
			t.Errorf("flags %s: parent = %s, want the header's span %s", tc.flags, spans[0].Parent().SpanID(), parentSpanID)
		}
	}
}

// TestTracingMiddlewareInstallsCorrelationFromTheSpan proves the other
// direction: with no framework correlation in the context, the span's identity
// becomes the correlation values the framework reads, so the HTTP client
// propagates this span as the parent and the logging middleware records the
// exported trace's ID.
//
// Stable marker: otel.traceparent-bridged (otel.go).
func TestTracingMiddlewareInstallsCorrelationFromTheSpan(t *testing.T) {
	provider, recorder := newTraceTestProvider(t)

	middleware := TracingMiddleware(provider.Tracer("test"), "GetUser")
	var observedTC endpoint.TraceContext
	var observedTraceID endpoint.TraceID
	_, _ = middleware(func(ctx context.Context, _ any) (any, error) {
		observedTC = endpoint.TraceContextFromContext(ctx)
		observedTraceID = endpoint.TraceIDFromContext(ctx)
		return nil, nil
	})(context.Background(), nil)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	sc := spans[0].SpanContext()
	if observedTC.TraceID != sc.TraceID().String() {
		t.Errorf("correlation trace ID = %s, want the span's %s", observedTC.TraceID, sc.TraceID())
	}
	if observedTC.SpanID != sc.SpanID().String() {
		t.Errorf("correlation span ID = %s, want the span's own %s", observedTC.SpanID, sc.SpanID())
	}
	if observedTC.String() != "00-"+sc.TraceID().String()+"-"+sc.SpanID().String()+"-"+observedTC.Flags {
		t.Errorf("correlation is not a traceparent over the span: %s", observedTC.String())
	}
	if observedTraceID != endpoint.TraceID(sc.TraceID().String()) {
		t.Errorf("logged trace ID = %s, want the span's %s", observedTraceID, sc.TraceID())
	}
}

// TestTracingMiddlewareDefersToAnOtelParent proves the precedence: a span
// already in the context — put there by an OpenTelemetry-native caller — stays
// the parent, and a correlation that arrived from elsewhere is left alone
// rather than replaced by the span's identity.
func TestTracingMiddlewareDefersToAnOtelParent(t *testing.T) {
	provider, recorder := newTraceTestProvider(t)

	parentTraceID := "33333333333333333333333333333333"
	parentSpanID := "4444444444444444"
	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    mustTraceID(t, parentTraceID),
		SpanID:     mustSpanID(t, parentSpanID),
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	correlation := endpoint.TraceContext{
		TraceID: "55555555555555555555555555555555",
		SpanID:  "6666666666666666",
		Flags:   "01",
	}

	middleware := TracingMiddleware(provider.Tracer("test"), "GetUser")
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), parent)
	ctx = endpoint.WithTraceContext(ctx, correlation)
	var observedTC endpoint.TraceContext
	_, _ = middleware(func(ctx context.Context, _ any) (any, error) {
		observedTC = endpoint.TraceContextFromContext(ctx)
		return nil, nil
	})(ctx, nil)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	sc := spans[0].SpanContext()
	if sc.TraceID().String() != parentTraceID || spans[0].Parent().SpanID().String() != parentSpanID {
		t.Errorf("span = trace %s parent %s, want the OpenTelemetry parent's trace %s span %s",
			sc.TraceID(), spans[0].Parent().SpanID(), parentTraceID, parentSpanID)
	}
	if observedTC != correlation {
		t.Errorf("correlation = %v, want the framework's own values left untouched", observedTC)
	}
}

func mustTraceID(t *testing.T, value string) trace.TraceID {
	t.Helper()
	var id trace.TraceID
	if _, err := hex.Decode(id[:], []byte(value)); err != nil {
		t.Fatalf("trace ID %q: %v", value, err)
	}
	return id
}

func mustSpanID(t *testing.T, value string) trace.SpanID {
	t.Helper()
	var id trace.SpanID
	if _, err := hex.Decode(id[:], []byte(value)); err != nil {
		t.Fatalf("span ID %q: %v", value, err)
	}
	return id
}
