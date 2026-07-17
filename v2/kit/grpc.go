package kit

import (
	"fmt"

	"google.golang.org/grpc"
)

// GRPCServer returns the underlying *grpc.Server so callers can register
// proto services. It is created lazily on first call.
// It returns an error if WithGRPC was not set.
func (s *Service) GRPCServer() (*grpc.Server, error) {
	if s == nil {
		return nil, fmt.Errorf("kit: nil Service")
	}
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	return s.grpcServerLocked()
}

func (s *Service) grpcServerLocked() (*grpc.Server, error) {
	if s.grpcAddr == "" {
		return nil, fmt.Errorf("kit: GRPCServer called without WithGRPC")
	}
	if s.grpcServer == nil {
		s.grpcServer = grpc.NewServer(s.grpcOpts...)
	}
	return s.grpcServer, nil
}
