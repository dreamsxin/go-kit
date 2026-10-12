package server

import "github.com/dreamsxin/go-kit/v2/transport"

// ServerOption configures an HTTP transport server.
type ServerOption func(*Server)

// ServerResponseEncoder sets the function used to encode successful endpoint
// responses for the JSON entry points (NewJSONServer, NewJSONEndpoint, and
// their typed and strict variants). The default is EncodeJSONResponse, which
// writes the response value as-is; use this option to assemble a transport
// envelope, such as {"code": 0, "message": "ok", "data": ...}, without
// changing business response types.
//
// The option also overrides the encode argument of a server built directly
// with NewServer: options apply after the constructor argument, which is what
// lets a generated transport's per-route defaults be replaced from a seam
// that appends options last.
func ServerResponseEncoder(ee EncodeResponseFunc) ServerOption {
	return func(s *Server) {
		if ee != nil {
			s.enc = ee
		}
	}
}

// ServerBefore adds RequestFunc hooks that run before the request is decoded.
// Each hook receives the current context and the raw *http.Request, and
// returns a (possibly enriched) context.  Hooks run in the order added.
func ServerBefore(before ...RequestFunc) ServerOption {
	return func(s *Server) {
		for _, hook := range before {
			if hook != nil {
				s.before = append(s.before, hook)
			}
		}
	}
}

// ServerAfter adds ResponseFunc hooks that run after the Endpoint returns
// successfully, but before the response is encoded.  Hooks run in order.
func ServerAfter(after ...ResponseFunc) ServerOption {
	return func(s *Server) {
		for _, hook := range after {
			if hook != nil {
				s.after = append(s.after, hook)
			}
		}
	}
}

// ServerErrorEncoder sets the function used to encode errors into HTTP
// responses. The default encoder writes plain text and never puts err.Error() on
// the wire at 5xx: 500 always reads "Internal Server Error", and another 5xx
// carries only an explicit PublicMessage.
func ServerErrorEncoder(ee ErrorEncoder) ServerOption {
	return func(s *Server) { s.errorEncoder = ee }
}

// ServerErrorHandler sets the handler that is called whenever an error
// occurs (decode, endpoint, or encode). The default logs through slog's
// default logger at Warn level (transport.DefaultErrorHandler); pass
// transport.NopErrorHandler to restore silence, or
// observability/slog.NewErrorHandler for the rich adapter with request
// correlation.
func ServerErrorHandler(errorHandler transport.ErrorHandler) ServerOption {
	return func(s *Server) { s.errorHandler = errorHandler }
}

// ServerFinalizer adds FinalizerFunc hooks that always run at the end of a
// request, regardless of success or failure.  Useful for logging latency or
// recording metrics.
func ServerFinalizer(f ...FinalizerFunc) ServerOption {
	return func(s *Server) {
		for _, hook := range f {
			if hook != nil {
				s.finalizer = append(s.finalizer, hook)
			}
		}
	}
}
