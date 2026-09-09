package kit

import (
	"context"
	"net/http"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// HandleSSE registers a raw HTTP handler at pattern. Despite the name it does
// nothing SSE-specific: the body is a call to Handle, so the endpoint middleware
// chain and the endpoint recorders do not apply, exactly as they do not for any
// raw handler.
//
// Deprecated: use Handle. A name that promises SSE behaviour while skipping the
// middleware chain is how a stream loses its middleware by accident — a reader
// following the name from HandleSSETyped has no reason to expect the difference.
// Handle is the same call and says what it gives up. HandleSSETyped is what to
// reach for when the stream should participate in the chain.
func (h *HTTP) HandleSSE(pattern string, handler http.Handler) {
	h.Handle(pattern, handler)
}

// HandleSSETyped registers a typed Server-Sent Events stream at pattern. The
// stream participates in the endpoint middleware chain: middleware installed
// through WithEndpointMiddleware (request ID, tracing, metrics, timeout,
// authentication) wraps the whole stream lifecycle as one request. Decode
// failures happen before the SSE headers are written, so they map to regular
// error responses; a middleware rejection is rendered with the same error
// encoder — the component's, or this route's if it passes one — so one route
// answers with one error contract. An error returned once streaming has started
// cannot change the response, but it does reach the endpoint middleware and the
// server error handler, so the stream records as the failure it was; see
// server.NewSSEServer for the hook semantics.
//
// A timeout middleware bounds the total stream duration, so long-lived
// streams should avoid or relax global deadlines.
//
// Example:
//
//	kit.HandleSSETyped(httpComponent, "GET /events",
//	    func(ctx context.Context, req eventsRequest, w *server.SSEStream) error {
//	        ticker := time.NewTicker(time.Second)
//	        defer ticker.Stop()
//	        for i := 0; ; i++ {
//	            select {
//	            case <-kit.Stopping(ctx):
//	                // The process is going away: end the stream so the client
//	                // sees it end, rather than a connection cut mid-deploy.
//	                return nil
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
	h *HTTP,
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
	// The component's JSON server options come first so a route's own options
	// still win. They were previously not passed at all, which left a deployment
	// that installed ProblemJSONErrorEncoder component-wide answering
	// application/problem+json on every JSON route and a plain envelope on an SSE
	// decode failure — one service with two error contracts.
	streamOptions := append(append([]httpserver.ServerOption(nil), h.jsonServerOptions...), opts...)
	handler := httpserver.NewSSEServerTyped(stream, dec, streamOptions...)
	h.Handle(pattern, h.sseMiddlewareHandler(pattern, handler))
}

// streamOutcome records whether the stream handler was reached, which is what
// separates "the chain refused this request" from "the stream itself failed".
//
// The two have to be told apart because only one of them may still write a
// response. A rejection produced by middleware never reached the SSEServer, so
// nothing has been sent and the error is the response. An error coming back out
// of the stream arrives after 200 and the events that were already flushed, and
// answering it again would append an error body to a valid stream.
type streamOutcome struct {
	served bool
}

type streamOutcomeKey struct{}

// sseMiddlewareHandler wraps an SSE handler so component-level endpoint
// middleware observes each stream as one request. The wrapped handler runs
// inside the HTTP context prepared by Handle, which carries the request and
// response writer for the endpoint bridge.
//
// The bridge returns the stream's own error rather than swallowing it: the
// middleware chain around it is where the request is counted, timed, and logged,
// and a bridge that always returned nil made a stream that died on its third
// event indistinguishable from one that ran to completion.
//
// Stable: kit.sse-failure-is-not-a-success — an error ending an SSE stream reaches the component's endpoint middleware instead of being reported as a completed request.
// Covered by: TestHandleSSETyped_StreamFailureReachesEndpointMiddleware
//
// Stable: kit.sse-one-error-contract — a middleware rejection of an SSE route is encoded with the same error encoder the route's decode failures use.
// Covered by: TestHandleSSETyped_RejectionUsesTheComponentErrorEncoder
func (h *HTTP) sseMiddlewareHandler(pattern string, handler *httpserver.SSEServer) http.Handler {
	if len(h.middleware) == 0 && len(h.recorders) == 0 {
		return handler
	}
	encodeError := handler.ErrorEncoder()
	base := endpoint.Endpoint(func(ctx context.Context, _ any) (any, error) {
		if outcome, ok := ctx.Value(streamOutcomeKey{}).(*streamOutcome); ok {
			outcome.served = true
		}
		request := requestFromContext(ctx)
		return struct{}{}, handler.ServeStream(responseWriterFromContext(ctx), request.WithContext(ctx))
	})
	wrapped := h.applyEndpointMiddleware(pattern, base)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		outcome := &streamOutcome{}
		ctx := context.WithValue(r.Context(), streamOutcomeKey{}, outcome)
		if _, err := wrapped(ctx, nil); err != nil && !outcome.served {
			encodeError(ctx, err, w)
		}
	})
}
