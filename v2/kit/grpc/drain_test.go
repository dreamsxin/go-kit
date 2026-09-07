package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/kit"
	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	testStreamService = "kit.grpc.test.Streamer"
	testStreamMethod  = "/kit.grpc.test.Streamer/Watch"
)

// registerStreamer installs a streaming method without generated code: the test
// needs a handler whose context it can inspect, not a message shape.
func registerStreamer(component *Component, handler func(stream googlegrpc.ServerStream) error) {
	component.Server().RegisterService(&googlegrpc.ServiceDesc{
		ServiceName: testStreamService,
		HandlerType: (*any)(nil),
		Streams: []googlegrpc.StreamDesc{{
			StreamName:    "Watch",
			ServerStreams: true,
			ClientStreams: true,
			Handler: func(_ any, stream googlegrpc.ServerStream) error {
				return handler(stream)
			},
		}},
		Metadata: "kit/grpc/drain_test.go",
	}, nil)
}

func dialComponent(t *testing.T, component *Component) *googlegrpc.ClientConn {
	t.Helper()
	conn, err := googlegrpc.NewClient(component.Addr().String(), googlegrpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func openStream(t *testing.T, conn *googlegrpc.ClientConn) googlegrpc.ClientStream {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	stream, err := conn.NewStream(ctx, &googlegrpc.StreamDesc{ClientStreams: true, ServerStreams: true}, testStreamMethod)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	return stream
}

// TestDrainTellsAStreamTheProcessIsStopping is the asymmetry this closes: before
// this, Host.Drain type-asserted every component to kit.Draining and silently
// skipped the gRPC one, so a stream never learned the process was going away.
func TestDrainTellsAStreamTheProcessIsStopping(t *testing.T) {
	component := MustNew("127.0.0.1:0")
	told := make(chan bool, 1)
	registerStreamer(component, func(stream googlegrpc.ServerStream) error {
		select {
		case <-kit.Stopping(stream.Context()):
			told <- true
		case <-time.After(3 * time.Second):
			told <- false
		}
		return nil
	})
	if err := component.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = component.Shutdown(context.Background()) }()

	stream := openStream(t, dialComponent(t, component))
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}

	host := kit.MustNewHost(kit.WithLifecycle(component))
	if err := host.Drain(context.Background()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	select {
	case stopped := <-told:
		if !stopped {
			t.Fatal("the stream handler was never told the process is stopping")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the stream handler did not react to the announcement")
	}
}

// TestDrainKeepsServing pins the half of the contract that is easy to break by
// being helpful: draining announces, it does not close the door. Refusing calls
// here would fail requests that were already routed to this instance, which is the
// problem the drain delay exists to avoid.
func TestDrainKeepsServing(t *testing.T) {
	component := MustNew("127.0.0.1:0")
	served := make(chan struct{}, 2)
	registerStreamer(component, func(googlegrpc.ServerStream) error {
		served <- struct{}{}
		return nil
	})
	if err := component.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = component.Shutdown(context.Background()) }()

	if err := component.Drain(context.Background()); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	conn := dialComponent(t, component)
	stream := openStream(t, conn)
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}
	select {
	case <-served:
	case <-time.After(5 * time.Second):
		t.Fatal("a call opened after the announcement was not served")
	}
}

// TestShutdownStopsAStreamThatIgnoresTheSignal states the limit, and it is the test
// that found the real one: building a bounded stop on grpc's GracefulStop plus Stop
// deadlocks, because GracefulStop holds the server mutex while waiting for handlers
// and Stop needs it. Shutdown counts calls itself instead, so the budget means
// something and the connections are closed when it expires.
func TestShutdownStopsAStreamThatIgnoresTheSignal(t *testing.T) {
	component := MustNew("127.0.0.1:0")
	release := make(chan struct{})
	defer close(release)
	registerStreamer(component, func(googlegrpc.ServerStream) error {
		<-release
		return nil
	})
	if err := component.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	stream := openStream(t, dialComponent(t, component))
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}
	// The handler is in flight before shutdown starts, which is what makes the
	// graceful wait wait rather than return immediately.
	var message []byte
	go func() { _ = stream.RecvMsg(&message) }()
	waitForCallsInFlight(t, component, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := component.Shutdown(ctx)
	if err == nil {
		t.Fatal("Shutdown = nil, want an incomplete-shutdown error: a stream ignoring the signal was still open")
	}
	if !errors.Is(err, kit.ErrShutdownIncomplete) {
		t.Errorf("Shutdown error = %v, want it to wrap kit.ErrShutdownIncomplete", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("Shutdown took %s, want it bounded by the context", elapsed)
	}
}

// TestShutdownWaitsForACallToFinish is the other half: a call that does return is
// waited for, and Shutdown reports success rather than an interrupted stop.
func TestShutdownWaitsForACallToFinish(t *testing.T) {
	component := MustNew("127.0.0.1:0")
	registerStreamer(component, func(googlegrpc.ServerStream) error {
		time.Sleep(150 * time.Millisecond)
		return nil
	})
	if err := component.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	stream := openStream(t, dialComponent(t, component))
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}
	var message []byte
	go func() { _ = stream.RecvMsg(&message) }()
	waitForCallsInFlight(t, component, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := component.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown = %v, want nil: the call finished inside the budget", err)
	}
	select {
	case err := <-component.Errors():
		t.Fatalf("closing the listener was reported as a serve failure: %v", err)
	default:
	}
}

// waitForCallsInFlight blocks until the component reports the expected number of
// calls in flight, so a test does not race the client's stream setup.
func waitForCallsInFlight(t *testing.T, component *Component, want int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if component.inFlight.Load() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("in-flight calls = %d, want %d", component.inFlight.Load(), want)
}
