package grpc

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	transportgrpc "github.com/dreamsxin/go-kit/v2/integrations/grpc"
)

// TestServerExtractsTraceparentWithoutWiring is the property WP4 asks for: a
// gRPC service continues an incoming trace with no interceptor the application
// had to remember. The probe check is the observation point because it runs
// with the request's context.
func TestServerExtractsTraceparentWithoutWiring(t *testing.T) {
	const (
		traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
		spanID  = "00f067aa0ba902b7"
	)
	seen := &atomic.Value{}
	seen.Store("")

	component := MustNew("127.0.0.1:0")
	if err := component.Probes().AddReadiness("trace", func(ctx context.Context) error {
		seen.Store(endpoint.TraceContextFromContext(ctx).TraceID)
		return nil
	}); err != nil {
		t.Fatalf("add readiness: %v", err)
	}
	client := startAndDial(t, component)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, transportgrpc.TraceparentKey,
		"00-"+traceID+"-"+spanID+"-01")

	if _, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{}); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := seen.Load().(string); got != traceID {
		t.Fatalf("trace ID reaching the handler = %q, want %q", got, traceID)
	}
}

// TestServerAcceptsCallerInterceptors keeps the default from claiming the one
// interceptor slot: chaining is what leaves grpc.UnaryInterceptor free.
func TestServerAcceptsCallerInterceptors(t *testing.T) {
	calls := &atomic.Int64{}
	counting := func(
		ctx context.Context,
		req any,
		_ *googlegrpc.UnaryServerInfo,
		handler googlegrpc.UnaryHandler,
	) (any, error) {
		calls.Add(1)
		return handler(ctx, req)
	}
	component, err := New("127.0.0.1:0", googlegrpc.UnaryInterceptor(counting))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	client := startAndDial(t, component)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{}); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("caller interceptor calls = %d, want 1", calls.Load())
	}
}

