// Package grpc provides an optional gRPC lifecycle component for kit.Host.
package grpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/dreamsxin/go-kit/v2/health"
	transportgrpc "github.com/dreamsxin/go-kit/v2/integrations/grpc"
	"github.com/dreamsxin/go-kit/v2/kit"
	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
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
	serverOptions := append([]googlegrpc.ServerOption{
		googlegrpc.ChainUnaryInterceptor(transportgrpc.TraceparentUnaryServerInterceptor()),
		googlegrpc.ChainStreamInterceptor(transportgrpc.TraceparentStreamServerInterceptor()),
	}, options...)
	component := &Component{
		addr:   addr,
		server: googlegrpc.NewServer(serverOptions...),
		probes: health.NewRegistry(),
		errors: make(chan error, 1),
	}
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

// healthService answers grpc.health.v1.Health from a probe registry.
//
// Check evaluates the readiness scope on every call rather than reading a status
// somebody remembered to set, so the answer cannot go stale. Watch is not
// implemented: it would have to poll the checks to synthesise transitions, and
// the tools that orchestrate on gRPC health — grpc_health_probe and Kubernetes'
// native gRPC probe — call Check.
type healthService struct {
	grpc_health_v1.UnimplementedHealthServer
	probes *health.Registry
}

func (s *healthService) Check(ctx context.Context, request *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	// The registry describes the process, not one service within it, so a
	// per-service question has no honest answer here. The protocol's own reply
	// for that is NotFound.
	if request != nil && request.GetService() != "" {
		return nil, status.Error(codes.NotFound, "kit/grpc: readiness is reported for the process, not per service")
	}
	serving := grpc_health_v1.HealthCheckResponse_SERVING
	if s.probes.Report(ctx, health.ScopeReadiness).Status != health.StatusOK {
		serving = grpc_health_v1.HealthCheckResponse_NOT_SERVING
	}
	return &grpc_health_v1.HealthCheckResponse{Status: serving}, nil
}

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
		if err := c.server.Serve(listener); err != nil && !errors.Is(err, googlegrpc.ErrServerStopped) {
			select {
			case c.errors <- fmt.Errorf("kit/grpc: serve: %w", err):
			default:
			}
		}
	}()
	return nil
}

// Errors reports asynchronous serving failures after Start.
func (c *Component) Errors() <-chan error {
	if c == nil {
		return nil
	}
	return c.errors
}

// Shutdown gracefully stops the gRPC server until ctx expires.
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
	c.mu.Unlock()

	done := make(chan struct{})
	go func() {
		c.server.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		c.server.Stop()
		return ctx.Err()
	}
}
