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

// TestHealthWatchReportsTheCurrentStatusThenChanges is the gap Milestone 14 declared
// away and got wrong: grpc-go's own client-side health checking calls Watch, not
// Check, and treats UNIMPLEMENTED as "healthy, stop asking" — so a drain never
// reached the clients that were watching for it.
func TestHealthWatchReportsTheCurrentStatusThenChanges(t *testing.T) {
	ready := &atomic.Bool{}
	ready.Store(true)

	component := MustNew("127.0.0.1:0")
	if err := component.Probes().AddReadiness("warmup", func(context.Context) error {
		if ready.Load() {
			return nil
		}
		return errors.New("no longer ready")
	}); err != nil {
		t.Fatalf("add readiness: %v", err)
	}
	client := startAndDial(t, component)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	stream, err := client.Watch(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("first message: %v", err)
	}
	if first.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("first status = %v, want SERVING immediately", first.GetStatus())
	}

	ready.Store(false)
	next, err := stream.Recv()
	if err != nil {
		t.Fatalf("message after readiness failed: %v", err)
	}
	if next.GetStatus() != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("status = %v, want NOT_SERVING", next.GetStatus())
	}

	// And back again: a watcher follows the process rather than latching.
	ready.Store(true)
	back, err := stream.Recv()
	if err != nil {
		t.Fatalf("message after recovery: %v", err)
	}
	if back.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("status = %v, want SERVING again", back.GetStatus())
	}
}

// TestHealthWatchRejectsAPerServiceRequest keeps Watch and Check answering the same
// question: the registry describes the process, so a named service has no honest
// answer here either.
func TestHealthWatchRejectsAPerServiceRequest(t *testing.T) {
	client := startAndDial(t, MustNew("127.0.0.1:0"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.Watch(ctx, &grpc_health_v1.HealthCheckRequest{Service: "catalog.Catalog"})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if _, err := stream.Recv(); status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want %v", status.Code(err), codes.NotFound)
	}
}

// TestHealthWatchStopsPollingWhenTheLastWatcherLeaves: a service nobody watches must
// not keep evaluating readiness checks, which can be database pings.
func TestHealthWatchStopsPollingWhenTheLastWatcherLeaves(t *testing.T) {
	var evaluations atomic.Int64
	component := MustNew("127.0.0.1:0")
	if err := component.Probes().AddReadiness("counted", func(context.Context) error {
		evaluations.Add(1)
		return nil
	}); err != nil {
		t.Fatalf("add readiness: %v", err)
	}
	client := startAndDial(t, component)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	stream, err := client.Watch(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("first message: %v", err)
	}
	time.Sleep(3 * HealthWatchInterval)
	cancel()

	// Give the poller a couple of intervals to notice the watcher is gone, then
	// check it has stopped evaluating.
	time.Sleep(3 * HealthWatchInterval)
	settled := evaluations.Load()
	time.Sleep(3 * HealthWatchInterval)
	if grown := evaluations.Load() - settled; grown != 0 {
		t.Errorf("readiness was evaluated %d more times after the last watcher left", grown)
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
