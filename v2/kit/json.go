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
	handleEndpoint(r, pattern, ep, func(settings routeOptions) httpserver.DecodeRequestFunc {
		return typedRequestDecoder[Req](httpserver.StrictJSONDecodeOptions(settings.maxBodyBytes))
	}, options...)
}

// HandleEndpoint registers an endpoint with its own request decoder on an HTTP
// component, keeping every property the component installs: the endpoint
// middleware chain, the recorders, the component's JSON server options, and the
// route's own options.
//
// It exists for a route whose request is not one struct — a resource route that
// reads its filter from the query string and its body from the same request, a
// protocol adapter that decodes a media type this package does not know. The
// alternative was to assemble an http.Handler by hand, which silently skips the
// endpoint chain; that trade is invisible until the day a rate limiter or a
// recorder turns out not to cover the route.
//
// The decoder is the only thing the caller owns. It returns the value the
// endpoint receives and an error, and an error it returns is encoded exactly as
// a decoded-then-refused request is: the same JSON error body and the same
// status the transport would produce.
//
// RouteBodyLimit does not apply here: a body limit is a property of a decoder,
// and this route brought its own. A caller that wants one enforces it in the
// decoder it passes; a resource route does so through rest.WithMaxBodyBytes.
//
// Stable: kit.custom-decoder-keeps-chain — a route registered with its own decoder keeps the component's endpoint middleware, recorders and error encoder.
// Covered by: TestHandleEndpointKeepsTheEndpointChain, TestHandleEndpointEncodesDecoderErrors
func HandleEndpoint(
	r Router,
	pattern string,
	ep endpoint.Endpoint,
	decode httpserver.DecodeRequestFunc,
	options ...RouteOption,
) {
	if decode == nil {
		panic("kit: request decoder cannot be nil")
	}
	handleEndpoint(r, pattern, ep, func(routeOptions) httpserver.DecodeRequestFunc {
		return decode
	}, options...)
}

// handleEndpoint is the one registration path the two public entry points share,
// so a custom decoder cannot end up with a different middleware order, a
// different error encoder, or a different set of route options than a typed one.
func handleEndpoint(
	r Router,
	pattern string,
	ep endpoint.Endpoint,
	decoder func(routeOptions) httpserver.DecodeRequestFunc,
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
	handler := newJSONHandler(ep, decoder(settings), h, settings)
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

// newTypedJSONHandler assembles the HTTP transport of a typed JSON route. The
// decoding rule is the only thing the typed path adds; everything after it is
// newJSONHandler, so a typed route and one with a custom decoder cannot drift
// in their error encoding or their server options.
func newTypedJSONHandler[Req any](
	ep endpoint.Endpoint,
	h *HTTP,
	settings routeOptions,
) http.Handler {
	return newJSONHandler(ep, typedRequestDecoder[Req](httpserver.StrictJSONDecodeOptions(settings.maxBodyBytes)), h, settings)
}

// newJSONHandler assembles the HTTP transport of one route: the component's
// JSON server options, then the route's — the same order the transport
// packages assemble by hand, so a per-route option can still override a
// component one. The decoder is the only per-route difference, because it is
// the only part a route may need to replace.
func newJSONHandler(
	ep endpoint.Endpoint,
	decode httpserver.DecodeRequestFunc,
	h *HTTP,
	settings routeOptions,
) http.Handler {
	routeOptions := append([]httpserver.ServerOption{httpserver.ServerErrorEncoder(httpserver.JSONErrorEncoder)}, h.jsonServerOptions...)
	routeOptions = append(routeOptions, settings.server...)
	return httpserver.NewServer(ep, decode, httpserver.EncodeJSONResponse, routeOptions...)
}
