package kit

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
)

// Run starts the HTTP server (and gRPC server if enabled) and blocks until
// SIGINT or SIGTERM. It performs a graceful shutdown with a 10-second deadline.
func (s *Service) Run() {
	if err := s.Start(); err != nil {
		s.logger.Sugar().Fatalf("start: %v", err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		s.logger.Sugar().Errorf("HTTP shutdown: %v", err)
	}
	s.logger.Sugar().Info("stopped")
}

// Start starts the HTTP server (and gRPC server if enabled) in the background.
// It returns an error if either listener fails to bind.
func (s *Service) Start() error {
	// Bind the HTTP listener synchronously so bind errors (e.g. port in use)
	// surface before we return to the caller.
	httpLis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("http listen: %w", err)
	}

	var grpcLis net.Listener
	var grpcServer *grpc.Server
	if s.grpcAddr != "" {
		grpcServer = s.GRPCServer()
		grpcLis, err = net.Listen("tcp", s.grpcAddr)
		if err != nil {
			_ = httpLis.Close()
			return fmt.Errorf("grpc listen: %w", err)
		}
	}

	s.srv = &http.Server{
		Addr:              s.addr,
		Handler:           s.mux,
		ReadHeaderTimeout: s.httpConfig.ReadHeaderTimeout,
		ReadTimeout:       s.httpConfig.ReadTimeout,
		WriteTimeout:      s.httpConfig.WriteTimeout,
		IdleTimeout:       s.httpConfig.IdleTimeout,
		MaxHeaderBytes:    s.httpConfig.MaxHeaderBytes,
	}
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
	var shutdownErr error
	if s.grpcServer != nil {
		shutdownErr = s.shutdownGRPC(ctx)
	}
	if s.srv == nil {
		return shutdownErr
	}
	return errors.Join(shutdownErr, s.srv.Shutdown(ctx))
}

func (s *Service) shutdownGRPC(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.grpcServer.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Sugar().Info("gRPC stopped")
		return nil
	case <-ctx.Done():
		s.grpcServer.Stop()
		s.logger.Sugar().Errorf("gRPC graceful stop timed out: %v", ctx.Err())
		return ctx.Err()
	}
}
