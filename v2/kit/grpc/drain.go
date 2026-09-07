package grpc

import (
	"context"

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

// stoppingInterceptors carry the signal into every handler context. They are
// installed ahead of the caller's options, and ahead of the trace interceptors, so
// a handler that never reaches the application layer still has the signal.
func (c *Component) stoppingInterceptors() []googlegrpc.ServerOption {
	stopping := c.stopping
	return []googlegrpc.ServerOption{
		googlegrpc.ChainUnaryInterceptor(func(ctx context.Context, request any, info *googlegrpc.UnaryServerInfo, handler googlegrpc.UnaryHandler) (any, error) {
			return handler(kit.WithStopping(ctx, stopping), request)
		}),
		googlegrpc.ChainStreamInterceptor(func(server any, stream googlegrpc.ServerStream, info *googlegrpc.StreamServerInfo, handler googlegrpc.StreamHandler) error {
			return handler(server, &stoppingStream{ServerStream: stream, ctx: kit.WithStopping(stream.Context(), stopping)})
		}),
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
