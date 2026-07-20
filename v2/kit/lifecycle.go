package kit

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"google.golang.org/grpc"
)

// DefaultShutdownTimeout is the graceful shutdown deadline used by Run.
const DefaultShutdownTimeout = 10 * time.Second

// Run starts the configured servers and blocks until ctx is cancelled or a
// server fails. Signal handling belongs to the calling main package.
func (s *Service) Run(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("kit: nil run context")
	}
	if err := s.Start(); err != nil {
		return err
	}

	var runErr error
	select {
	case <-ctx.Done():
		cause := context.Cause(ctx)
		if cause != nil && !errors.Is(cause, context.Canceled) {
			runErr = cause
		}
	case runErr = <-s.Errors():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()
	return errors.Join(runErr, s.Shutdown(shutdownCtx))
}

// Start starts the HTTP server (and gRPC server if enabled) in the background.
// It returns an error if either listener fails to bind.
func (s *Service) Start() error {
	if s == nil {
		return fmt.Errorf("kit: nil Service")
	}
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.started {
		return fmt.Errorf("kit: service already started")
	}
	if s.stopped {
		return fmt.Errorf("kit: service cannot be restarted after shutdown")
	}

	// Bind the HTTP listener synchronously so bind errors (e.g. port in use)
	// surface before we return to the caller.
	httpLis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("http listen: %w", err)
	}

	var grpcLis net.Listener
	var grpcServer *grpc.Server
	if s.grpcAddr != "" {
		grpcServer, err = s.grpcServerLocked()
		if err != nil {
			_ = httpLis.Close()
			return err
		}
		grpcLis, err = net.Listen("tcp", s.grpcAddr)
		if err != nil {
			_ = httpLis.Close()
			return fmt.Errorf("grpc listen: %w", err)
		}
	}

	s.srv = &http.Server{
		Addr:              s.addr,
		Handler:           s.httpHandler,
		ReadHeaderTimeout: s.httpConfig.ReadHeaderTimeout,
		ReadTimeout:       s.httpConfig.ReadTimeout,
		WriteTimeout:      s.httpConfig.WriteTimeout,
		IdleTimeout:       s.httpConfig.IdleTimeout,
		MaxHeaderBytes:    s.httpConfig.MaxHeaderBytes,
	}
	s.started = true
	go func() {
		s.logger.Sugar().Infof("HTTP listening on %s", s.addr)
		if err := s.srv.Serve(httpLis); err != nil && err != http.ErrServerClosed {
			s.reportServeError(fmt.Errorf("http serve: %w", err))
		}
	}()

	if grpcLis != nil {
		go func() {
			s.logger.Sugar().Infof("gRPC listening on %s", s.grpcAddr)
			if err := grpcServer.Serve(grpcLis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
				s.reportServeError(fmt.Errorf("grpc serve: %w", err))
			}
		}()
	}
	return nil
}

// Errors reports asynchronous HTTP or gRPC serving failures after Start.
// Listener bind failures are still returned directly from Start.
func (s *Service) Errors() <-chan error {
	return s.serveErrors
}

func (s *Service) reportServeError(err error) {
	s.logger.Sugar().Error(err)
	select {
	case s.serveErrors <- err:
	default:
	}
}

// Shutdown gracefully stops the HTTP server and gRPC server if running.
func (s *Service) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		return fmt.Errorf("kit: nil shutdown context")
	}
	s.lifecycleMu.Lock()
	if !s.started {
		s.lifecycleMu.Unlock()
		return nil
	}
	srv := s.srv
	grpcServer := s.grpcServer
	s.started = false
	s.stopped = true
	s.lifecycleMu.Unlock()

	var shutdownErr error
	if grpcServer != nil {
		shutdownErr = s.shutdownGRPC(ctx, grpcServer)
	}
	if srv == nil {
		return shutdownErr
	}
	return errors.Join(shutdownErr, srv.Shutdown(ctx))
}

func (s *Service) shutdownGRPC(ctx context.Context, server *grpc.Server) error {
	done := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Sugar().Info("gRPC stopped")
		return nil
	case <-ctx.Done():
		server.Stop()
		s.logger.Sugar().Errorf("gRPC graceful stop timed out: %v", ctx.Err())
		return ctx.Err()
	}
}
