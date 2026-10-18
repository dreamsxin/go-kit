package kit

import (
	"net/http"
	"strings"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// Group is a prefixed view of an HTTP component. Every pattern registered
// through it carries the group's prefix, so a service with one versioned
// surface states the version once instead of repeating it on every route:
//
//	api := svc.Group("/api/v1")
//	api.HandleJSON("POST /todos", createTodo)
//	api.HandleJSON("GET /todos/{id}", getTodo)
//
// Groups nest (a group of a group joins both prefixes) and expose the same
// registration surface as the component: JSON registration, the raw escape
// hatch, and the SSE stream. Registration is all a group does — it is
// not a sub-mux, takes no options, and carries no middleware of its own; a
// route joined through a group is the component's route in every other
// respect.
//
// Stable: kit.route-groups — Group prefixes every pattern registered through
// it, nests by joining prefixes, carries its own middleware layers
// (UseEndpointMiddleware inside the component's chain, UseHTTPMiddleware
// around the route's handler, both first-outermost and inherited by nested
// groups), and changes nothing else: the joined route goes through the
// component's HTTP context, recorders, and JSON server options exactly as if
// the pattern had been written out in full.
// Covered by: TestGroupPrefixesMethodPatterns, TestGroupPrefixesBarePatterns,
// TestNestedGroupsJoinPrefixes, TestGroupKeepsEndpointMiddleware,
// TestGroupJoinPreservesPathParameters, TestGroupJoinPreservesSubtree,
// TestGroupEndpointMiddlewareAppliesToGroupRoutes,
// TestGroupEndpointMiddlewareRunsInsideComponentChain,
// TestGroupHTTPMiddlewareWrapsGroupHandlers,
// TestNestedGroupInheritsMiddleware
type Group struct {
	h             *HTTP
	prefix        string
	endpointChain []endpoint.Middleware
	httpChain     []func(http.Handler) http.Handler
}

// Group returns a registration view that prefixes every pattern registered
// through it. An empty prefix returns the component's own view.
func (h *HTTP) Group(prefix string) *Group {
	if h == nil {
		panic("kit: HTTP component cannot be nil")
	}
	return &Group{h: h, prefix: joinPrefixes("", prefix)}
}

// Group returns a nested view: the patterns registered through it carry both
// prefixes, and the parent's middleware governs the child's routes.
func (g *Group) Group(prefix string) *Group {
	if g == nil {
		panic("kit: group cannot be nil")
	}
	return &Group{
		h:             g.h,
		prefix:        joinPrefixes(g.prefix, prefix),
		endpointChain: g.endpointChain,
		httpChain:     g.httpChain,
	}
}

// UseEndpointMiddleware adds endpoint middleware for the routes registered
// through this group from here on. The chain is fixed at registration: a
// group route's endpoint runs inside the component's chain (and its
// recorders) and outside the route's own middleware, with the group's
// middleware in the order given — the first is the outermost of the group's.
// Call Use before the registrations it should govern.
func (g *Group) UseEndpointMiddleware(mw ...endpoint.Middleware) {
	g.endpointChain = append(g.endpointChain, mw...)
}

// UseHTTPMiddleware adds HTTP middleware around the handlers of the routes
// registered through this group from here on, inside the component's HTTP
// middleware, in the order given — the first is the outermost of the group's.
// It does not touch the endpoint chain; use UseEndpointMiddleware for that.
func (g *Group) UseHTTPMiddleware(mw ...func(http.Handler) http.Handler) {
	g.httpChain = append(g.httpChain, mw...)
}

// joinPrefixes joins two prefix segments with exactly one slash between them.
// A prefix is not a pattern: it names a subtree, so its trailing slashes are
// noise and are trimmed, and its leading slash is normalized in.
func joinPrefixes(base, add string) string {
	add = strings.Trim(strings.TrimSpace(add), "/")
	if add == "" {
		return base
	}
	if base == "" {
		return "/" + add
	}
	return base + "/" + add
}

// join returns pattern with the group's prefix in front of its path. A
// "METHOD /path" pattern keeps its method; a bare "/path" pattern stays bare
// (the mux answers every method for it, and a joined route should not decide
// differently). Trailing slashes and path parameters pass through untouched.
func (g *Group) join(pattern string) string {
	method, path, found := strings.Cut(pattern, " ")
	if !found {
		method, path = "", strings.TrimSpace(pattern)
	}
	joined := g.prefix + "/" + strings.TrimLeft(path, "/")
	if method != "" {
		return method + " " + joined
	}
	return joined
}

// Handle registers a raw handler on the component, prefixed. The raw-route
// trades of Handle apply unchanged.
func (g *Group) Handle(pattern string, handler http.Handler) {
	g.h.Handle(g.join(pattern), g.wrapHTTP(handler))
}

// HandleFunc registers a raw handler function on the component, prefixed.
func (g *Group) HandleFunc(pattern string, fn http.HandlerFunc) {
	g.h.Handle(g.join(pattern), g.wrapHTTP(fn))
}

// routePattern answers with the joined pattern; component answers unchanged.
func (g *Group) routePattern(pattern string) string { return g.join(pattern) }

// component answers with the component the group cuts from.
func (g *Group) component() *HTTP { return g.h }

// routerEndpointMiddleware answers with the group's endpoint chain.
func (g *Group) routerEndpointMiddleware() []endpoint.Middleware { return g.endpointChain }

// routerHTTPMiddleware answers with the group's HTTP chain.
func (g *Group) routerHTTPMiddleware() []func(http.Handler) http.Handler { return g.httpChain }

// wrapHTTP wraps a handler with the group's HTTP middleware, first outermost.
func (g *Group) wrapHTTP(handler http.Handler) http.Handler {
	for i := len(g.httpChain) - 1; i >= 0; i-- {
		handler = g.httpChain[i](handler)
	}
	return handler
}
