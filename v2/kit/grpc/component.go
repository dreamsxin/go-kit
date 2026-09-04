// Package grpc provides an optional gRPC lifecycle component for kit.Host.
package grpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/dreamsxin/go-kit/v2/kit"
	googlegrpc "google.golang.org/grpc"
)

// Component implements kit.Lifecycle; the assertion keeps the contract honest
// at compile time, in this module, instead of inside an application build.
var _ kit.Lifecycle = (*Component)(nil)

// Component owns a gRPC server and implements kit.Lifecycle. Register services
// through Server before attaching the component to a kit.Host.
type Component struct {
	addr   string
	server *googlegrpc.Server
	errors chan error

	mu       sync.Mutex
	listener net.Listener
	started  bool
	stopped  bool
}

// New creates a gRPC lifecycle component listening on addr.
func New(addr string, options ...googlegrpc.ServerOption) (*Component, error) {
	if strings.TrimSpace(addr) == "" {
		return nil, fmt.Errorf("kit/grpc: address cannot be empty")
	}
	for i, option := range options {
		if option == nil {
			return nil, fmt.Errorf("kit/grpc: server option %d is nil", i)
		}
	}
	return &Component{
		addr:   addr,
		server: googlegrpc.NewServer(options...),
		errors: make(chan error, 1),
	}, nil
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
