package grpc

import (
	"context"
	"time"

	"github.com/dreamsxin/go-kit/v2/kit"
	googlegrpc "google.golang.org/grpc"
)

// Draining a gRPC component.
//
// Milestone 11 declared stopping as a sequence: announce, wait, tear down. The
// announcement reached HTTP handlers and stopped there, which left a gRPC stream
// with no way to learn the process was going away and left Host.Drain silently
// skipping this component. Both are fixed here, and the part gRPC cannot promise is
// stated rather than implied.

// Component implements kit.Draining, so a Host announces the stop to it in the same
// reverse-attachment order it uses for every other component.
var _ kit.Draining = (*Component)(nil)

// callPollInterval is how often Shutdown re-checks whether the in-flight calls have
// finished. It is short enough not to add noticeable latency to a clean stop and long
// enough not to spin.
const callPollInterval = 5 * time.Millisecond

// Drain announces the stop to this component's handlers. The server keeps serving:
// draining tells the calls in flight that the process is going away, and the grace
// period they are given belongs to Shutdown.
//
// Refusing new RPCs is deliberately not part of it. A client that gets an
// UNAVAILABLE during the drain delay has been told to retry — at an instance that
// may be the same one, because the routing layer has not caught up yet. Failing
// readiness is the honest signal, and that is what the drain probe does.
//
// Stable: grpc.drains — the gRPC component closes its stopping signal when the process drains, while it is still serving.
// Covered by: TestDrainTellsAStreamTheProcessIsStopping, TestDrainKeepsServing
func (c *Component) Drain(context.Context) error {
	if c == nil {
		return nil
	}
	c.announceStopping()
	return nil
}

func (c *Component) announceStopping() {
	c.stopOnce.Do(func() { close(c.stopping) })
}

// lifecycleInterceptors carry the stopping signal into every handler context and
// count the calls in flight. They are installed ahead of the caller's options, and
// ahead of the trace interceptors, so a handler that never reaches the application
// layer is still counted and still has the signal.
//
// The count is what makes a bounded shutdown possible without grpc's GracefulStop:
// see waitForCalls.
func (c *Component) lifecycleInterceptors() []googlegrpc.ServerOption {
	stopping := c.stopping
	return []googlegrpc.ServerOption{
		googlegrpc.ChainUnaryInterceptor(func(ctx context.Context, request any, info *googlegrpc.UnaryServerInfo, handler googlegrpc.UnaryHandler) (any, error) {
			c.inFlight.Add(1)
			defer c.inFlight.Add(-1)
			return handler(kit.WithStopping(ctx, stopping), request)
		}),
		googlegrpc.ChainStreamInterceptor(func(server any, stream googlegrpc.ServerStream, info *googlegrpc.StreamServerInfo, handler googlegrpc.StreamHandler) error {
			c.inFlight.Add(1)
			defer c.inFlight.Add(-1)
			return handler(server, &stoppingStream{ServerStream: stream, ctx: kit.WithStopping(stream.Context(), stopping)})
		}),
	}
}

// waitForCalls waits until no call is in flight, or until ctx expires, and reports
// how many were still running.
//
// This is the graceful wait, and it is ours rather than grpc's on purpose.
// GracefulStop holds the server's own mutex while it waits for handlers to return,
// and Stop needs that mutex — so a handler that never returns makes GracefulStop
// wait forever and Stop unable to interrupt it. Counting the calls ourselves keeps
// the budget meaningful: when it expires we can still close the transports, which
// is what Stop does when it has not been blocked first.
func (c *Component) waitForCalls(ctx context.Context) int64 {
	ticker := time.NewTicker(callPollInterval)
	defer ticker.Stop()
	for {
		if remaining := c.inFlight.Load(); remaining == 0 {
			return 0
		}
		select {
		case <-ctx.Done():
			return c.inFlight.Load()
		case <-ticker.C:
		}
	}
}

// stoppingStream replaces a stream's context with one carrying the signal.
// grpc.ServerStream has no setter, so wrapping is the only way; every other method
// is the embedded stream's.
type stoppingStream struct {
	googlegrpc.ServerStream
	ctx context.Context
}

func (s *stoppingStream) Context() context.Context { return s.ctx }
