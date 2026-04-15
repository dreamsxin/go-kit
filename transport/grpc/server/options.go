package server

import (
	"github.com/dreamsxin/go-kit/log"
	"github.com/dreamsxin/go-kit/transport"
)

type ServerOption func(*Server)

// ServerBefore adds RequestFunc hooks that run before the request is decoded.
func ServerBefore(before ...RequestFunc) ServerOption {
	return func(s *Server) {
		for _, hook := range before {
			if hook != nil {
				s.before = append(s.before, hook)
			}
		}
	}
}

// ServerAfter adds ResponseFunc hooks that run after the Endpoint returns.
func ServerAfter(after ...ResponseFunc) ServerOption {
	return func(s *Server) {
		for _, hook := range after {
			if hook != nil {
				s.after = append(s.after, hook)
			}
		}
	}
}

// ServerErrorLogger sets a logger-based error handler (convenience wrapper).
func ServerErrorLogger(logger *log.Logger) ServerOption {
	return func(s *Server) { s.errorHandler = transport.NewLogErrorHandler(logger) }
}

// ServerErrorHandler sets the handler called when any step returns an error.
func ServerErrorHandler(errorHandler transport.ErrorHandler) ServerOption {
	return func(s *Server) { s.errorHandler = errorHandler }
}

// ServerFinalizer adds FinalizerFunc hooks that always run at the end of a call.
func ServerFinalizer(f ...FinalizerFunc) ServerOption {
	return func(s *Server) {
		for _, hook := range f {
			if hook != nil {
				s.finalizer = append(s.finalizer, hook)
			}
		}
	}
}
