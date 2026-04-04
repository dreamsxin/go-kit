package endpoint

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/dreamsxin/go-kit/log"
)

// Logger is an alias kept for convenience within this package.
type Logger = log.Logger

// TraceID is a unique identifier for a distributed trace.
type TraceID string

// SpanID is a unique identifier for a single operation within a trace.
type SpanID string

type traceKey struct{}
type spanKey struct{}
type requestIDKey struct{}

// WithTraceID injects a trace ID into the context.
func WithTraceID(ctx context.Context, id TraceID) context.Context {
	return context.WithValue(ctx, traceKey{}, id)
}

// TraceIDFromContext extracts the trace ID from the context.
// Returns an empty string if not set.
func TraceIDFromContext(ctx context.Context) TraceID {
	id, _ := ctx.Value(traceKey{}).(TraceID)
	return id
}

// WithRequestID injects a request ID into the context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFromContext extracts the request ID from the context.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// newID generates a short random hex ID.
func newID() string {
	return fmt.Sprintf("%016x", rand.Int63()) //nolint:gosec
}

// TracingMiddleware returns a Middleware that propagates or generates a
// trace ID and request ID in the context.
//
// If the context already contains a trace ID (injected by the transport
// layer from an incoming header), it is preserved.  Otherwise a new one
// is generated.
//
// This enables end-to-end request correlation across service boundaries
// without requiring an external tracing system.
//
// Example:
//
//	ep = endpoint.TracingMiddleware()(ep)
//
//	// In a handler, read the IDs:
//	traceID := endpoint.TraceIDFromContext(ctx)
//	reqID   := endpoint.RequestIDFromContext(ctx)
func TracingMiddleware() Middleware {
	return func(next Endpoint) Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			if TraceIDFromContext(ctx) == "" {
				ctx = WithTraceID(ctx, TraceID(newID()))
			}
			if RequestIDFromContext(ctx) == "" {
				ctx = WithRequestID(ctx, newID())
			}
			return next(ctx, request)
		}
	}
}

// ── Builder shortcuts ─────────────────────────────────────────────────────────

// WithTracing appends TracingMiddleware to the Builder.
func (b *Builder) WithTracing() *Builder {
	return b.Use(TracingMiddleware())
}

// WithBackpressure appends BackpressureMiddleware with the given concurrency limit.
func (b *Builder) WithBackpressure(max int64) *Builder {
	return b.Use(BackpressureMiddleware(max))
}

// WithLogging appends LoggingMiddleware for the named operation.
// This is a shorthand for Use(LoggingMiddleware(logger, operation)).
func (b *Builder) WithLogging(logger *Logger, operation string) *Builder {
	return b.Use(LoggingMiddleware(logger, operation))
}
