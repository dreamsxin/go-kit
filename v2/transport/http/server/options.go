package server

import "github.com/dreamsxin/go-kit/v2/transport"

type ServerOption func(*Server)

// ServerResponseEncoder sets the function used to encode successful endpoint
// responses for the JSON entry points (NewJSONServer, NewJSONEndpoint, and
// their typed and strict variants). The default is EncodeJSONResponse, which
// writes the response value as-is; use this option to assemble a transport
// envelope, such as {"code": 0, "message": "ok", "data": ...}, without
// changing business response types.
//
// It has no effect on servers built directly with NewServer, which take their
// encode function as a constructor argument.
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
// responses. The default encoder writes plain text and redacts 5xx details.
func ServerErrorEncoder(ee ErrorEncoder) ServerOption {
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
	return func(s *Server) {
		for _, hook := range f {
			if hook != nil {
				s.finalizer = append(s.finalizer, hook)
			}
		}
	}
}
