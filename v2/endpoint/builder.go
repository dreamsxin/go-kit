package endpoint

import (
	"context"
	"time"
)

// Builder provides a fluent API for assembling an Endpoint with a middleware
// chain.  Middlewares are applied in the order they are added (outermost
// first), matching the behaviour of Chain().
//
// Example:
//
//	ep := endpoint.NewBuilder(myEndpoint).
//	    Use(loggingMiddleware).
//	    Use(ratelimit.NewErroringLimiter(limiter)).
//	    Use(circuitbreaker.Gobreaker(cb)).
//	    Build()
type Builder struct {
	base        Endpoint
	middlewares []Middleware
}

// NewBuilder creates a Builder wrapping the given base Endpoint.
func NewBuilder(base Endpoint) *Builder {
	if base == nil {
		panic("base endpoint cannot be nil")
	}
	return &Builder{base: base}
}

// Use appends a Middleware to the chain.  Returns the same Builder for
// method chaining.
func (b *Builder) Use(m Middleware) *Builder {
	if m == nil {
		panic("middleware cannot be nil")
	}
	b.middlewares = append(b.middlewares, m)
	return b
}

// WithMetrics appends a MetricsMiddleware and returns the Metrics pointer so
// the caller can inspect counters later.
//
//	var m endpoint.Metrics
//	ep := endpoint.NewBuilder(base).WithMetrics(&m).Build()
func (b *Builder) WithMetrics(m *Metrics) *Builder {
	return b.Use(MetricsMiddleware(m))
}

// WithErrorHandling appends an ErrorHandlingMiddleware for the named operation.
func (b *Builder) WithErrorHandling(operation string) *Builder {
	return b.Use(ErrorHandlingMiddleware(operation))
}

// WithTimeout appends a TimeoutMiddleware that cancels the context after d.
// This is a shorthand for Use(TimeoutMiddleware(d)).
func (b *Builder) WithTimeout(d time.Duration) *Builder {
	return b.Use(TimeoutMiddleware(d))
}

// Build applies all middlewares and returns the final Endpoint.
// The Builder can be reused after calling Build.
func (b *Builder) Build() Endpoint {
	if len(b.middlewares) == 0 {
		return b.base
	}
	return Chain(b.middlewares[0], b.middlewares[1:]...)(b.base)
}

// ─────────────────────────── Timeout middleware ───────────────────────────

// TimeoutMiddleware returns a Middleware that cancels the context after d.
// The wrapped endpoint receives a context that will be cancelled when the
// deadline is exceeded.
//
// Example:
//
//	ep = endpoint.TimeoutMiddleware(5 * time.Second)(ep)
func TimeoutMiddleware(d time.Duration) Middleware {
	return func(next Endpoint) Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			ctx, cancel := context.WithTimeout(ctx, d)
			defer cancel()
			return next(ctx, request)
		}
	}
}
