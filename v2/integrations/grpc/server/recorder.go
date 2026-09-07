package server

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Recording an RPC.
//
// This mirrors transport/http/server: the transport reports protocol facts, and
// something else decides what they mean. What is deliberately different is the
// label. HTTP had to work to get a bounded one — the matched route pattern, only
// readable inside the mux — while gRPC gets it for free: the full method name comes
// from the service definition, so the set of series is fixed at compile time.

// Observation is one RPC handed to a Recorder. It carries protocol facts only: the
// method, the status code the server returned, and how long it took. What the call
// meant is the endpoint layer's to report.
//
// Unstable: grpc.observation-fields — what an Observation carries is a metrics input, not a wire promise, and grows as the transport learns more.
type Observation struct {
	// Method is the full method name, "/package.Service/Method".
	Method string
	// Code is the gRPC status code the server returned. codes.OK is a success.
	Code codes.Code
	// Stream reports whether this was a streaming call. A stream's duration is
	// its whole lifetime, which is a different quantity from a unary latency and
	// should not be averaged together with one.
	Stream bool
	// Duration is how long the handler took, measured around it.
	Duration time.Duration
}

// Recorder receives one Observation per RPC. Implement it to report gRPC server
// metrics to a backend.
//
// ObserveGRPC runs on the request path and must not block.
type Recorder interface {
	ObserveGRPC(ctx context.Context, obs Observation)
}

// RecorderFunc adapts a function to Recorder.
type RecorderFunc func(ctx context.Context, obs Observation)

// ObserveGRPC implements Recorder.
func (f RecorderFunc) ObserveGRPC(ctx context.Context, obs Observation) { f(ctx, obs) }

// RecordingUnaryInterceptor reports one Observation per unary call to each
// recorder, in the order they were passed.
//
// It panics when a recorder is nil so misassembly fails at startup rather than on
// the first call.
//
// Stable: grpc.recording-method-label — a gRPC observation is labelled with the full method name from the service definition, so the series a scrape returns are bounded by the contract rather than by traffic.
// Covered by: TestRecordingUnaryInterceptorReportsTheFullMethod
func RecordingUnaryInterceptor(recorders ...Recorder) grpc.UnaryServerInterceptor {
	observers := checkedRecorders(recorders)
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if len(observers) == 0 {
			return handler(ctx, request)
		}
		start := time.Now()
		response, err := handler(ctx, request)
		observe(ctx, observers, Observation{
			Method:   info.FullMethod,
			Code:     status.Code(err),
			Duration: time.Since(start),
		})
		return response, err
	}
}

// RecordingStreamInterceptor reports one Observation per stream, when the stream
// ends. A stream that never ends is never recorded, which is the honest answer: its
// duration is not known until it is over.
func RecordingStreamInterceptor(recorders ...Recorder) grpc.StreamServerInterceptor {
	observers := checkedRecorders(recorders)
	return func(server any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if len(observers) == 0 {
			return handler(server, stream)
		}
		start := time.Now()
		err := handler(server, stream)
		observe(stream.Context(), observers, Observation{
			Method:   info.FullMethod,
			Code:     status.Code(err),
			Stream:   true,
			Duration: time.Since(start),
		})
		return err
	}
}

func checkedRecorders(recorders []Recorder) []Recorder {
	for _, recorder := range recorders {
		if recorder == nil {
			panic("grpc server: recorder cannot be nil")
		}
	}
	return append([]Recorder(nil), recorders...)
}

func observe(ctx context.Context, recorders []Recorder, obs Observation) {
	for _, recorder := range recorders {
		recorder.ObserveGRPC(ctx, obs)
	}
}
