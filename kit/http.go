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
		s.mux.Handle(pattern, handler)
		return
	}

	base := endpoint.Endpoint(func(ctx context.Context, req any) (any, error) {
		rw := req.(http.ResponseWriter)
		r := requestFromContext(ctx).WithContext(ctx)
		handler.ServeHTTP(rw, r)
		return nil, nil
	})

	b := endpoint.NewBuilder(base)
	for _, mw := range s.middleware {
		b = b.Use(mw)
	}
	ep := b.Build()

	s.mux.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), httpRequestKey{}, r)
		if _, err := ep(ctx, w); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
		}
	}))
}

// HandleFunc registers a plain http.HandlerFunc.
func (s *Service) HandleFunc(pattern string, fn http.HandlerFunc) {
	s.Handle(pattern, fn)
}
