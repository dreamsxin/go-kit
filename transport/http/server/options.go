package server

import "github.com/dreamsxin/go-kit/transport"

type ServerOption func(*Server)

func ServerBefore(before ...RequestFunc) ServerOption {
	return func(s *Server) { s.before = append(s.before, before...) }
}

func ServerAfter(after ...ResponseFunc) ServerOption {
	return func(s *Server) { s.after = append(s.after, after...) }
}

func ServerErrorEncoder(ee transport.ErrorEncoder) ServerOption {
	return func(s *Server) { s.errorEncoder = ee }
}

func ServerErrorHandler(errorHandler transport.ErrorHandler) ServerOption {
	return func(s *Server) { s.errorHandler = errorHandler }
}

func ServerFinalizer(f ...FinalizerFunc) ServerOption {
	return func(s *Server) { s.finalizer = append(s.finalizer, f...) }
}
