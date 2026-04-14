package kit

import (
	"context"
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
	s.Start()

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
func (s *Service) Start() {
	s.srv = &http.Server{Addr: s.addr, Handler: s.mux}
	go func() {
		s.logger.Sugar().Infof("HTTP listening on %s", s.addr)
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Sugar().Fatalf("listen: %v", err)
		}
	}()

	if s.grpcAddr != "" {
		gs := s.GRPCServer()
		lis, err := net.Listen("tcp", s.grpcAddr)
		if err != nil {
			s.logger.Sugar().Fatalf("gRPC listen: %v", err)
		}
		go func() {
			s.logger.Sugar().Infof("gRPC listening on %s", s.grpcAddr)
			if err := gs.Serve(lis); err != nil {
				s.logger.Sugar().Errorf("gRPC serve: %v", err)
			}
		}()
	}
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
