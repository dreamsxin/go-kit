package oteladapter_test

import (
	"context"
	"testing"

	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	transportgrpc "github.com/dreamsxin/go-kit/v2/integrations/grpc"
	oteladapter "github.com/dreamsxin/go-kit/v2/observability/otel"
)

// A stub server stream whose context the interceptor's wrapper must replace.
type stubServerStream struct {
	grpc.ServerStream
	base context.Context
}

func (s *stubServerStream) Context() context.Context { return s.base }

// A stub client stream carrying what the interceptor put on it.
type stubClientStream struct {
	grpc.ClientStream
	ctx    context.Context
	header metadata.MD
}

func (s *stubClientStream) Header() (metadata.MD, error) { return s.header, nil }
func (s *stubClientStream) Context() context.Context     { return s.ctx }

// TestGRPCServerStreamSpanCoversTheStreamLifetime pins
// otel.grpc-server-stream-span: one SERVER span named after the full method,
// alive for the handler's whole run, Error when the handler fails, and the
// handler receiving the span context — not the bare stream context.
func TestGRPCServerStreamSpanCoversTheStreamLifetime(t *testing.T) {
	tracer, recorder := newRecordingTracer(t)
	interceptor := oteladapter.GRPCServerStreamSpanInterceptor(tracer)

	var handlerSaw trace.SpanID
	handlerErr := context.DeadlineExceeded
	err := interceptor(nil,
		&stubServerStream{base: context.Background()},
		&grpc.StreamServerInfo{FullMethod: "/test.SpanService/Watch"},
		func(_ any, stream grpc.ServerStream) error {
			handlerSaw = trace.SpanContextFromContext(stream.Context()).SpanID()
			return handlerErr
		},
	)
	if err != handlerErr {
		t.Fatalf("handler error surfaced as %v, want the original", err)
	}

	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("spans = %d, want one", len(ended))
	}
	stub := ended[0]
	if stub.Name() != "/test.SpanService/Watch" || stub.SpanKind() != trace.SpanKindServer {
		t.Fatalf("span = %q/%v, want the SERVER method span", stub.Name(), stub.SpanKind())
	}
	if stub.Status().Code != otelcodes.Error {
		t.Fatalf("status = %v, want Error for a failed stream", stub.Status().Code)
	}
	if handlerSaw != stub.SpanContext().SpanID() {
		t.Fatalf("handler saw span %s, want the SERVER span %s", handlerSaw, stub.SpanContext().SpanID())
	}
}

// TestGRPCClientStreamSpanPublishesItsIdentity pins otel.grpc-client-stream-span:
// the CLIENT span starts before the stream is created, its identity is
// published to the correlation (so the transport's default traceparent
// injector names it), and the returned stream carries the span context.
func TestGRPCClientStreamSpanPublishesItsIdentity(t *testing.T) {
	tracer, recorder := newRecordingTracer(t)
	spanInterceptor := oteladapter.GRPCClientStreamSpanInterceptor(tracer)

	var injected string
	// The transport's traceparent injector is unary-shaped; drive it with
	// the span context the interceptor put on the streamer's context, as a
	// chained unary interceptor would sit between it and the wire.
	streamer := func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		// The transport's injector builds its own outgoing context and
		// hands it to its invoker; capture the header there, which is
		// exactly what the wire would see.
		injector := transportgrpc.TraceparentUnaryClientInterceptor()
		err := injector(ctx, "x", nil, nil, nil,
			func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
				md, _ := metadata.FromOutgoingContext(ctx)
				if values := md.Get("traceparent"); len(values) == 1 {
					injected = values[0]
				}
				return nil
			})
		if err != nil {
			return nil, err
		}
		return &stubClientStream{ctx: ctx, header: metadata.MD{}}, nil
	}

	clientStream, err := spanInterceptor(context.Background(), &grpc.StreamDesc{StreamName: "Watch"},
		nil, "/test.SpanService/Watch", streamer)
	if err != nil {
		t.Fatalf("streamer error surfaced: %v", err)
	}
	if clientStream == nil {
		t.Fatal("no client stream returned")
	}

	ended := recorder.Ended()
	if len(ended) != 1 || ended[0].SpanKind() != trace.SpanKindClient {
		t.Fatalf("spans = %d, want exactly the client span", len(ended))
	}
	sc := ended[0].SpanContext()
	if want := "00-" + sc.TraceID().String() + "-" + sc.SpanID().String() + "-01"; injected != want {
		t.Fatalf("traceparent = %q, want the span the interceptor started %q", injected, want)
	}
	if sc := trace.SpanContextFromContext(clientStream.Context()); sc.SpanID() != ended[0].SpanContext().SpanID() {
		t.Fatalf("stream context span = %s, want the CLIENT span %s", sc.SpanID(), ended[0].SpanContext().SpanID())
	}
}
