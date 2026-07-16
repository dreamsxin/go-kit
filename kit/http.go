package kit

import (
	"context"
	"net/http"

	"github.com/dreamsxin/go-kit/endpoint"
)

// ServeHTTP implements http.Handler, allowing Service to be used directly
// with httptest.NewServer or http.ListenAndServe.
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// Handle registers an http.Handler for the given pattern.
// Service-level middleware is applied by wrapping the handler as an endpoint.
func (s *Service) Handle(pattern string, handler http.Handler) {
	if len(s.middleware) == 0 {
		s.mux.Handle(pattern, s.withHTTPContext(handler))
		return
	}

	base := endpoint.Endpoint(func(ctx context.Context, req any) (any, error) {
		rw := req.(http.ResponseWriter)
		r := requestFromContext(ctx).WithContext(ctx)
		handler.ServeHTTP(rw, r)
		return nil, nil
	})

	ep := s.applyEndpointMiddleware(base)

	s.mux.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := s.prepareHTTPContext(r.Context(), r, w)
		if _, err := ep(ctx, w); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
		}
	}))
}

// HandleFunc registers a plain http.HandlerFunc.
func (s *Service) HandleFunc(pattern string, fn http.HandlerFunc) {
	s.Handle(pattern, fn)
}

func (s *Service) applyEndpointMiddleware(base endpoint.Endpoint) endpoint.Endpoint {
	if len(s.middleware) == 0 {
		return base
	}
	b := endpoint.NewBuilder(base)
	for _, mw := range s.middleware {
		b = b.Use(mw)
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
