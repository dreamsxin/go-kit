package kit

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
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
	s.srv = &http.Server{Addr: s.addr, Handler: s.mux}
	go func() {
		s.logger.Sugar().Infof("HTTP listening on %s", s.addr)
		if err := s.srv.Serve(httpLis); err != nil && err != http.ErrServerClosed {
			s.logger.Sugar().Errorf("HTTP serve: %v", err)
		}
	}()

	if s.grpcAddr != "" {
		gs := s.GRPCServer()
		lis, err := net.Listen("tcp", s.grpcAddr)
		if err != nil {
			return fmt.Errorf("grpc listen: %w", err)
		}
		go func() {
			s.logger.Sugar().Infof("gRPC listening on %s", s.grpcAddr)
			if err := gs.Serve(lis); err != nil {
				s.logger.Sugar().Errorf("gRPC serve: %v", err)
			}
		}()
	}
	return nil
}

// Shutdown gracefully stops the HTTP server and gRPC server if running.
func (s *Service) Shutdown(ctx context.Context) error {
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
		s.logger.Sugar().Info("gRPC stopped")
	}
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}
