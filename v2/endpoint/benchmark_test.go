package endpoint_test

import (
	"context"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// The benchmarks in this file cover the paths every request crosses, so an
// optimization can be shown to work and a regression can be seen.

func BenchmarkChainEmpty(b *testing.B) {
	ctx := context.Background()
	ep := endpoint.Endpoint(endpoint.Nop)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ep(ctx, struct{}{}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkChainFiveMiddlewares is the shape a production endpoint has: the
// difference against BenchmarkChainEmpty is what the chain itself costs.
func BenchmarkChainFiveMiddlewares(b *testing.B) {
	ctx := context.Background()
	metrics := &endpoint.Metrics{}
	ep := endpoint.Chain(
		endpoint.RecoveryMiddleware(nil),
		endpoint.RecordingMiddleware("bench", metrics),
		endpoint.TracingMiddleware(),
		endpoint.ValidationMiddleware(),
		endpoint.TimeoutMiddleware(time.Minute),
	)(endpoint.Nop)

	b.ReportAllocs()
	for b.Loop() {
		if _, err := ep(ctx, struct{}{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTracingMiddleware(b *testing.B) {
	ctx := context.Background()
	ep := endpoint.TracingMiddleware()(endpoint.Nop)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ep(ctx, struct{}{}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTracingMiddlewareJoiningATrace covers the other half of the
// middleware: continuing an inbound trace rather than minting one.
func BenchmarkTracingMiddlewareJoiningATrace(b *testing.B) {
	traceContext, ok := endpoint.ParseTraceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	if !ok {
		b.Fatal("ParseTraceparent rejected a valid header")
	}
	ctx := endpoint.WithTraceContext(context.Background(), traceContext)
	ep := endpoint.TracingMiddleware()(endpoint.Nop)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ep(ctx, struct{}{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMetricsObserve(b *testing.B) {
	ctx := context.Background()
	metrics := &endpoint.Metrics{}
	observation := endpoint.Observation{Operation: "GET /users", Duration: time.Millisecond}
	b.ReportAllocs()
	for b.Loop() {
		metrics.Observe(ctx, observation)
	}
}

// BenchmarkMetricsObserveConcurrent reports whether recording contends: the
// collector is one value shared by every route of a service.
func BenchmarkMetricsObserveConcurrent(b *testing.B) {
	for _, operations := range []int{1, 16} {
		b.Run(operationLabel(operations), func(b *testing.B) {
			ctx := context.Background()
			metrics := &endpoint.Metrics{}
			labels := make([]string, operations)
			for i := range labels {
				labels[i] = "GET /route/" + string(rune('a'+i%26))
			}
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					metrics.Observe(ctx, endpoint.Observation{
						Operation: labels[i%len(labels)],
						Duration:  time.Millisecond,
					})
					i++
				}
			})
		})
	}
}

// BenchmarkMetricsSnapshotUnderLoad measures the scrape path while requests are
// being recorded, which is the situation a metrics endpoint creates.
func BenchmarkMetricsSnapshotUnderLoad(b *testing.B) {
	ctx := context.Background()
	metrics := &endpoint.Metrics{}
	for i := 0; i < 32; i++ {
		metrics.Observe(ctx, endpoint.Observation{Operation: "GET /route/" + string(rune('a'+i%26))})
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				metrics.Observe(ctx, endpoint.Observation{Operation: "GET /users", Duration: time.Millisecond})
			}
		}
	}()
	b.ReportAllocs()
	for b.Loop() {
		_ = metrics.Operations()
		_ = metrics.Snapshot()
	}
	b.StopTimer()
	close(stop)
	<-done
}

func operationLabel(operations int) string {
	if operations == 1 {
		return "one-operation"
	}
	return "many-operations"
}
