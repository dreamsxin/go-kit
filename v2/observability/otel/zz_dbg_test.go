package oteladapter_test

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	endpoint "github.com/dreamsxin/go-kit/v2/endpoint"
	oteladapter "github.com/dreamsxin/go-kit/v2/observability/otel"
)

func TestZZDebugStreamCorrelation(t *testing.T) {
	tracer, recorder := newRecordingTracer(t)
	spanInterceptor := oteladapter.GRPCClientStreamSpanInterceptor(tracer)

	var sawCorrelation, sawTraceparent bool
	streamer := func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		tc := endpoint.TraceContextFromContext(ctx)
		sawCorrelation = tc.Valid()
		md, _ := metadata.FromOutgoingContext(ctx)
		sawTraceparent = len(md.Get("traceparent")) > 0
		return &stubClientStream{ctx: ctx, header: metadata.MD{}}, nil
	}

	_, err := spanInterceptor(context.Background(), &grpc.StreamDesc{StreamName: "W"}, nil, "/s.S/M", streamer)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("correlation valid=%v traceparent=%v ended=%d", sawCorrelation, sawTraceparent, len(recorder.Ended()))
}
