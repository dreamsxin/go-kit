package grpc

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// TestHealthServiceAnswersFromTheProbeRegistry is the gap this closes: before
// the registry was mountable, a gRPC-only service had no readiness surface, so
// an orchestrator had nothing to ask.
func TestHealthServiceAnswersFromTheProbeRegistry(t *testing.T) {
	ready := &atomic.Bool{}
	ready.Store(true)

	component := MustNew("127.0.0.1:0")
	if err := component.Probes().AddReadiness("warmup", func(context.Context) error {
		if ready.Load() {
			return nil
		}
		return errors.New("still warming up")
	}); err != nil {
		t.Fatalf("add readiness: %v", err)
	}
	client := startAndDial(t, component)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if response.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("status = %v, want SERVING", response.GetStatus())
	}

	ready.Store(false)
	response, err = client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Check after failure: %v", err)
	}
	if response.GetStatus() != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("status = %v, want NOT_SERVING", response.GetStatus())
	}
}

// TestHealthServiceRejectsAPerServiceQuestion: the registry describes the
// process, so the protocol's NotFound is the honest answer for a named service.
func TestHealthServiceRejectsAPerServiceQuestion(t *testing.T) {
	client := startAndDial(t, MustNew("127.0.0.1:0"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: "catalog.Catalog"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want %v", status.Code(err), codes.NotFound)
	}
}

// TestHealthServiceReportsReadinessOnlyKeepsLivenessOutOfTheAnswer: a gRPC
// health check answers "should I receive traffic", which is readiness.
func TestHealthServiceReportsReadinessOnly(t *testing.T) {
	component := MustNew("127.0.0.1:0")
	if err := component.Probes().AddLiveness("self", func(context.Context) error {
		return errors.New("liveness is not what a health check answers")
	}); err != nil {
		t.Fatalf("add liveness: %v", err)
	}
	client := startAndDial(t, component)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if response.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("status = %v, want SERVING", response.GetStatus())
	}
}

func TestProbesIsUsableOnANilComponent(t *testing.T) {
	var component *Component
	if component.Probes() != nil {
		t.Fatal("expected a nil registry")
	}
}

func startAndDial(t *testing.T, component *Component) grpc_health_v1.HealthClient {
	t.Helper()
	if err := component.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = component.Shutdown(ctx)
	})

	connection, err := googlegrpc.NewClient(
		component.Addr().String(),
		googlegrpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return grpc_health_v1.NewHealthClient(connection)
}
