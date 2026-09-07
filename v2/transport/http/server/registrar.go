package server

import "net/http"

// RouteRegistrar is what registering a route needs from a mux: the two methods
// http.ServeMux offers for it, and nothing else.
//
// Generated code and library helpers take this instead of *http.ServeMux so a
// caller can pass something that decorates each handler as it is registered.
// Registration is the only place a handler and its route pattern are both in
// scope, which is what per-route middleware needs — middleware wrapped around a
// mux sees no pattern at all, because a ServeMux sets http.Request.Pattern on the
// request it hands to the matched handler.
//
//	recording := server.RecordingMiddleware(recorder)
//	registrar := server.DecorateRoutes(mux, recording)
//	RegisterHTTPRoutes(registrar, endpoints, "/v1")
//
// *http.ServeMux satisfies it, so passing a mux directly keeps working.
type RouteRegistrar interface {
	Handle(pattern string, handler http.Handler)
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

var _ RouteRegistrar = (*http.ServeMux)(nil)

// DecorateRoutes returns a RouteRegistrar that applies middleware to every
// handler registered through it, then registers the result on the underlying
// registrar.
//
// The decoration happens at registration, so the handler the mux dispatches to is
// the wrapped one and http.Request.Pattern is the route it matched. That is the
// difference between per-route metrics and one series called "/".
//
// A nil middleware registers handlers unchanged; a nil registrar is a
// misassembly and panics, because silently dropping every route would look like a
// service with no endpoints.
func DecorateRoutes(registrar RouteRegistrar, middleware func(http.Handler) http.Handler) RouteRegistrar {
	if registrar == nil {
		panic("http server: route registrar cannot be nil")
	}
	if middleware == nil {
		return registrar
	}
	return decoratedRoutes{registrar: registrar, middleware: middleware}
}

type decoratedRoutes struct {
	registrar  RouteRegistrar
	middleware func(http.Handler) http.Handler
}

func (d decoratedRoutes) Handle(pattern string, handler http.Handler) {
	d.registrar.Handle(pattern, d.middleware(handler))
}

func (d decoratedRoutes) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	d.registrar.Handle(pattern, d.middleware(http.HandlerFunc(handler)))
}
