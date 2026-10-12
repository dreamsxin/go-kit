// Package oteladapter provides optional OpenTelemetry endpoint adapters.
// Provider setup, exporters, sampling, and resource attributes remain under
// application control.
package oteladapter

import (
	"context"
	"encoding/hex"
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
// The trace context the framework's transports extracted — the W3C traceparent
// header parsed into an endpoint.TraceContext — parents this span when the
// context carries no OpenTelemetry span of its own, and the span's identity is
// installed as the correlation values the framework reads when none are set.
// One request therefore describes one trace everywhere it goes: the traceparent
// header a client receives continues this span, and the trace_id the logging
// middleware records is the exported trace's own.
//
// A panic ends the span with an Error status and a recorded exception, then
// propagates. Only span.End would run otherwise, leaving the span Unset —
// indistinguishable from a success to most backends.
//
// Stable: otel.traceparent-bridged — a framework-extracted traceparent parents
// the span when no OpenTelemetry span is present, and a span with no
// framework correlation installs its own, so the header, the exported trace,
// and the logged trace_id name one trace.
// Covered by: TestTracingMiddlewareBridgesTheFrameworkTraceContext, TestTracingMiddlewareInstallsCorrelationFromTheSpan, TestTracingMiddlewareDefersToAnOtelParent
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
			ctx = bridgeFrameworkTraceContext(ctx)
			spanCtx, span := tracer.Start(ctx, operation,
				trace.WithSpanKind(cfg.kind),
				trace.WithAttributes(cfg.attributes...),
			)
			spanCtx = installSpanCorrelation(spanCtx, span)
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

// bridgeFrameworkTraceContext parents the span in the trace context the
// framework's transports extracted when the request context carries no
// OpenTelemetry span of its own. Without it, the first span of every service
// would be a fresh root with a fresh trace ID, and the cross-service tree the
// traceparent header already connects would stay invisible to a tracing
// backend.
//
// An OpenTelemetry span already in the context wins: a caller that extracted
// through the OpenTelemetry API — or a middleware above this one that started
// a span — set the parent, and the correlation values it carried are left
// untouched rather than overridden.
func bridgeFrameworkTraceContext(ctx context.Context) context.Context {
	if trace.SpanContextFromContext(ctx).IsValid() {
		return ctx
	}
	tc := endpoint.TraceContextFromContext(ctx)
	if !tc.Valid() {
		return ctx
	}
	var traceID trace.TraceID
	var spanID trace.SpanID
	// The lengths and the lowercase-hex alphabet were checked by tc.Valid();
	// the decode cannot fail, and a zero value on the impossible path would
	// simply fail the span context's own validation below.
	_, _ = hex.Decode(traceID[:], []byte(tc.TraceID))
	_, _ = hex.Decode(spanID[:], []byte(tc.SpanID))
	flags := w3cFlags(tc.Flags)
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: flags,
		Remote:     true,
	})
	if !sc.IsValid() {
		return ctx
	}
	return trace.ContextWithRemoteSpanContext(ctx, sc)
}

// installSpanCorrelation publishes the span's identity through the
// correlation values the framework reads when none are set: the trace context
// the HTTP client injects into outgoing traceparent headers, and the trace ID
// the logging middleware records. It fills an absence; a correlation that
// arrived from elsewhere is not overridden, so an assembly that runs both this
// middleware and endpoint.TracingMiddleware keeps the identity the built-in
// middleware minted.
func installSpanCorrelation(ctx context.Context, span trace.Span) context.Context {
	if endpoint.TraceContextFromContext(ctx).Valid() {
		return ctx
	}
	sc := span.SpanContext()
	if !sc.IsValid() {
		return ctx
	}
	flags := "00"
	if sc.IsSampled() {
		flags = "01"
	}
	tc := endpoint.TraceContext{
		TraceID: sc.TraceID().String(),
		SpanID:  sc.SpanID().String(),
		Flags:   flags,
	}
	ctx = endpoint.WithTraceContext(ctx, tc)
	return endpoint.WithTraceID(ctx, endpoint.TraceID(tc.TraceID))
}

// w3cFlags converts the two-hex-digit trace-flags field into OpenTelemetry
// trace flags. Only the sampled bit is defined by the specification today; the
// reserved bits are cleared rather than forwarded, which is what a
// propagator is supposed to do with them.
func w3cFlags(field string) trace.TraceFlags {
	raw, err := hex.DecodeString(field)
	if err != nil || len(raw) != 1 {
		return 0
	}
	return trace.TraceFlags(raw[0] & byte(trace.FlagsSampled))
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
//
// Both ways out of v2 are fed by endpoint.RecordingMiddleware, so a dashboard
// built on the pushed series and an alert built on the scraped one describe the
// same traffic.
//
// Stable: otel.metrics-agree-with-the-exposition — the OpenTelemetry adapter and the metrics exposition report the same counts and the same total duration for the same observations.
// Covered by: TestMetricsAgreeWithTheExposition
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
