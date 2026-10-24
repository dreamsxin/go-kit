package oteladapter

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/semconv/v1.40.0/rpcconv"
	"go.opentelemetry.io/otel/trace"

	"google.golang.org/grpc"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	grpcserver "github.com/dreamsxin/go-kit/v2/integrations/grpc/server"
)

// gRPC span attribute names under the RPC semantic conventions. v1.40 spells
// the system attribute rpc.system.name; the grpc status code keeps its
// grpc-specific attribute because the conventions define it there.
const (
	attrRPCSystemName = "rpc.system.name"
	attrRPCService    = "rpc.service"
	attrRPCMethod     = "rpc.method"
	attrGRPCCode      = "rpc.grpc.status_code"
)

// GRPCServerSpanInterceptor returns a SERVER interceptor that starts one span
// per unary RPC, named after the full method per the RPC conventions:
// `package.Service/Method`. Chain it with the traceparent extractor — the
// framework-extracted context parents the span, and the endpoint span this
// adapter starts beneath it joins the same trace.
//
// The status rule is the conventions' one for RPC: the status code is an
// attribute on every span, and any non-OK code marks the span Error.
//
// Stable: otel.grpc-server-span — the interceptor starts one SERVER span per
// unary RPC named package.Service/Method, carrying rpc.system.name,
// rpc.service, rpc.method and rpc.grpc.status_code, marked Error on any
// non-OK code.
// Covered by: TestGRPCServerSpanInterceptorNamesTheMethod,
// TestGRPCServerSpanInterceptorMarksNonOKAsError
func GRPCServerSpanInterceptor(tracer trace.Tracer) grpc.UnaryServerInterceptor {
	if tracer == nil {
		tracer = otel.Tracer(instrumentationName)
	}
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		ctx, span := tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(grpcSpanAttributes(info.FullMethod)...),
		)
		defer span.End()

		response, err := handler(ctx, req)
		applyGRPCStatus(span, err)
		return response, err
	}
}

// GRPCClientSpanInterceptor returns a CLIENT interceptor that starts one span
// per unary call on the same full-method shape, so the client side of a hop
// appears beneath the server span that received the request. Chain it beside
// transportgrpc.TraceparentUnaryClientInterceptor; both read the active
// context, so the injected header names the span this one started.
//
// Stable: otel.grpc-client-span — the interceptor starts one CLIENT span per
// unary call named package.Service/Method with the same attributes, marked
// Error on any non-OK code.
// Covered by: TestGRPCClientSpanStartsUnderAnActiveParent
func GRPCClientSpanInterceptor(tracer trace.Tracer) grpc.UnaryClientInterceptor {
	if tracer == nil {
		tracer = otel.Tracer(instrumentationName)
	}
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		ctx, span := tracer.Start(ctx, method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(grpcSpanAttributes(method)...),
		)
		defer span.End()

		err := invoker(ctx, method, req, reply, cc, opts...)
		applyGRPCStatus(span, err)
		return err
	}
}

// applyGRPCStatus records the conventions' outcome: the status code is an
// attribute on every span, and a non-OK code marks the span Error.
func applyGRPCStatus(span trace.Span, err error) {
	code := grpccodes.OK
	if err != nil {
		if s, ok := status.FromError(err); ok {
			code = s.Code()
		} else {
			code = grpccodes.Unknown
		}
	}
	span.SetAttributes(attribute.Int(attrGRPCCode, int(code)))
	if code != grpccodes.OK {
		span.SetStatus(otelcodes.Error, code.String())
	}
}

// grpcSpanAttributes splits a full method into the conventions' dimensions.
// A method outside the `/package.Service/Method` shape still records bounded
// values — the split falls back to the whole string as the service — because
// the set of methods is fixed by the service definition, not by traffic.
func grpcSpanAttributes(fullMethod string) []attribute.KeyValue {
	service, method := splitFullMethod(fullMethod)
	return []attribute.KeyValue{
		attribute.String(attrRPCSystemName, "grpc"),
		attribute.String(attrRPCService, service),
		attribute.String(attrRPCMethod, method),
	}
}

func splitFullMethod(fullMethod string) (service, method string) {
	rest := strings.TrimPrefix(fullMethod, "/")
	if at := strings.LastIndex(rest, "/"); at >= 0 {
		return rest[:at], rest[at+1:]
	}
	return rest, ""
}

// GRPCMetrics records gRPC server RPCs under the RPC semantic conventions:
// the rpc.server.call.duration histogram carrying rpc.system.name,
// rpc.method, the response status code, and a stream marker, so a stream's
// whole-lifetime duration is never averaged with a unary latency. The unit is
// seconds because the instrument defines it so; the Observation's Duration is
// what the transport measured.
//
// It implements grpcserver.Recorder, so it plugs into the transport's
// recording interceptors the same way the exposition bridge does:
//
//	grpc.ChainUnaryInterceptor(grpcserver.RecordingUnaryInterceptor(otelGRPC))
//
// A nil meter is a misassembly rather than a silent no-op.
//
// Stable: otel.grpc-metrics-agree-with-the-recorder — the OpenTelemetry adapter and the built-in exposition report the same counts for the same RPCs.
// Covered by: TestGRPCMetricsAgreeWithTheRecorder
type GRPCMetrics struct {
	duration rpcconv.ServerCallDuration
}

var _ grpcserver.Recorder = (*GRPCMetrics)(nil)

// NewGRPCMetrics creates the RPC instrument from the application-owned meter.
func NewGRPCMetrics(meter metric.Meter) (*GRPCMetrics, error) {
	if meter == nil {
		return nil, errNilMeter{}
	}
	duration, err := rpcconv.NewServerCallDuration(meter)
	if err != nil {
		return nil, err
	}
	return &GRPCMetrics{duration: duration}, nil
}

// ObserveGRPC implements grpcserver.Recorder.
func (m *GRPCMetrics) ObserveGRPC(ctx context.Context, obs grpcserver.Observation) {
	if m == nil {
		return
	}
	_, method := splitFullMethod(obs.Method)
	m.duration.Record(ctx, obs.Duration.Seconds(), rpcconv.SystemNameGRPC,
		m.duration.AttrMethod(method),
		m.duration.AttrResponseStatusCode(obs.Code.String()),
		attribute.Bool("rpc.stream", obs.Stream),
	)
}

type errNilMeter struct{}

func (errNilMeter) Error() string { return "oteladapter: meter is nil" }
