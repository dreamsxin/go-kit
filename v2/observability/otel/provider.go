package oteladapter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	noopMetric "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
	noopTrace "go.opentelemetry.io/otel/trace/noop"
)

// Protocol selects the OTLP wire protocol an exporter speaks.
type Protocol string

// The OTLP protocols this package assembles. They are the values the
// OTEL_EXPORTER_OTLP_PROTOCOL environment variable takes, so a deployment can
// keep using the name it already knows.
const (
	// ProtocolGRPC exports OTLP over gRPC, the default, on port 4317.
	ProtocolGRPC Protocol = "grpc"
	// ProtocolHTTP exports OTLP over HTTP with protobuf payloads, on port 4318.
	ProtocolHTTP Protocol = "http/protobuf"
)

// Signals selects which signals Setup assembles. The zero value means every
// signal, so a Config that says nothing about signals gets the whole pipeline.
type Signals uint8

// The signals Setup can assemble.
const (
	SignalTraces Signals = 1 << iota
	SignalMetrics
)

// AllSignals is every signal Setup assembles, which is also what the zero
// value of Signals selects.
const AllSignals = SignalTraces | SignalMetrics

func (s Signals) has(signal Signals) bool {
	if s == 0 {
		return true
	}
	return s&signal != 0
}

// Config describes the OpenTelemetry pipeline Setup assembles.
//
// The zero value plus a ServiceName is a working configuration: both signals,
// OTLP over gRPC to the endpoint the OTEL_EXPORTER_OTLP_* environment
// variables name, and everything sampled.
type Config struct {
	// ServiceName is the service.name resource attribute. Required: telemetry
	// without it cannot be attributed to a service by any backend.
	ServiceName string
	// ServiceVersion is the service.version resource attribute.
	ServiceVersion string
	// Environment is the deployment.environment.name resource attribute, for
	// example "production".
	Environment string
	// ResourceAttributes adds application-owned resource attributes.
	ResourceAttributes []attribute.KeyValue

	// Signals selects the signals to assemble. The zero value assembles all.
	Signals Signals

	// Endpoint is the OTLP collector endpoint: "host:port" for ProtocolGRPC,
	// "host:port" or a base URL for ProtocolHTTP. Empty defers to the
	// OTEL_EXPORTER_OTLP_ENDPOINT environment variable, and then to the
	// protocol's default localhost port.
	Endpoint string
	// Protocol selects the OTLP protocol. Empty means ProtocolGRPC.
	Protocol Protocol
	// Insecure sends OTLP without TLS. Use it for a collector on localhost or
	// inside a pod; a collector reached across a network should keep TLS.
	Insecure bool
	// Headers are sent with every OTLP request, for a collector or vendor
	// endpoint that authenticates.
	Headers map[string]string

	// SampleRatio is the fraction of traces to sample, applied to root spans
	// only: a sampled parent keeps its children. Values <= 0 or >= 1 sample
	// every trace.
	SampleRatio float64
	// MetricInterval is how often metrics are exported. Zero uses the SDK
	// default, which is also what OTEL_METRIC_EXPORT_INTERVAL configures.
	MetricInterval time.Duration

	// SpanExporter replaces the OTLP span exporter. Set it to export
	// somewhere else, or to a recording exporter in a test.
	SpanExporter sdktrace.SpanExporter
	// MetricReader replaces the OTLP periodic reader. Set it to export
	// somewhere else, for example a Prometheus reader that a scrape endpoint
	// serves.
	MetricReader sdkmetric.Reader
}

// Providers is the assembled pipeline: the providers to read instruments from,
// and the one Shutdown that flushes and stops all of them.
type Providers struct {
	// TracerProvider is the assembled tracer provider, or a no-op provider
	// when traces were not selected.
	TracerProvider trace.TracerProvider
	// MeterProvider is the assembled meter provider, or a no-op provider when
	// metrics were not selected.
	MeterProvider metric.MeterProvider
	// Resource is the resource every signal is attributed to.
	Resource *resource.Resource

	shutdownOnce sync.Once
	shutdowns    []func(context.Context) error
}

// Setup assembles an OpenTelemetry pipeline and installs it globally: the
// tracer and meter providers become the process-wide ones, and the global
// propagator becomes W3C trace context plus baggage, so any library that
// propagates through the OpenTelemetry API interoperates with the traceparent
// header this framework's transports carry.
//
// The caller owns shutdown. Defer Providers.Shutdown; without it a batch of
// spans and the last metric interval are lost on exit.
//
//	providers, err := oteladapter.Setup(ctx, oteladapter.Config{
//	    ServiceName: "checkout",
//	    Endpoint:    "collector:4317",
//	    Insecure:    true,
//	})
//	if err != nil {
//	    return err
//	}
//	defer providers.Shutdown(context.Background())
//
//	metrics, err := oteladapter.NewMetrics(providers.Meter())
func Setup(ctx context.Context, cfg Config) (*Providers, error) {
	if ctx == nil {
		return nil, errors.New("oteladapter: nil context")
	}
	if strings.TrimSpace(cfg.ServiceName) == "" {
		return nil, errors.New("oteladapter: service name cannot be empty")
	}
	if cfg.Protocol == "" {
		cfg.Protocol = ProtocolGRPC
	}
	if cfg.Protocol != ProtocolGRPC && cfg.Protocol != ProtocolHTTP {
		return nil, fmt.Errorf("oteladapter: unknown OTLP protocol %q", cfg.Protocol)
	}

	res, err := buildResource(cfg)
	if err != nil {
		return nil, err
	}
	providers := &Providers{
		TracerProvider: noopTrace.NewTracerProvider(),
		MeterProvider:  noopMetric.NewMeterProvider(),
		Resource:       res,
	}

	if cfg.Signals.has(SignalTraces) {
		tracerProvider, err := buildTracerProvider(ctx, cfg, res)
		if err != nil {
			// Nothing has been installed globally yet, but an exporter may
			// already hold a connection.
			_ = providers.Shutdown(ctx)
			return nil, err
		}
		providers.TracerProvider = tracerProvider
		providers.shutdowns = append(providers.shutdowns, tracerProvider.Shutdown)
	}
	if cfg.Signals.has(SignalMetrics) {
		meterProvider, err := buildMeterProvider(ctx, cfg, res)
		if err != nil {
			_ = providers.Shutdown(ctx)
			return nil, err
		}
		providers.MeterProvider = meterProvider
		providers.shutdowns = append(providers.shutdowns, meterProvider.Shutdown)
	}

	if cfg.Signals.has(SignalTraces) {
		otel.SetTracerProvider(providers.TracerProvider)
	}
	if cfg.Signals.has(SignalMetrics) {
		otel.SetMeterProvider(providers.MeterProvider)
	}
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return providers, nil
}

// Tracer returns the tracer this package's middleware uses by default, so
// TracingMiddleware(providers.Tracer(), "operation") and
// TracingMiddleware(nil, "operation") name the same instrumentation scope.
func (p *Providers) Tracer() trace.Tracer {
	if p == nil {
		return otel.Tracer(instrumentationName)
	}
	return p.TracerProvider.Tracer(instrumentationName)
}

// Meter returns the meter to build this package's instruments from:
// NewMetrics(providers.Meter()) and NewHTTPMetrics(providers.Meter()).
func (p *Providers) Meter() metric.Meter {
	if p == nil {
		return otel.Meter(instrumentationName)
	}
	return p.MeterProvider.Meter(instrumentationName)
}

// Shutdown flushes and stops every assembled provider, and reports the joined
// errors. It runs once: later calls return nil, so a deferred Shutdown and an
// explicit one in a signal handler cannot double-shutdown.
//
// Pass a context with a deadline. Shutdown blocks while the last batch is
// exported, and a collector that stopped answering would otherwise hold
// process exit for as long as the exporter retries.
func (p *Providers) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("oteladapter: nil shutdown context")
	}
	var err error
	p.shutdownOnce.Do(func() {
		// Reverse order: the provider assembled last shuts down first, which
		// is the order the assembly would unwind.
		for i := len(p.shutdowns) - 1; i >= 0; i-- {
			err = errors.Join(err, p.shutdowns[i](ctx))
		}
		p.shutdowns = nil
	})
	return err
}

func buildResource(cfg Config) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{semconv.ServiceName(cfg.ServiceName)}
	if cfg.ServiceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(cfg.ServiceVersion))
	}
	if cfg.Environment != "" {
		attrs = append(attrs, semconv.DeploymentEnvironmentName(cfg.Environment))
	}
	attrs = append(attrs, cfg.ResourceAttributes...)

	// Merging over resource.Default keeps the SDK's telemetry.sdk.* attributes
	// and the OTEL_RESOURCE_ATTRIBUTES a deployment sets, with the explicit
	// configuration winning.
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(semconv.SchemaURL, attrs...))
	if err != nil {
		return nil, fmt.Errorf("oteladapter: build resource: %w", err)
	}
	return res, nil
}

func buildTracerProvider(ctx context.Context, cfg Config, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	exporter := cfg.SpanExporter
	if exporter == nil {
		var err error
		switch cfg.Protocol {
		case ProtocolHTTP:
			exporter, err = otlptracehttp.New(ctx, traceHTTPOptions(cfg)...)
		default:
			exporter, err = otlptracegrpc.New(ctx, traceGRPCOptions(cfg)...)
		}
		if err != nil {
			return nil, fmt.Errorf("oteladapter: create span exporter: %w", err)
		}
	}
	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler(cfg.SampleRatio)),
	), nil
}

func buildMeterProvider(ctx context.Context, cfg Config, res *resource.Resource) (*sdkmetric.MeterProvider, error) {
	reader := cfg.MetricReader
	if reader == nil {
		var (
			exporter sdkmetric.Exporter
			err      error
		)
		switch cfg.Protocol {
		case ProtocolHTTP:
			exporter, err = otlpmetrichttp.New(ctx, metricHTTPOptions(cfg)...)
		default:
			exporter, err = otlpmetricgrpc.New(ctx, metricGRPCOptions(cfg)...)
		}
		if err != nil {
			return nil, fmt.Errorf("oteladapter: create metric exporter: %w", err)
		}
		var readerOptions []sdkmetric.PeriodicReaderOption
		if cfg.MetricInterval > 0 {
			readerOptions = append(readerOptions, sdkmetric.WithInterval(cfg.MetricInterval))
		}
		reader = sdkmetric.NewPeriodicReader(exporter, readerOptions...)
	}
	return sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
	), nil
}

// sampler samples root spans by ratio and defers to the parent decision
// otherwise, so a sampled request stays sampled through every service it
// reaches.
func sampler(ratio float64) sdktrace.Sampler {
	if ratio <= 0 || ratio >= 1 {
		return sdktrace.AlwaysSample()
	}
	return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
}

func traceGRPCOptions(cfg Config) []otlptracegrpc.Option {
	options := []otlptracegrpc.Option{}
	if cfg.Endpoint != "" {
		options = append(options, otlptracegrpc.WithEndpoint(cfg.Endpoint))
	}
	if cfg.Insecure {
		options = append(options, otlptracegrpc.WithInsecure())
	}
	if len(cfg.Headers) > 0 {
		options = append(options, otlptracegrpc.WithHeaders(cfg.Headers))
	}
	return options
}

func traceHTTPOptions(cfg Config) []otlptracehttp.Option {
	options := []otlptracehttp.Option{}
	if cfg.Endpoint != "" {
		options = append(options, otlptracehttp.WithEndpoint(cfg.Endpoint))
	}
	if cfg.Insecure {
		options = append(options, otlptracehttp.WithInsecure())
	}
	if len(cfg.Headers) > 0 {
		options = append(options, otlptracehttp.WithHeaders(cfg.Headers))
	}
	return options
}

func metricGRPCOptions(cfg Config) []otlpmetricgrpc.Option {
	options := []otlpmetricgrpc.Option{}
	if cfg.Endpoint != "" {
		options = append(options, otlpmetricgrpc.WithEndpoint(cfg.Endpoint))
	}
	if cfg.Insecure {
		options = append(options, otlpmetricgrpc.WithInsecure())
	}
	if len(cfg.Headers) > 0 {
		options = append(options, otlpmetricgrpc.WithHeaders(cfg.Headers))
	}
	return options
}

func metricHTTPOptions(cfg Config) []otlpmetrichttp.Option {
	options := []otlpmetrichttp.Option{}
	if cfg.Endpoint != "" {
		options = append(options, otlpmetrichttp.WithEndpoint(cfg.Endpoint))
	}
	if cfg.Insecure {
		options = append(options, otlpmetrichttp.WithInsecure())
	}
	if len(cfg.Headers) > 0 {
		options = append(options, otlpmetrichttp.WithHeaders(cfg.Headers))
	}
	return options
}
