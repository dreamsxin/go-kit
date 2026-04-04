package server

import "github.com/dreamsxin/go-kit/transport"

type ServerOption func(*Server)

// ServerBefore adds RequestFunc hooks that run before the request is decoded.
// Each hook receives the current context and the raw *http.Request, and
// returns a (possibly enriched) context.  Hooks run in the order added.
func ServerBefore(before ...RequestFunc) ServerOption {
	return func(s *Server) { s.before = append(s.before, before...) }
}

// ServerAfter adds ResponseFunc hooks that run after the Endpoint returns
// successfully, but before the response is encoded.  Hooks run in order.
func ServerAfter(after ...ResponseFunc) ServerOption {
	return func(s *Server) { s.after = append(s.after, after...) }
}

// ServerErrorEncoder sets the function used to encode errors into HTTP
// responses.  The default encoder writes a plain-text body with status 500.
func ServerErrorEncoder(ee transport.ErrorEncoder) ServerOption {
	return func(s *Server) { s.errorEncoder = ee }
}

// ServerErrorHandler sets the handler that is called whenever an error
// occurs (decode, endpoint, or encode).  The default handler logs via zap.
func ServerErrorHandler(errorHandler transport.ErrorHandler) ServerOption {
	return func(s *Server) { s.errorHandler = errorHandler }
}

// ServerFinalizer adds FinalizerFunc hooks that always run at the end of a
// request, regardless of success or failure.  Useful for logging latency or
// recording metrics.
func ServerFinalizer(f ...FinalizerFunc) ServerOption {
	return func(s *Server) { s.finalizer = append(s.finalizer, f...) }
}
