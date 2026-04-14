package kit

import "google.golang.org/grpc"

// GRPCServer returns the underlying *grpc.Server so callers can register
// proto services. It is created lazily on first call.
// Panics if WithGRPC was not set.
func (s *Service) GRPCServer() *grpc.Server {
	if s.grpcAddr == "" {
		panic("kit: GRPCServer() called but WithGRPC option was not set")
	}
	if s.grpcServer == nil {
		s.grpcServer = grpc.NewServer(s.grpcOpts...)
	}
	return s.grpcServer
}
