package kit

import (
	"net/http"
	"strings"
)

// Group is a prefixed view of an HTTP component. Every pattern registered
// through it carries the group's prefix, so a service with one versioned
// surface states the version once instead of repeating it on every route:
//
//	api := svc.Group("/api/v1")
//	api.HandleJSONTyped("POST /todos", createTodo)
//	api.HandleJSONTyped("GET /todos/{id}", getTodo)
//
// Groups nest (a group of a group joins both prefixes) and expose the same
// registration surface as the component: the full typed JSON family, the raw
// escape hatch, and the SSE stream. Registration is all a group does — it is
// not a sub-mux, takes no options, and carries no middleware of its own; a
// route joined through a group is the component's route in every other
// respect.
//
// Stable: kit.route-groups — Group prefixes every pattern registered through
// it, nests by joining prefixes, and changes nothing else: the joined route
// goes through the component's HTTP context, endpoint middleware, recorders,
// and JSON server options exactly as if the pattern had been written out in
// full.
// Covered by: TestGroupPrefixesMethodPatterns, TestGroupPrefixesBarePatterns,
// TestNestedGroupsJoinPrefixes, TestGroupKeepsEndpointMiddleware,
// TestGroupJoinPreservesPathParameters, TestGroupJoinPreservesSubtree
type Group struct {
	h      *HTTP
	prefix string
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
// prefixes.
func (g *Group) Group(prefix string) *Group {
	if g == nil {
		panic("kit: group cannot be nil")
	}
	return &Group{h: g.h, prefix: joinPrefixes(g.prefix, prefix)}
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
	g.h.Handle(g.join(pattern), handler)
}

// HandleFunc registers a raw handler function on the component, prefixed.
func (g *Group) HandleFunc(pattern string, fn http.HandlerFunc) {
	g.h.HandleFunc(g.join(pattern), fn)
}

// routePattern answers with the joined pattern; component answers unchanged.
func (g *Group) routePattern(pattern string) string { return g.join(pattern) }

// component answers with the component the group cuts from.
func (g *Group) component() *HTTP { return g.h }
