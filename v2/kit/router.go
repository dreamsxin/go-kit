package kit

// Router is what a route is registered on: an *HTTP component, or a *Group cut
// from one. The typed JSON registration family takes it, so a component and a
// prefixed group register through the same call:
//
//	kit.HandleJSONTyped(svc, "POST /todos", createTodo)
//	kit.HandleJSONTyped(svc.Group("/api/v1"), "POST /todos", createTodo)
//
// Only this package implements it: the methods are unexported on purpose, so a
// Router is a component or one of its groups and nothing else.
type Router interface {
	// routePattern returns the pattern the route is registered under: the
	// pattern as given on a component, the joined pattern on a group.
	routePattern(pattern string) string
	// component returns the component the route ultimately mounts on.
	component() *HTTP
}

func (h *HTTP) routePattern(pattern string) string { return pattern }

func (h *HTTP) component() *HTTP { return h }
