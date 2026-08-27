package kit

import (
	"context"
	"net/http"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// JSON creates a typed JSON http.Handler without needing a Service.
func JSON[Req any](handler func(ctx context.Context, req Req) (any, error)) http.Handler {
	return httpserver.NewJSONServer[Req](handler,
		httpserver.ServerErrorEncoder(httpserver.JSONErrorEncoder),
	)
}

// JSONTyped creates a JSON http.Handler with compile-time request and response
// types without needing a Service.
func JSONTyped[Req, Resp any](handler func(ctx context.Context, req Req) (Resp, error)) http.Handler {
	return httpserver.NewTypedJSONServer[Req, Resp](handler,
		httpserver.ServerErrorEncoder(httpserver.JSONErrorEncoder),
	)
}

// HandleJSON registers a typed JSON endpoint on an HTTP component.
//
// It is the recommended high-level path for small services: component
// middleware wraps the business endpoint, then the HTTP transport decodes
// and encodes JSON exactly once. This keeps the same service -> endpoint ->
// transport boundary as generated projects while avoiding boilerplate. JSON
// decoding is strict by default.
func HandleJSON[Req any](
	h *HTTP,
	pattern string,
	handler func(ctx context.Context, req Req) (any, error),
	options ...httpserver.ServerOption,
) {
	if handler == nil {
		panic("kit: JSON handler cannot be nil")
	}
	ep := endpoint.TypedEndpoint[Req, any](handler).Wrap()
	HandleJSONEndpoint[Req](h, pattern, ep, options...)
}

// HandleJSONTyped registers a JSON endpoint with compile-time request and
// response types. Prefer it for new handlers that return a concrete response.
func HandleJSONTyped[Req, Resp any](
	h *HTTP,
	pattern string,
	handler func(ctx context.Context, req Req) (Resp, error),
	options ...httpserver.ServerOption,
) {
	if handler == nil {
		panic("kit: JSON handler cannot be nil")
	}
	HandleJSONEndpoint[Req](h, pattern, endpoint.TypedEndpoint[Req, Resp](handler).Wrap(), options...)
}

// HandleJSONEndpoint registers an already-built endpoint.Endpoint as a strict
// JSON route on an HTTP component, preserving the normal endpoint middleware
// and HTTP transport chain.
func HandleJSONEndpoint[Req any](
	h *HTTP,
	pattern string,
	ep endpoint.Endpoint,
	options ...httpserver.ServerOption,
) {
	if h == nil {
		panic("kit: HTTP component cannot be nil")
	}
	if ep == nil {
		panic("kit: JSON endpoint cannot be nil")
	}
	ep = h.applyEndpointMiddleware(ep)
	routeOptions := append(append([]httpserver.ServerOption(nil), h.jsonServerOptions...), options...)
	handler := httpserver.NewStrictJSONEndpoint[Req](ep, h.jsonMaxBodyBytes, routeOptions...)
	h.mux.Handle(pattern, h.withHTTPContext(handler))
}
