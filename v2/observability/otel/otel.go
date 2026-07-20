// Package oteladapter provides optional OpenTelemetry endpoint adapters.
// Provider setup, exporters, sampling, and resource attributes remain under
// application control.
package oteladapter

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

const instrumentationName = "github.com/dreamsxin/go-kit/v2/observability/otel"

type traceOptions struct {
	kind       trace.SpanKind
	attributes []attribute.KeyValue
}

// TraceOption configures TracingMiddleware.
type TraceOption func(*traceOptions)

// WithSpanKind sets the OpenTelemetry span kind. Internal is the default.
func WithSpanKind(kind trace.SpanKind) TraceOption {
	return func(options *traceOptions) { options.kind = kind }
}

// WithSpanAttributes adds bounded application-owned span attributes.
func WithSpanAttributes(attributes ...attribute.KeyValue) TraceOption {
	return func(options *traceOptions) {
		options.attributes = append(options.attributes, attributes...)
	}
}

// TracingMiddleware creates one span for each endpoint invocation. A nil
// tracer uses the application's configured global provider.
func TracingMiddleware(tracer trace.Tracer, operation string, options ...TraceOption) endpoint.Middleware {
	if tracer == nil {
		tracer = otel.Tracer(instrumentationName)
	}
	cfg := traceOptions{kind: trace.SpanKindInternal}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}

	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (response any, err error) {
			spanCtx, span := tracer.Start(ctx, operation,
				trace.WithSpanKind(cfg.kind),
				trace.WithAttributes(cfg.attributes...),
			)
			defer span.End()

			response, err = next(spanCtx, request)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "endpoint failed")
			} else {
				span.SetStatus(codes.Ok, "")
			}
			return response, err
		}
	}
}

type metricsOptions struct {
	attributes []attribute.KeyValue
}

// MetricsOption configures Metrics.
type MetricsOption func(*metricsOptions)

// WithMetricAttributes adds bounded application-owned attributes to every
// measurement. Resource attributes such as service.name belong on the meter
// provider configured by the application.
func WithMetricAttributes(attributes ...attribute.KeyValue) MetricsOption {
	return func(options *metricsOptions) {
		options.attributes = append(options.attributes, attributes...)
	}
}

// Metrics records endpoint request counts, error counts, and duration in
// milliseconds. It owns instruments but not the provider lifecycle.
type Metrics struct {
	requests metric.Int64Counter
	errors   metric.Int64Counter
	duration metric.Float64Histogram
	attrs    []attribute.KeyValue
}

// NewMetrics creates instruments from the application-owned meter.
func NewMetrics(meter metric.Meter, options ...MetricsOption) (*Metrics, error) {
	if meter == nil {
		return nil, errors.New("oteladapter: meter is nil")
	}
	cfg := metricsOptions{}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	attrs := append([]attribute.KeyValue(nil), cfg.attributes...)
	requests, err := meter.Int64Counter(
		"go_kit.endpoint.requests",
		metric.WithDescription("Endpoint requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}
	errorsCounter, err := meter.Int64Counter(
		"go_kit.endpoint.errors",
		metric.WithDescription("Endpoint errors"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return nil, err
	}
	duration, err := meter.Float64Histogram(
		"go_kit.endpoint.duration",
		metric.WithDescription("Endpoint duration"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}
	return &Metrics{requests: requests, errors: errorsCounter, duration: duration, attrs: attrs}, nil
}

// Middleware returns endpoint metrics middleware for a bounded operation name.
func (m *Metrics) Middleware(operation string) endpoint.Middleware {
	if m == nil {
		return func(next endpoint.Endpoint) endpoint.Endpoint { return next }
	}
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (response any, err error) {
			start := time.Now()
			response, err = next(ctx, request)
			attrs := append([]attribute.KeyValue(nil), m.attrs...)
			attrs = append(attrs,
				attribute.String("operation", operation),
				attribute.String("outcome", outcome(err)),
			)
			options := metric.WithAttributes(attrs...)
			m.requests.Add(ctx, 1, options)
			m.duration.Record(ctx, float64(time.Since(start))/float64(time.Millisecond), options)
			if err != nil {
				m.errors.Add(ctx, 1, options)
			}
			return response, err
		}
	}
}

func outcome(err error) string {
	if err != nil {
		return "error"
	}
	return "success"
}
