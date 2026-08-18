package endpoint

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	"math/rand"
)

// TraceID is a unique identifier for a distributed trace.
type TraceID string

// SpanID is a unique identifier for a single operation within a trace.
type SpanID string

type traceKey struct{}
type spanKey struct{}
type requestIDKey struct{}
type traceContextKey struct{}

// TraceContext carries the W3C Trace Context fields for the active request,
// as defined by the W3C Trace Context specification's traceparent header:
//
//	00-<trace-id>-<span-id>-<flags>
//
// TraceID is 32 lowercase hex characters and SpanID is 16 lowercase hex
// characters. Flags is the raw two hex digit trace-flags field; bit 0x01 is
// the W3C sampled flag.
type TraceContext struct {
	TraceID string
	SpanID  string
	Flags   string
}

// ParseTraceparent parses a W3C traceparent header value. It returns false
// when the value does not conform to the specification: wrong field count or
// length, non-lowercase-hex characters, an all-zero trace or span ID, or the
// reserved version ff. Unknown future versions with a matching shape are
// accepted, as recommended by the specification.
func ParseTraceparent(header string) (TraceContext, bool) {
	const (
		traceIDLen = 32
		spanIDLen  = 16
	)
	parts := splitN(header, '-')
	if len(parts) < 4 {
		return TraceContext{}, false
	}
	version, traceID, spanID, flags := parts[0], parts[1], parts[2], parts[3]
	if version == "ff" {
		return TraceContext{}, false
	}
	if !isLowerHex(version) || len(version) != 2 {
		return TraceContext{}, false
	}
	if version == "00" && len(parts) != 4 {
		// Version 00 has exactly four fields; only future versions may
		// append fields.
		return TraceContext{}, false
	}
	if len(traceID) != traceIDLen || !isLowerHex(traceID) || isAllZero(traceID) {
		return TraceContext{}, false
	}
	if len(spanID) != spanIDLen || !isLowerHex(spanID) || isAllZero(spanID) {
		return TraceContext{}, false
	}
	if len(flags) != 2 || !isLowerHex(flags) {
		return TraceContext{}, false
	}
	return TraceContext{TraceID: traceID, SpanID: spanID, Flags: flags}, true
}

// String formats the TraceContext as a version 00 traceparent header value.
func (tc TraceContext) String() string {
	return "00-" + tc.TraceID + "-" + tc.SpanID + "-" + tc.Flags
}

// Valid reports whether the TraceContext fields hold a usable W3C trace
// context.
func (tc TraceContext) Valid() bool {
	return isLowerHex(tc.TraceID) && len(tc.TraceID) == 32 && !isAllZero(tc.TraceID) &&
		isLowerHex(tc.SpanID) && len(tc.SpanID) == 16 && !isAllZero(tc.SpanID)
}

// NewTraceContext mints a random W3C-conformant TraceContext with a cleared
// flags field.
func NewTraceContext() TraceContext {
	var b [24]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		// Fall back to a weaker ID rather than failing the request path;
		// correlation degrades but the call still proceeds.
		return TraceContext{
			TraceID: fmt.Sprintf("%032x", rand.Uint64()),
			SpanID:  fmt.Sprintf("%016x", rand.Uint64()),
			Flags:   "00",
		}
	}
	return TraceContext{
		TraceID: hex.EncodeToString(b[0:16]),
		SpanID:  hex.EncodeToString(b[16:24]),
		Flags:   "00",
	}
}

// WithTraceContext injects a W3C TraceContext into the context.
func WithTraceContext(ctx context.Context, tc TraceContext) context.Context {
	return context.WithValue(ctx, traceContextKey{}, tc)
}

// TraceContextFromContext extracts the W3C TraceContext from the context.
// The zero value is returned when none is present.
func TraceContextFromContext(ctx context.Context) TraceContext {
	tc, _ := ctx.Value(traceContextKey{}).(TraceContext)
	return tc
}

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

// newSpanID generates a random W3C-conformant span ID.
func newSpanID() string {
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		return fmt.Sprintf("%016x", rand.Uint64())
	}
	return hex.EncodeToString(b[:])
}

// TracingMiddleware returns a Middleware that propagates or generates a W3C
// trace context and a request ID in the context.
//
// If the context already carries a TraceContext (extracted by the transport
// layer from an incoming traceparent header), the trace ID and flags are
// preserved and a fresh span ID is minted for this operation, keeping the
// incoming span as the parent. Otherwise a new trace is minted in W3C format.
// A trace ID set through WithTraceID without a TraceContext is bridged into a
// propagated trace with the same trace ID.
//
// TraceIDFromContext keeps working for logging; new IDs are 32 lowercase hex
// characters as required by the W3C Trace Context specification.
//
// This enables end-to-end request correlation across service boundaries
// without requiring an external tracing system. Full span trees remain the
// domain of the observability/otel module.
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
			tc := TraceContextFromContext(ctx)
			switch {
			case tc.Valid():
				tc.SpanID = newSpanID()
			case TraceIDFromContext(ctx) != "":
				tc = TraceContext{TraceID: string(TraceIDFromContext(ctx)), SpanID: newSpanID(), Flags: "00"}
				if !tc.Valid() {
					tc = NewTraceContext()
				}
			default:
				tc = NewTraceContext()
			}
			ctx = WithTraceContext(ctx, tc)
			if TraceIDFromContext(ctx) == "" {
				ctx = WithTraceID(ctx, TraceID(tc.TraceID))
			}
			if RequestIDFromContext(ctx) == "" {
				ctx = WithRequestID(ctx, newID())
			}
			return next(ctx, request)
		}
	}
}

func splitN(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func isLowerHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return len(s) > 0
}

func isAllZero(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '0' {
			return false
		}
	}
	return true
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
