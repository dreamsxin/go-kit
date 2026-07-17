package endpoint

// Middleware is a function that wraps an Endpoint to add cross-cutting
// concerns such as logging, metrics, rate limiting, or circuit breaking.
//
// The recommended way to compose middlewares is via Builder:
//
//	ep := endpoint.NewBuilder(base).
//	    Use(m1).Use(m2).Use(m3).
//	    Build()
//
// Chain is available for cases where a Middleware value itself is needed.
type Middleware func(Endpoint) Endpoint

// Chain composes multiple Middlewares into a single Middleware.
// Prefer Builder.Use() for most use cases — it is more readable and
// supports named shortcuts (WithTimeout, WithMetrics, etc.).
//
// The first argument is the outermost wrapper; subsequent arguments are
// applied inward:
//
//	outer → others[0] → others[1] → … → Endpoint
func Chain(outer Middleware, others ...Middleware) Middleware {
	if outer == nil {
		panic("outer middleware cannot be nil")
	}
	for _, mw := range others {
		if mw == nil {
			panic("middleware cannot be nil")
		}
	}
	return func(next Endpoint) Endpoint {
		for i := len(others) - 1; i >= 0; i-- { // reverse
			next = others[i](next)
		}
		return outer(next)
	}
}
