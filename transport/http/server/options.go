package server

type ServerOption func(*Server)

func ServerBefore(before ...RequestFunc) ServerOption {
	return func(s *Server) { s.before = append(s.before, before...) }
}

func ServerAfter(after ...ResponseFunc) ServerOption {
	return func(s *Server) { s.after = append(s.after, after...) }
}

func ServerErrorEncoder(ee ErrorEncoder) ServerOption {
	return func(s *Server) { s.errorEncoder = ee }
}

func ServerErrorHandler(errorHandler ErrorHandler) ServerOption {
	return func(s *Server) { s.errorHandler = errorHandler }
}

func ServerFinalizer(f ...ResponseFunc) ServerOption {
	return func(s *Server) { s.finalizer = append(s.finalizer, f...) }
}
