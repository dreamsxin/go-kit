// Package oteladapter provides optional OpenTelemetry endpoint adapters.
// Provider setup, exporters, sampling, and resource attributes remain under
// application control.
package oteladapter

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
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

// errEndpointPanicked is what a span records when the endpoint unwound through
// a panic. The panic value itself is not available without recovering it, which
// would change the program's behaviour.
var errEndpointPanicked = errors.New("endpoint panicked")

// TracingMiddleware creates one span for each endpoint invocation. A nil
// tracer uses the application's configured global provider.
//
// A panic ends the span with an Error status and a recorded exception, then
// propagates. Only span.End would run otherwise, leaving the span Unset —
// indistinguishable from a success to most backends.
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
			returned := false
			defer func() {
				if !returned {
					span.RecordError(errEndpointPanicked)
					span.SetStatus(codes.Error, "endpoint panicked")
				}
				span.End()
			}()

			response, err = next(spanCtx, request)
			returned = true
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

// Metrics records endpoint request counts and duration. It owns instruments
// but not the provider lifecycle.
//
// The instruments follow the OpenTelemetry conventions: duration is a
// histogram in seconds, and a failed call is a request carrying the
// error.type attribute rather than a second counter. Error rate is therefore
// a ratio over one time series, and the taxonomy an error names through
// interface{ ErrorKindName() string } — every apperror value does — becomes
// the error.type value.
type Metrics struct {
	requests metric.Int64Counter
	duration metric.Float64Histogram
	attrs    []attribute.KeyValue
}

var _ endpoint.Recorder = (*Metrics)(nil)

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
		metric.WithDescription("Endpoint requests."),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}
	duration, err := meter.Float64Histogram(
		"go_kit.endpoint.duration",
		metric.WithDescription("Duration of endpoint calls."),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}
	return &Metrics{requests: requests, duration: duration, attrs: attrs}, nil
}

// Observe implements endpoint.Recorder, so the adapter plugs into the same
// recording path as every other backend: endpoint.RecordingMiddleware, or
// kit.WithRecorder to cover every route with its pattern as the operation.
func (m *Metrics) Observe(ctx context.Context, obs endpoint.Observation) {
	if m == nil {
		return
	}
	attrs := append([]attribute.KeyValue(nil), m.attrs...)
	attrs = append(attrs, attribute.String("operation", obs.Operation))
	if obs.Err != nil {
		attrs = append(attrs, errorType(obs.Err))
	}
	options := metric.WithAttributes(attrs...)
	m.requests.Add(ctx, 1, options)
	m.duration.Record(ctx, obs.Duration.Seconds(), options)
}

// Middleware returns endpoint metrics middleware for a bounded operation name.
// It is shorthand for endpoint.RecordingMiddleware(operation, m).
func (m *Metrics) Middleware(operation string) endpoint.Middleware {
	if m == nil {
		return func(next endpoint.Endpoint) endpoint.Endpoint { return next }
	}
	return endpoint.RecordingMiddleware(operation, m)
}

// errorType maps an error to the semantic conventions' error.type attribute.
// An error that names its kind through the structural classification contract
// — interface{ ErrorKindName() string }, which every apperror value satisfies
// — reports that kind, so the dimension stays bounded and readable. Anything
// else reports the conventions' _OTHER.
func errorType(err error) attribute.KeyValue {
	var namer interface{ ErrorKindName() string }
	if errors.As(err, &namer) {
		if kind := namer.ErrorKindName(); kind != "" {
			return semconv.ErrorTypeKey.String(kind)
		}
	}
	return semconv.ErrorTypeOther
}
