package oteladapter_test

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	transportgrpc "github.com/dreamsxin/go-kit/v2/integrations/grpc"
	grpcserver "github.com/dreamsxin/go-kit/v2/integrations/grpc/server"
	metricsgrpc "github.com/dreamsxin/go-kit/v2/observability/metrics/grpc"
	oteladapter "github.com/dreamsxin/go-kit/v2/observability/otel"
)

// The gRPC span interceptors are exercised the way the transport tests
// exercise theirs: by calling the interceptor with a stub handler or
// invoker, which keeps every assertion deterministic — a bufconn round-trip
// measures the machine and the scheduler, not the span.
func TestGRPCServerSpanInterceptorNamesTheMethod(t *testing.T) {
	tracer, recorder := newRecordingTracer(t)
	interceptor := oteladapter.GRPCServerSpanInterceptor(tracer)

	if _, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/test.SpanService/Ping"},
		func(context.Context, any) (any, error) { return &struct{}{}, nil },
	); err != nil {
		t.Fatalf("handler error surfaced: %v", err)
	}

	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("spans = %d, want one", len(ended))
	}
	stub := ended[0]
	if stub.Name() != "/test.SpanService/Ping" {
		t.Errorf("name = %q, want the full method", stub.Name())
	}
	if stub.SpanKind() != trace.SpanKindServer {
		t.Errorf("kind = %v, want SERVER", stub.SpanKind())
	}
	assertGRPCAttributes(t, stub, "test.SpanService/Ping", "0")
	if stub.Status().Code == codes.Error {
		t.Errorf("an OK RPC marked the span Error: %v", stub.Status())
	}
}

func TestGRPCServerSpanInterceptorMarksNonOKAsError(t *testing.T) {
	tracer, recorder := newRecordingTracer(t)
	interceptor := oteladapter.GRPCServerSpanInterceptor(tracer)
	notFound := status.Error(grpccodes.NotFound, "no such key")

	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/test.SpanService/Missing"},
		func(context.Context, any) (any, error) { return nil, notFound },
	)
	if err == nil {
		t.Fatal("the interceptor must surface the handler error unchanged")
	}

	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("spans = %d, want one", len(ended))
	}
	stub := ended[0]
	if stub.Status().Code != codes.Error {
		t.Fatalf("status = %v, want Error for NotFound", stub.Status().Code)
	}
	assertGRPCAttributes(t, stub, "test.SpanService/Missing", "5")
}

// TestGRPCClientSpanStartsUnderAnActiveParent pins the tree property with a
// stub invoker: the CLIENT span the interceptor starts is a child of the
// span active in the call context — which is what the SERVER span on the
// receiving side becomes through the traceparent bridge.
func TestGRPCClientSpanStartsUnderAnActiveParent(t *testing.T) {
	tracer, recorder := newRecordingTracer(t)
	interceptor := oteladapter.GRPCClientSpanInterceptor(tracer)

	ctx, parent := tracer.Start(context.Background(), "/test.SpanService/Ping",
		trace.WithSpanKind(trace.SpanKindServer))
	var invokerSaw trace.SpanID
	err := interceptor(ctx, "/test.SpanService/Ping", nil, nil, nil,
		func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			invokerSaw = trace.SpanContextFromContext(ctx).SpanID()
			return nil
		})
	if err != nil {
		t.Fatalf("invoker error surfaced: %v", err)
	}
	parent.End()

	var client sdktrace.ReadOnlySpan
	for _, stub := range recorder.Ended() {
		if stub.SpanKind() == trace.SpanKindClient {
			client = stub
		}
	}
	if client == nil {
		t.Fatal("no CLIENT span was started")
	}
	if client.Name() != "/test.SpanService/Ping" || client.SpanKind() != trace.SpanKindClient {
		t.Errorf("client span = %q/%v", client.Name(), client.SpanKind())
	}
	if client.Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Fatalf("client parent = %v, want the active span %s",
			client.Parent().SpanID(), parent.SpanContext().SpanID())
	}
	if invokerSaw != client.SpanContext().SpanID() {
		t.Fatalf("invoker saw span %s, want the CLIENT span %s", invokerSaw, client.SpanContext().SpanID())
	}
}

// TestGRPCMetricsAgreeWithTheRecorder pins otel.grpc-metrics-agree-with-the-recorder:
// the same observation feeds the OTel instrument and the built-in exposition,
// and both count it once.
func TestGRPCMetricsAgreeWithTheRecorder(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	otelGRPC, err := oteladapter.NewGRPCMetrics(mp.Meter("oteladapter_test"))
	if err != nil {
		t.Fatalf("NewGRPCMetrics: %v", err)
	}
	observation := grpcserver.Observation{
		Method:   "/test.SpanService/Ping",
		Code:     grpccodes.OK,
		Duration: 5 * time.Millisecond,
	}
	collector := &endpoint.Metrics{}
	bridge := metricsgrpc.Recorder(collector)
	otelGRPC.ObserveGRPC(context.Background(), observation)
	bridge.ObserveGRPC(context.Background(), observation)

	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatalf("collect: %v", err)
	}
	var count uint64
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != "rpc.server.call.duration" {
				continue
			}
			histogram, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("data type = %T", m.Data)
			}
			if len(histogram.DataPoints) != 1 {
				t.Fatalf("data points = %d, want one series", len(histogram.DataPoints))
			}
			point := histogram.DataPoints[0]
			count = point.Count
			attrs := map[string]string{}
			for _, kv := range point.Attributes.ToSlice() {
				attrs[string(kv.Key)] = kv.Value.Emit()
			}
			if attrs["rpc.method"] != "test.SpanService/Ping" || attrs["rpc.system.name"] != "grpc" {
				t.Errorf("attrs = %v, want the method and system dimensions", attrs)
			}
		}
	}
	if count != 1 {
		t.Fatalf("OTel count = %d, want 1", count)
	}
	if got := collector.SnapshotFor("/test.SpanService/Ping").RequestCount; got != 1 {
		t.Errorf("exposition count = %d, want 1", got)
	}
}

func assertGRPCAttributes(t *testing.T, stub sdktrace.ReadOnlySpan, fullMethod, code string) {
	t.Helper()
	for key, want := range map[string]string{
		"rpc.system.name":      "grpc",
		"rpc.service":          "test.SpanService",
		"rpc.method":           fullMethod,
		"rpc.grpc.status_code": code,
	} {
		got := ""
		for _, kv := range stub.Attributes() {
			if string(kv.Key) == key {
				got = kv.Value.Emit()
			}
		}
		if got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

// TestGRPCServerSpanContinuesTheExtractedTrace pins the chaining the doc
// promises: the interceptor bridges a framework-extracted W3C context, so
// the server span is a child of the caller's trace, not a fresh root.
func TestGRPCServerSpanContinuesTheExtractedTrace(t *testing.T) {
	tracer, recorder := newRecordingTracer(t)
	interceptor := oteladapter.GRPCServerSpanInterceptor(tracer)
	inbound := endpoint.TraceContext{
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:  "00f067aa0ba902b7",
		Flags:   "01",
	}
	ctx := endpoint.WithIncomingTraceContext(context.Background(), inbound)

	var handlerSpan trace.SpanID
	if _, err := interceptor(ctx, nil,
		&grpc.UnaryServerInfo{FullMethod: "/test.SpanService/Ping"},
		func(ctx context.Context, _ any) (any, error) {
			handlerSpan = trace.SpanContextFromContext(ctx).SpanID()
			return &struct{}{}, nil
		},
	); err != nil {
		t.Fatalf("handler error surfaced: %v", err)
	}

	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("spans = %d, want one", len(ended))
	}
	stub := ended[0]
	if stub.SpanContext().TraceID().String() != inbound.TraceID {
		t.Fatalf("server span trace = %s, want the inbound trace %s",
			stub.SpanContext().TraceID(), inbound.TraceID)
	}
	if stub.Parent().SpanID().String() != inbound.SpanID {
		t.Fatalf("server span parent = %s, want the caller span %s",
			stub.Parent().SpanID(), inbound.SpanID)
	}
	if handlerSpan != stub.SpanContext().SpanID() {
		t.Fatalf("handler saw span %s, want the server span %s", handlerSpan, stub.SpanContext().SpanID())
	}
}

// TestGRPCClientSpanFeedsTheTraceparentInjector pins the other chaining the
// doc promises: the span's identity is published as the framework
// correlation, so the transport's default injector puts it on the wire.
func TestGRPCClientSpanFeedsTheTraceparentInjector(t *testing.T) {
	tracer, recorder := newRecordingTracer(t)
	spanInterceptor := oteladapter.GRPCClientSpanInterceptor(tracer)
	injector := transportgrpc.TraceparentUnaryClientInterceptor()

	var injected string
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return nil
	}
	wrapped := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		md, _ := metadata.FromOutgoingContext(ctx)
		values := md.Get("traceparent")
		if len(values) != 1 {
			t.Fatalf("outgoing traceparent = %v, want exactly one", values)
		}
		injected = values[0]
		return invoker(ctx, method, req, reply, cc, opts...)
	}

	if err := spanInterceptor(context.Background(), "/test.SpanService/Ping", nil, nil, nil,
		func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			// The transport's injector runs inside, as a chained
			// interceptor would.
			return injector(ctx, method, req, reply, cc, wrapped, opts...)
		}); err != nil {
		t.Fatalf("invoker error surfaced: %v", err)
	}

	ended := recorder.Ended()
	if len(ended) != 1 || ended[0].SpanKind() != trace.SpanKindClient {
		t.Fatalf("spans = %d, want exactly the client span", len(ended))
	}
	sc := ended[0].SpanContext()
	if want := "00-" + sc.TraceID().String() + "-" + sc.SpanID().String() + "-01"; injected != want {
		t.Fatalf("traceparent = %q, want the span the interceptor started %q", injected, want)
	}
}
