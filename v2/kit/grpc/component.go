// Package grpc provides an optional gRPC lifecycle component for kit.Host.
//
// The component answers the same operational questions the HTTP component does: it
// serves the standard gRPC health service from a probe registry, it is told when the
// process begins draining, and it carries the stopping signal into every handler
// context so a stream can end itself. See drain.go for what it can and cannot
// promise about stopping.
package grpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/dreamsxin/go-kit/v2/health"
	transportgrpc "github.com/dreamsxin/go-kit/v2/integrations/grpc"
	"github.com/dreamsxin/go-kit/v2/kit"
	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// Component implements kit.Lifecycle; the assertion keeps the contract honest
// at compile time, in this module, instead of inside an application build.
var _ kit.Lifecycle = (*Component)(nil)

// Component also serves readiness, so a Host bridges lifecycle readiness into
// it the same way it does for the HTTP component.
var _ kit.ReadinessSink = (*Component)(nil)

// Component owns a gRPC server and implements kit.Lifecycle. Register services
// through Server before attaching the component to a kit.Host.
//
// The standard gRPC health service is registered on the server, answering
// grpc.health.v1.Health/Check from the component's probe registry. That is what
// grpc_health_probe and Kubernetes' native gRPC probe call, so a gRPC-only
// service is orchestrated on the same answer an HTTP service serves at /readyz.
type Component struct {
	addr   string
	server *googlegrpc.Server
	probes *health.Registry
	errors chan error

	// stopping closes when the process announces that it is going away, so a
	// long-lived call can end itself instead of being cut. See drain.go.
	stopping chan struct{}
	stopOnce sync.Once
	inFlight atomic.Int64

	mu       sync.Mutex
	listener net.Listener
	started  bool
	stopped  bool
}

// New creates a gRPC lifecycle component listening on addr.
//
// The server extracts the incoming W3C trace context without extra wiring, so
// a gRPC service continues a trace the same way an HTTP one does. The
// interceptors are chained, which leaves grpc.UnaryInterceptor and
// grpc.ChainUnaryInterceptor free for the caller.
func New(addr string, options ...googlegrpc.ServerOption) (*Component, error) {
	if strings.TrimSpace(addr) == "" {
		return nil, fmt.Errorf("kit/grpc: address cannot be empty")
	}
	for i, option := range options {
		if option == nil {
			return nil, fmt.Errorf("kit/grpc: server option %d is nil", i)
		}
	}
	component := &Component{
		addr:     addr,
		probes:   health.NewRegistry(),
		errors:   make(chan error, 1),
		stopping: make(chan struct{}),
	}
	// The stopping signal and the call count are installed first, then trace
	// extraction, then the caller's options: a handler reached through any of them
	// has the signal and is counted.
	serverOptions := append(component.lifecycleInterceptors(),
		googlegrpc.ChainUnaryInterceptor(transportgrpc.TraceparentUnaryServerInterceptor()),
		googlegrpc.ChainStreamInterceptor(transportgrpc.TraceparentStreamServerInterceptor()),
	)
	serverOptions = append(serverOptions, options...)
	component.server = googlegrpc.NewServer(serverOptions...)
	grpc_health_v1.RegisterHealthServer(component.server, &healthService{probes: component.probes})
	return component, nil
}

// MustNew creates a Component and panics if its configuration is invalid.
func MustNew(addr string, options ...googlegrpc.ServerOption) *Component {
	component, err := New(addr, options...)
	if err != nil {
		panic(err)
	}
	return component
}

// Server returns the underlying gRPC server for generated service registration.
func (c *Component) Server() *googlegrpc.Server {
	if c == nil {
		return nil
	}
	return c.server
}

// Probes returns the component's probe registry, the source the gRPC health
// service answers from. A Host bridges lifecycle readiness into it; an
// application can add a readiness check of its own.
func (c *Component) Probes() *health.Registry {
	if c == nil {
		return nil
	}
	return c.probes
}

// The health service and its Check and Watch methods live in health.go.

// Addr returns the bound listener address after Start, or nil before Start.
func (c *Component) Addr() net.Addr {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.listener == nil {
		return nil
	}
	return c.listener.Addr()
}

// Start binds the listener synchronously and serves in the background.
func (c *Component) Start() error {
	if c == nil {
		return fmt.Errorf("kit/grpc: nil component")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return fmt.Errorf("kit/grpc: component already started")
	}
	if c.stopped {
		return fmt.Errorf("kit/grpc: component cannot be restarted after shutdown")
	}

	listener, err := net.Listen("tcp", c.addr)
	if err != nil {
		return fmt.Errorf("kit/grpc: listen: %w", err)
	}
	c.listener = listener
	c.started = true
	go func() {
		err := c.server.Serve(listener)
		if err == nil || errors.Is(err, googlegrpc.ErrServerStopped) {
			return
		}
		// Shutdown closes the listener to stop new connections arriving, so Serve
		// returns net.ErrClosed on the way out. That is the stop working, not a
		// failure to report.
		if errors.Is(err, net.ErrClosed) && c.stoppingBegun() {
			return
		}
		select {
		case c.errors <- fmt.Errorf("kit/grpc: serve: %w", err):
		default:
		}
	}()
	return nil
}

// stoppingBegun reports whether the stop has been announced, which is what makes a
// closed listener expected rather than a fault.
func (c *Component) stoppingBegun() bool {
	select {
	case <-c.stopping:
		return true
	default:
		return false
	}
}

// Errors reports asynchronous serving failures after Start.
func (c *Component) Errors() <-chan error {
	if c == nil {
		return nil
	}
	return c.errors
}

// Shutdown stops the gRPC server: it announces, waits for the calls in flight until
// ctx expires, and then closes the transports.
//
// The graceful wait is this package's, not grpc's GracefulStop. GracefulStop holds
// the server's own mutex while it waits for handlers to return, and Stop needs that
// mutex, so a handler that never returns makes GracefulStop wait forever and leaves
// Stop unable to interrupt it — a bounded shutdown built on the pair is not bounded
// at all. Counting the calls through the component's own interceptors keeps the
// budget meaningful: when it runs out, Stop has not been blocked first and can still
// close the connections.
//
// What is left after that is a handler goroutine that watched neither its context
// nor kit.Stopping. Its connection is gone; the goroutine ends when the process does.
//
// Stable: grpc.shutdown-ends — Shutdown announces the stop, gives in-flight RPCs until ctx expires, then closes the transports and reports how many calls it interrupted rather than returning success.
// Covered by: TestShutdownStopsAStreamThatIgnoresTheSignal, TestShutdownWaitsForACallToFinish
func (c *Component) Shutdown(ctx context.Context) error {
	if c == nil {
		return nil
	}
	if ctx == nil {
		return fmt.Errorf("kit/grpc: nil shutdown context")
	}
	c.mu.Lock()
	if !c.started {
		c.mu.Unlock()
		return nil
	}
	c.started = false
	c.stopped = true
	listener := c.listener
	c.mu.Unlock()

	c.announceStopping()
	// Closing the listener stops new connections from arriving while the calls
	// already here finish. Serve returns as a result, which Start reports as an
	// expected stop rather than a failure.
	if listener != nil {
		_ = listener.Close()
	}

	interrupted := c.waitForCalls(ctx)
	c.server.Stop()
	if interrupted == 0 {
		return nil
	}
	return fmt.Errorf("%w: closed the listener and %d call(s) still in flight: %w",
		kit.ErrShutdownIncomplete, interrupted, ctx.Err())
}
