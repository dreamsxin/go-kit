package oteladapter

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
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
