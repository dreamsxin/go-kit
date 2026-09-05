package oteladapter

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

func TestSetupInstallsProvidersPropagatorAndResource(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	reader := sdkmetric.NewManualReader()
	providers, err := Setup(context.Background(), Config{
		ServiceName:    "checkout",
		ServiceVersion: "1.4.0",
		Environment:    "staging",
		SpanExporter:   exporter,
		MetricReader:   reader,
	})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() { _ = providers.Shutdown(context.Background()) })

	// One call has to be enough: a service that got a provider but no global
	// propagator would emit spans that no downstream service can join.
	fields := otel.GetTextMapPropagator().Fields()
	if !contains(fields, "traceparent") || !contains(fields, "baggage") {
		t.Fatalf("propagator fields = %v, want traceparent and baggage", fields)
	}
	if otel.GetTracerProvider() != providers.TracerProvider {
		t.Fatal("global tracer provider is not the assembled one")
	}
	if otel.GetMeterProvider() != providers.MeterProvider {
		t.Fatal("global meter provider is not the assembled one")
	}

	_, span := providers.Tracer().Start(context.Background(), "checkout")
	span.End()
	// Flush rather than shut down: tracetest's exporter clears what it
	// collected when it is shut down.
	if err := providers.TracerProvider.(*sdktrace.TracerProvider).ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	if got := resourceAttribute(spans[0].Resource.Attributes(), semconv.ServiceNameKey); got != "checkout" {
		t.Fatalf("service.name = %q, want %q", got, "checkout")
	}
	if got := resourceAttribute(spans[0].Resource.Attributes(), semconv.ServiceVersionKey); got != "1.4.0" {
		t.Fatalf("service.version = %q, want %q", got, "1.4.0")
	}
	if got := resourceAttribute(spans[0].Resource.Attributes(), semconv.DeploymentEnvironmentNameKey); got != "staging" {
		t.Fatalf("deployment.environment.name = %q, want %q", got, "staging")
	}
}

func TestSetupSelectsSignalsIndividually(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	providers, err := Setup(context.Background(), Config{
		ServiceName:  "checkout",
		Signals:      SignalMetrics,
		MetricReader: reader,
	})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() { _ = providers.Shutdown(context.Background()) })

	// Metrics alone must not require a trace exporter: asking for one signal
	// and paying for a connection to another is how "optional" stops being
	// true.
	if _, ok := providers.TracerProvider.(*sdktrace.TracerProvider); ok {
		t.Fatal("traces were assembled although only metrics were selected")
	}
	if _, ok := providers.MeterProvider.(*sdkmetric.MeterProvider); !ok {
		t.Fatalf("meter provider = %T, want the SDK provider", providers.MeterProvider)
	}
}

func TestSetupRejectsIncompleteConfig(t *testing.T) {
	if _, err := Setup(context.Background(), Config{}); err == nil {
		t.Fatal("Setup without a service name returned no error")
	}
	if _, err := Setup(context.Background(), Config{ServiceName: "checkout", Protocol: "thrift"}); err == nil {
		t.Fatal("Setup with an unknown protocol returned no error")
	}
}

func TestShutdownRunsOnce(t *testing.T) {
	exporter := &countingExporter{}
	providers, err := Setup(context.Background(), Config{
		ServiceName:  "checkout",
		SpanExporter: exporter,
		MetricReader: sdkmetric.NewManualReader(),
	})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := providers.Shutdown(context.Background()); err != nil {
		t.Fatalf("first Shutdown: %v", err)
	}
	// A deferred Shutdown plus one in a signal handler is the normal shape, so
	// the second call has to be a no-op rather than a second teardown.
	if err := providers.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown = %v, want nil", err)
	}
	if exporter.shutdowns != 1 {
		t.Fatalf("exporter shutdowns = %d, want 1", exporter.shutdowns)
	}
}

func TestShutdownRejectsNilContext(t *testing.T) {
	providers, err := Setup(context.Background(), Config{
		ServiceName:  "checkout",
		SpanExporter: &countingExporter{},
		MetricReader: sdkmetric.NewManualReader(),
	})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() { _ = providers.Shutdown(context.Background()) })
	//nolint:staticcheck // a nil context is exactly what this rejects
	if err := providers.Shutdown(nil); err == nil {
		t.Fatal("Shutdown(nil) returned no error")
	}
}

// countingExporter counts teardowns, which is how a test observes that
// Shutdown ran once without depending on how the SDK reports exporter errors.
type countingExporter struct {
	shutdowns int
}

func (*countingExporter) ExportSpans(context.Context, []sdktrace.ReadOnlySpan) error { return nil }

func (e *countingExporter) Shutdown(context.Context) error {
	e.shutdowns++
	return nil
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func resourceAttribute(attrs []attribute.KeyValue, key attribute.Key) string {
	for _, attr := range attrs {
		if attr.Key == key {
			return attr.Value.Emit()
		}
	}
	return ""
}
