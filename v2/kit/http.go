package kit

import (
	"context"
	"net/http"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// ServeHTTP implements http.Handler, allowing Service to be used directly
// with httptest.NewServer or http.ListenAndServe.
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// Handle registers a raw http.Handler for the given pattern.
//
// This is an escape hatch for HTTP integrations that do not model naturally as
// framework endpoints, such as static files, third-party handlers, or custom
// protocol endpoints.
//
// Endpoint middleware is intentionally not applied to plain HTTP handlers.
// Use HandleJSON or HandleJSONEndpoint for application endpoints that should
// use the service -> endpoint -> transport chain and endpoint middleware such
// as timeout, logging, metrics, rate limiting, or circuit breaking.
func (s *Service) Handle(pattern string, handler http.Handler) {
	s.mux.Handle(pattern, s.withHTTPContext(handler))
}

// HandleFunc registers a raw http.HandlerFunc.
func (s *Service) HandleFunc(pattern string, fn http.HandlerFunc) {
	s.Handle(pattern, fn)
}

func (s *Service) applyEndpointMiddleware(route string, base endpoint.Endpoint) endpoint.Endpoint {
	if len(s.middleware) == 0 && len(s.routeMiddleware) == 0 {
		return base
	}
	b := endpoint.NewBuilder(base)
	for _, mw := range s.middleware {
		b = b.Use(mw)
	}
	for _, build := range s.routeMiddleware {
		b = b.Use(build(route))
	}
	return b.Build()
}

func (s *Service) withHTTPContext(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := s.prepareHTTPContext(r.Context(), r, w)
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Service) prepareHTTPContext(ctx context.Context, r *http.Request, w http.ResponseWriter) context.Context {
	ctx = withHTTPContext(ctx, r, w)
	if !s.requestID {
		return ctx
	}
	requestID := requestIDFromContextOrHeader(ctx)
	w.Header().Set(requestIDHeader, requestID)
	return endpoint.WithRequestID(ctx, requestID)
}
