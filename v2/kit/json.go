package kit

import (
	"context"
	"net/http"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// RouteOption customises one registration without changing the component it
// mounts on. It exists so a route that needs one different number, one extra
// middleware, or one transport option keeps every other property the component
// installs — the endpoint middleware chain, the recorders, and the JSON server
// options.
type RouteOption func(*routeOptions)

// routeOptions is what a registration needs to know before it assembles its
// handler. The zero value is not used: the component's body limit is the
// starting point, and an option replaces it.
type routeOptions struct {
	maxBodyBytes int64
	middleware   []endpoint.Middleware
	server       []httpserver.ServerOption
}

// RouteBodyLimit gives one route a different maximum request body than the rest
// of the component.
//
// It exists because the alternative was a trap. WithJSONMaxBodyBytes is
// component-wide, so a service with one upload route used to reach for Handle and a
// raw JSON server constructor — which silently skips the endpoint middleware, the
// recorders, and the component's JSON server options. Losing observability as a
// side effect of changing a size is not a trade anyone would make on purpose, so
// this is the same registration path with one number replaced.
//
// A limit of zero or less is a mistake rather than "no limit": an unbounded body is
// the component default's job to refuse, not a value to smuggle in per route.
func RouteBodyLimit(maxBodyBytes int64) RouteOption {
	return func(o *routeOptions) {
		if maxBodyBytes <= 0 {
			panic("kit: per-route JSON body limit must be positive")
		}
		o.maxBodyBytes = maxBodyBytes
	}
}

// RouteMiddleware adds endpoint middleware closest to the handler. Middleware
// installed through WithEndpointMiddleware wraps outside it and a group's
// middleware wraps between the two, so route middleware is where a per-route
// concern such as a route-specific rate limiter or audit hook belongs.
func RouteMiddleware(middleware ...endpoint.Middleware) RouteOption {
	return func(o *routeOptions) {
		for _, mw := range middleware {
			if mw == nil {
				panic("kit: route middleware cannot be nil")
			}
			o.middleware = append(o.middleware, mw)
		}
	}
}

// RouteServerOptions passes transport options to the one route's JSON server.
// They apply after the component's JSON server options, so a route can override
// the component's error encoder or response encoder for itself.
func RouteServerOptions(options ...httpserver.ServerOption) RouteOption {
	return func(o *routeOptions) {
		for _, option := range options {
			if option != nil {
				o.server = append(o.server, option)
			}
		}
	}
}

// routeOptionsFor applies the registration's options over the component
// defaults.
func (h *HTTP) routeOptionsFor(options []RouteOption) routeOptions {
	settings := routeOptions{maxBodyBytes: h.jsonMaxBodyBytes}
	for _, option := range options {
		if option != nil {
			option(&settings)
		}
	}
	return settings
}

// NewJSONHandler creates a typed JSON http.Handler without needing a component.
//
// It registers nothing. The returned handler is yours to mount, on a plain
// *http.ServeMux or anywhere else, which also means it carries none of the
// endpoint middleware, recorders or JSON server options an HTTP component
// installs. When you have a component, HandleJSON is the function you want.
func NewJSONHandler[Req, Resp any](handler func(ctx context.Context, req Req) (Resp, error)) http.Handler {
	if handler == nil {
		panic("kit: JSON handler cannot be nil")
	}
	return httpserver.NewTypedJSON[Req, Resp](handler,
		httpserver.DefaultJSONDecodeOptions(),
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
//
// Req and Resp are inferred from the handler, so a concrete response type needs
// no explicit type arguments:
//
//	kit.HandleJSON(http, "POST /todos", createTodo)
//
// A dynamic response type is the same call whose handler returns any. Route-level
// differences are options, so no second spelling of the function exists:
//
//	kit.HandleJSON(http, "POST /upload", upload, kit.RouteBodyLimit(8<<20))
func HandleJSON[Req, Resp any](
	r Router,
	pattern string,
	handler func(ctx context.Context, req Req) (Resp, error),
	options ...RouteOption,
) {
	if handler == nil {
		panic("kit: JSON handler cannot be nil")
	}
	HandleJSONEndpoint[Req](r, pattern, endpoint.TypedEndpoint[Req, Resp](handler).Wrap(), options...)
}

// HandleJSONEndpoint registers an already-built endpoint.Endpoint as a strict
// JSON route on an HTTP component, preserving the normal endpoint middleware
// and HTTP transport chain.
//
// Stable: kit.per-route-body-limit — a route can accept a different body size through the same registration path, so it keeps the endpoint middleware, recorders, and JSON server options the component installs.
// Covered by: TestPerRouteBodyLimitKeepsEndpointMiddleware, TestPerRouteBodyLimitAppliesToThatRouteOnly
func HandleJSONEndpoint[Req any](
	r Router,
	pattern string,
	ep endpoint.Endpoint,
	options ...RouteOption,
) {
	h := r.component()
	pattern = r.routePattern(pattern)
	if h == nil {
		panic("kit: HTTP component cannot be nil")
	}
	if ep == nil {
		panic("kit: JSON endpoint cannot be nil")
	}
	settings := h.routeOptionsFor(options)
	for _, mw := range settings.middleware {
		ep = mw(ep)
	}
	for _, mw := range r.routerEndpointMiddleware() {
		ep = mw(ep)
	}
	ep = h.applyEndpointMiddleware(pattern, ep)
	handler := newTypedJSONHandler[Req](ep, h, settings)
	handler = wrapHTTPMiddleware(r.routerHTTPMiddleware(), handler)
	h.mux.Handle(pattern, h.withHTTPContext(handler))
}

// wrapHTTPMiddleware applies middleware to a handler, first outermost.
func wrapHTTPMiddleware(middleware []func(http.Handler) http.Handler, handler http.Handler) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		handler = middleware[i](handler)
	}
	return handler
}

// newTypedJSONHandler assembles the HTTP transport of a typed JSON route:
// the component's decode strictness and body limit, the request filled from
// the request itself (see typedRequestDecoder), the shared JSON response
// encoder, the default error encoder, then the component's JSON server
// options, then the route's — the same order the transport packages
// assemble by hand, so a per-route option can still override a component one.
func newTypedJSONHandler[Req any](
	ep endpoint.Endpoint,
	h *HTTP,
	settings routeOptions,
) http.Handler {
	routeOptions := append([]httpserver.ServerOption{httpserver.ServerErrorEncoder(httpserver.JSONErrorEncoder)}, h.jsonServerOptions...)
	routeOptions = append(routeOptions, settings.server...)
	decoder := typedRequestDecoder[Req](httpserver.StrictJSONDecodeOptions(settings.maxBodyBytes))
	return httpserver.NewServer(ep, decoder, httpserver.EncodeJSONResponse, routeOptions...)
}
