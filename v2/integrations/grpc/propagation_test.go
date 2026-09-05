package grpc

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

const (
	testTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	testSpanID  = "00f067aa0ba902b7"
	testParent  = "00-" + testTraceID + "-" + testSpanID + "-01"
)

func TestExtractTraceparentContinuesTheIncomingTrace(t *testing.T) {
	md := metadata.Pairs(TraceparentKey, testParent)
	ctx := ExtractTraceparent(context.Background(), md)

	traceContext := endpoint.TraceContextFromContext(ctx)
	if traceContext.TraceID != testTraceID || traceContext.SpanID != testSpanID {
		t.Fatalf("trace context = %#v", traceContext)
	}
	if traceContext.Flags != "01" {
		t.Fatalf("flags = %q, want %q — the sampling decision must survive the hop", traceContext.Flags, "01")
	}
}

func TestExtractTraceparentIgnoresMalformedAndMissingValues(t *testing.T) {
	for name, md := range map[string]metadata.MD{
		"missing":   metadata.MD{},
		"malformed": metadata.Pairs(TraceparentKey, "00-not-a-trace"),
		"all zero":  metadata.Pairs(TraceparentKey, "00-00000000000000000000000000000000-0000000000000000-01"),
	} {
		t.Run(name, func(t *testing.T) {
			ctx := ExtractTraceparent(context.Background(), md)
			if endpoint.TraceContextFromContext(ctx).Valid() {
				t.Fatal("a trace context was invented from an unusable header")
			}
		})
	}
}

func TestInjectTraceparentWritesOutgoingMetadata(t *testing.T) {
	ctx := endpoint.WithTraceContext(context.Background(), endpoint.TraceContext{
		TraceID: testTraceID,
		SpanID:  testSpanID,
		Flags:   "01",
	})
	md := metadata.MD{}
	InjectTraceparent(ctx, &md)

	if got := md.Get(TraceparentKey); len(got) != 1 || got[0] != testParent {
		t.Fatalf("metadata = %v, want [%s]", got, testParent)
	}
}

func TestInjectTraceparentSkipsAnAbsentTraceContext(t *testing.T) {
	md := metadata.MD{}
	InjectTraceparent(context.Background(), &md)
	if len(md.Get(TraceparentKey)) != 0 {
		t.Fatalf("metadata = %v, want empty", md)
	}
}

func TestTraceparentUnaryServerInterceptorReachesTheHandler(t *testing.T) {
	interceptor := TraceparentUnaryServerInterceptor()
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(TraceparentKey, testParent))

	var seen endpoint.TraceContext
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, _ any) (any, error) {
		seen = endpoint.TraceContextFromContext(ctx)
		return nil, nil
	})
	if err != nil {
		t.Fatalf("interceptor: %v", err)
	}
	if seen.TraceID != testTraceID {
		t.Fatalf("handler trace ID = %q, want %q", seen.TraceID, testTraceID)
	}
}

func TestTraceparentStreamServerInterceptorReachesTheHandler(t *testing.T) {
	interceptor := TraceparentStreamServerInterceptor()
	stream := &fakeServerStream{
		ctx: metadata.NewIncomingContext(context.Background(), metadata.Pairs(TraceparentKey, testParent)),
	}

	var seen endpoint.TraceContext
	err := interceptor(nil, stream, &grpc.StreamServerInfo{}, func(_ any, stream grpc.ServerStream) error {
		seen = endpoint.TraceContextFromContext(stream.Context())
		return nil
	})
	if err != nil {
		t.Fatalf("interceptor: %v", err)
	}
	if seen.TraceID != testTraceID {
		t.Fatalf("handler trace ID = %q, want %q", seen.TraceID, testTraceID)
	}
}

func TestTraceparentUnaryClientInterceptorReplacesRatherThanAppends(t *testing.T) {
	interceptor := TraceparentUnaryClientInterceptor()
	ctx := metadata.NewOutgoingContext(
		endpoint.WithTraceContext(context.Background(), endpoint.TraceContext{
			TraceID: testTraceID,
			SpanID:  testSpanID,
			Flags:   "01",
		}),
		metadata.Pairs(TraceparentKey, "00-11111111111111111111111111111111-2222222222222222-00"),
	)

	var sent metadata.MD
	err := interceptor(ctx, "/svc/Method", nil, nil, nil,
		func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			sent, _ = metadata.FromOutgoingContext(ctx)
			return nil
		})
	if err != nil {
		t.Fatalf("interceptor: %v", err)
	}
	// Two traceparent values are not a trace context; a receiver reads the
	// first and the second is a lie.
	if got := sent.Get(TraceparentKey); len(got) != 1 || got[0] != testParent {
		t.Fatalf("outgoing metadata = %v, want [%s]", got, testParent)
	}
}

type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *fakeServerStream) Context() context.Context { return s.ctx }
