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
//	    Use(endpoint.RateLimitMiddleware(limiter)).
//	    Use(endpoint.NewCircuitBreaker().Middleware()).
//	    Build()
type Builder struct {
	base        Endpoint
	middlewares []Middleware
	labels      []string
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
	b.labels = append(b.labels, "")
	return b
}

// UseNamed is Use with a label recorded for Describe. Labeled middleware make
// the assembled chain inspectable for debugging and startup logs.
func (b *Builder) UseNamed(label string, m Middleware) *Builder {
	if m == nil {
		panic("middleware cannot be nil")
	}
	b.middlewares = append(b.middlewares, m)
	b.labels = append(b.labels, label)
	return b
}

// Describe returns the middleware labels in application order (outermost
// first). Unlabeled middleware appear as "?". The description is for
// debugging and startup logs; it does not affect runtime behavior.
//
//	ep := endpoint.NewBuilder(base).
//	    UseNamed("timeout", endpoint.TimeoutMiddleware(5*time.Second)).
//	    UseNamed("circuit_breaker", breaker.Middleware()).
//	    Build()
//	// log: "chain: timeout -> circuit_breaker -> <endpoint>"
func (b *Builder) Describe() []string {
	labels := make([]string, len(b.labels))
	for i, label := range b.labels {
		if label == "" {
			label = "?"
		}
		labels[i] = label
	}
	return labels
}

// WithMetrics appends a MetricsMiddleware and returns the Metrics pointer so
// the caller can inspect counters later.
//
//	var m endpoint.Metrics
//	ep := endpoint.NewBuilder(base).WithMetrics(&m).Build()
func (b *Builder) WithMetrics(m *Metrics) *Builder {
	return b.UseNamed("metrics", MetricsMiddleware(m))
}

// WithErrorHandling appends an ErrorHandlingMiddleware for the named operation.
func (b *Builder) WithErrorHandling(operation string) *Builder {
	return b.UseNamed("error_handling:"+operation, ErrorHandlingMiddleware(operation))
}

// WithTimeout appends a TimeoutMiddleware that cancels the context after d.
// This is a shorthand for Use(TimeoutMiddleware(d)).
func (b *Builder) WithTimeout(d time.Duration) *Builder {
	return b.UseNamed("timeout", TimeoutMiddleware(d))
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
