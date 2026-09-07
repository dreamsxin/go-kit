// Package grpc bridges gRPC server observations into the metrics collector the
// exposition endpoint renders.
//
// It is a separate package from observability/metrics on purpose. That package is
// gated to endpoint and the HTTP transport so that mounting a scrape endpoint costs
// nothing but the standard library; bridging gRPC means importing the gRPC
// libraries, and an HTTP-only service should not acquire them by asking to be
// scraped. A service that speaks gRPC already depends on them, so it pays here.
package grpc

import (
	"context"
	"fmt"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	grpcserver "github.com/dreamsxin/go-kit/v2/integrations/grpc/server"
	"google.golang.org/grpc/codes"
)

// Recorder adapts an endpoint.Metrics collector to the gRPC transport's recorder,
// so the same collector a scrape renders is fed by RPCs as well as by HTTP
// requests.
//
//	collector := &endpoint.Metrics{}
//	component := kitgrpc.MustNew(":8081",
//	    grpc.ChainUnaryInterceptor(grpcserver.RecordingUnaryInterceptor(metricsgrpc.Recorder(collector))),
//	    grpc.ChainStreamInterceptor(grpcserver.RecordingStreamInterceptor(metricsgrpc.Recorder(collector))),
//	)
//	mux.Handle("GET /metrics", metrics.Handler(collector))
//
// The operation label is the full method name. Unlike an HTTP route it needs no
// defending: the set of methods is fixed by the service definition, and a call to a
// method that does not exist is refused by gRPC before an interceptor runs, so it
// cannot add a series.
//
// A nil collector is a misassembly rather than a silent no-op.
//
// Stable: metrics.grpc-bridge-error-class — an RPC that failed with a server-side status code is recorded as an error; a code that reports the caller was told no is not, because a refused call is not a broken server.
// Covered by: TestRecorderCountsServerErrorsOnly
func Recorder(collector *endpoint.Metrics) grpcserver.Recorder {
	if collector == nil {
		panic("metrics/grpc: collector cannot be nil")
	}
	return grpcserver.RecorderFunc(func(ctx context.Context, obs grpcserver.Observation) {
		collector.Observe(ctx, endpoint.Observation{
			Operation: obs.Method,
			Duration:  obs.Duration,
			Err:       outcome(obs.Code),
		})
	})
}

// outcome decides whether a status code means this service failed.
//
// The split follows the same reasoning as the HTTP bridge, where 5xx is the server
// failing and 4xx is the caller being told no. gRPC's codes do not divide as neatly,
// so the ones that describe a caller's request — or a caller's own cancellation —
// are named here and everything else counts as a failure. NotFound is a caller
// question with a legitimate answer; Internal is not.
func outcome(code codes.Code) error {
	switch code {
	case codes.OK,
		codes.Canceled,
		codes.InvalidArgument,
		codes.NotFound,
		codes.AlreadyExists,
		codes.PermissionDenied,
		codes.Unauthenticated,
		codes.FailedPrecondition,
		codes.OutOfRange:
		return nil
	default:
		return fmt.Errorf("grpc %s", code)
	}
}
