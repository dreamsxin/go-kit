package kit

import (
	"context"
	"net/http"

	"github.com/dreamsxin/go-kit/endpoint"
	httpserver "github.com/dreamsxin/go-kit/transport/http/server"
)

// JSON creates a typed JSON http.Handler without needing a Service.
func JSON[Req any](handler func(ctx context.Context, req Req) (any, error)) http.Handler {
	return httpserver.NewJSONServer[Req](handler,
		httpserver.ServerErrorEncoder(httpserver.JSONErrorEncoder),
	)
}

// HandleJSON registers a typed JSON endpoint on a Service.
//
// It is the recommended high-level path for small services: Service
// middleware wraps the business endpoint, then the HTTP transport decodes and
// encodes JSON exactly once. JSON decoding is strict by default.
func HandleJSON[Req any](
	s *Service,
	pattern string,
	handler func(ctx context.Context, req Req) (any, error),
	options ...httpserver.ServerOption,
) {
	if handler == nil {
		panic("kit: JSON handler cannot be nil")
	}
	ep := endpoint.Endpoint(func(ctx context.Context, request any) (any, error) {
		return handler(ctx, request.(Req))
	})
	HandleJSONEndpoint[Req](s, pattern, ep, options...)
}

// HandleJSONEndpoint registers an already-built endpoint.Endpoint as a strict
// JSON route on a Service.
func HandleJSONEndpoint[Req any](
	s *Service,
	pattern string,
	ep endpoint.Endpoint,
	options ...httpserver.ServerOption,
) {
	if s == nil {
		panic("kit: Service cannot be nil")
	}
	if ep == nil {
		panic("kit: JSON endpoint cannot be nil")
	}
	ep = s.applyEndpointMiddleware(ep)
	h := httpserver.NewStrictJSONEndpoint[Req](ep, s.jsonMaxBodyBytes, options...)
	s.mux.Handle(pattern, s.withHTTPContext(h))
}
