package kit

import (
	"context"
	"net/http"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// HandleSSE registers a raw HTTP handler for a Server-Sent Events stream at
// pattern. Like Service.Handle, this is an escape hatch: endpoint middleware
// does not apply. Prefer HandleSSETyped for streams that should participate
// in the service -> endpoint -> transport chain.
func HandleSSE(s *Service, pattern string, handler http.Handler) {
	s.Handle(pattern, handler)
}

// HandleSSETyped registers a typed Server-Sent Events stream at pattern. The
// stream participates in the endpoint middleware chain: middleware installed
// through WithEndpointMiddleware (request ID, tracing, metrics, timeout,
// authentication) wraps the whole stream lifecycle as one request. Decode
// failures happen before the SSE headers are written, so they map to regular
// error responses; middleware rejections are rendered with the JSON error
// encoder. Errors returned once streaming has started can only be reported
// through the server error handler; see server.NewSSEServer for the hook
// semantics.
//
// A timeout middleware bounds the total stream duration, so long-lived
// streams should avoid or relax global deadlines.
//
// Example:
//
//	kit.HandleSSETyped(svc, "GET /events",
//	    func(ctx context.Context, req eventsRequest, w *server.SSEStream) error {
//	        ticker := time.NewTicker(time.Second)
//	        defer ticker.Stop()
//	        for i := 0; ; i++ {
//	            select {
//	            case <-ctx.Done():
//	                return nil
//	            case <-ticker.C:
//	                if err := w.EventJSON("progress", map[string]int{"step": i}); err != nil {
//	                    return err
//	                }
//	            }
//	        }
//	    },
//	    decodeEventsRequest,
//	)
func HandleSSETyped[Req any](
	s *Service,
	pattern string,
	stream func(ctx context.Context, req Req, w *httpserver.SSEStream) error,
	dec func(*http.Request) (Req, error),
	opts ...httpserver.ServerOption,
) {
	if stream == nil {
		panic("kit: SSE stream function cannot be nil")
	}
	if dec == nil {
		panic("kit: SSE decode function cannot be nil")
	}
	handler := httpserver.NewSSEServerTyped(stream, dec, opts...)
	s.Handle(pattern, s.sseMiddlewareHandler(handler))
}

// sseMiddlewareHandler wraps an SSE handler so service-level endpoint
// middleware observes each stream as one request. The wrapped handler runs
// inside the HTTP context prepared by Service.Handle, which carries the
// request and response writer for the endpoint bridge.
func (s *Service) sseMiddlewareHandler(handler http.Handler) http.Handler {
	if len(s.middleware) == 0 {
		return handler
	}
	base := endpoint.Endpoint(func(ctx context.Context, _ any) (any, error) {
		request := requestFromContext(ctx)
		handler.ServeHTTP(responseWriterFromContext(ctx), request.WithContext(ctx))
		return struct{}{}, nil
	})
	wrapped := s.applyEndpointMiddleware(base)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := wrapped(r.Context(), nil); err != nil {
			httpserver.JSONErrorEncoder(r.Context(), err, w)
		}
	})
}
