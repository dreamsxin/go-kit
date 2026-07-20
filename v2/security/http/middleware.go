// Package httpsecurity provides optional standard-library HTTP security
// middleware. Authentication and application authorization remain outside
// this package.
package httpsecurity

import "net/http"

// Middleware wraps an HTTP handler.
type Middleware func(http.Handler) http.Handler

// Chain composes middleware in declaration order. The first middleware is the
// outermost handler.
func Chain(middlewares ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			if middlewares[i] != nil {
				next = middlewares[i](next)
			}
		}
		return next
	}
}
