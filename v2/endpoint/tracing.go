package endpoint

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"math/rand"
)

// TraceID is a unique identifier for a distributed trace.
type TraceID string

// SpanID is a unique identifier for a single operation within a trace.
type SpanID string

type correlationKey struct{}

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
		// correlation degrades but the call still proceeds, and the result is
		// still the shape the specification requires.
		fillWeakRandom(b[:])
	}
	return TraceContext{
		TraceID: hex.EncodeToString(b[0:16]),
		SpanID:  hex.EncodeToString(b[16:24]),
		Flags:   "00",
	}
}

// WithTraceContext injects a W3C TraceContext as the identity this node is
// operating with — for example the OpenTelemetry span the tracing middleware
// started — so SpanIDFromContext reports its span ID.
func WithTraceContext(ctx context.Context, tc TraceContext) context.Context {
	values := correlationFromContext(ctx)
	values.trace = tc
	values.spanOwned = tc.Valid()
	return withCorrelation(ctx, values)
}

// WithIncomingTraceContext injects a trace context extracted from an inbound
// request. Its span ID is the caller's — a remote parent — not a span this
// node started, so TraceIDFromContext joins the caller's trace while
// SpanIDFromContext stays empty until this node continues the trace through
// TracingMiddleware or ContinueIncomingTrace.
func WithIncomingTraceContext(ctx context.Context, tc TraceContext) context.Context {
	values := correlationFromContext(ctx)
	values.trace = tc
	values.spanOwned = false
	return withCorrelation(ctx, values)
}

// TraceContextFromContext extracts the W3C TraceContext from the context.
// The zero value is returned when none is present.
func TraceContextFromContext(ctx context.Context) TraceContext {
	return correlationFromContext(ctx).trace
}

// WithTraceID injects a trace ID into the context.
func WithTraceID(ctx context.Context, id TraceID) context.Context {
	values := correlationFromContext(ctx)
	values.traceID = id
	return withCorrelation(ctx, values)
}

// TraceIDFromContext extracts the trace ID from the context.
//
// The W3C TraceContext is the single source of truth: an ID extracted from an
// inbound traceparent is visible here even when no TracingMiddleware ran,
// which is what lets a raw handler's logs join the caller's trace. A trace ID
// installed explicitly through WithTraceID still wins, so the pre-W3C string
// correlation API keeps its published semantics.
// Returns an empty string when neither is set.
func TraceIDFromContext(ctx context.Context) TraceID {
	values := correlationFromContext(ctx)
	if values.traceID != "" {
		return values.traceID
	}
	return TraceID(values.trace.TraceID)
}

// SpanIDFromContext extracts the span ID the framework minted for this
// operation. It is empty when the context only carries a remote parent
// extracted from an inbound request and no framework span was started on this
// node: reporting the parent's ID as this node's span would build a wrong
// tree. TracingMiddleware and ContinueIncomingTrace mint one.
func SpanIDFromContext(ctx context.Context) SpanID {
	values := correlationFromContext(ctx)
	if !values.spanOwned {
		return ""
	}
	return SpanID(values.trace.SpanID)
}

// WithRequestID injects a request ID into the context.
func WithRequestID(ctx context.Context, id string) context.Context {
	values := correlationFromContext(ctx)
	values.requestID = id
	return withCorrelation(ctx, values)
}

// RequestIDFromContext extracts the request ID from the context.
func RequestIDFromContext(ctx context.Context) string {
	return correlationFromContext(ctx).requestID
}

// correlation holds the request's correlation identifiers in one context value.
//
// One key means one node per write and one lookup per read, and it also means
// precedence follows the order the values were installed: a later With* wins
// because it replaces the whole set, which is what a caller overriding an
// identifier expects.
type correlation struct {
	trace     TraceContext
	traceID   TraceID
	requestID string
	// spanOwned marks trace.SpanID as minted by this node. Extraction installs
	// a remote parent, whose ID must not be reported as this node's span.
	spanOwned bool
}

func withCorrelation(ctx context.Context, values correlation) context.Context {
	return context.WithValue(ctx, correlationKey{}, values)
}

func correlationFromContext(ctx context.Context) correlation {
	values, _ := ctx.Value(correlationKey{}).(correlation)
	return values
}

// newID generates a short random hex ID.
func newID() string {
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		fillWeakRandom(b[:])
	}
	return hex.EncodeToString(b[:])
}

// newSpanID generates a random W3C-conformant span ID, which is the same 8
// random bytes a request ID is.
func newSpanID() string {
	return newID()
}

// fillWeakRandom is the degraded path for a system whose entropy source failed.
// Correlation gets weaker; the identifier keeps the length and alphabet the
// specification requires, and the request still proceeds.
func fillWeakRandom(b []byte) {
	for i := 0; i < len(b); i += 8 {
		var word [8]byte
		binary.LittleEndian.PutUint64(word[:], rand.Uint64()) //nolint:gosec
		copy(b[i:], word[:])
	}
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
			return next(ContinueIncomingTrace(ctx), request)
		}
	}
}

// NextAttempt mints a fresh span ID for one retry attempt of the current
// operation, keeping the trace ID, flags and request ID. It is a no-op
// unless the context carries a span this node started — a remote parent's
// span ID is the caller's, not something a retry may replace — so wiring it
// into a retry loop is safe for every position in the chain.
//
// Downstream services and flat log records carry the span ID, so per-attempt
// IDs are what make attempt N attributable in a replay: without this, every
// attempt of one operation logs the same span and only timestamps tell them
// apart.
//
// Stable: endpoint.next-attempt-span — NextAttempt mints a new span ID for
// a span this node owns and leaves trace ID, flags and request ID
// unchanged; on a remote-parent context it does nothing.
// Covered by: TestNextAttemptMintsPerCall, TestNextAttemptNoopForRemoteParent,
// TestNextAttemptKeepsRequestID
func NextAttempt(ctx context.Context) context.Context {
	values := correlationFromContext(ctx)
	if !values.spanOwned || !values.trace.Valid() {
		return ctx
	}
	values.trace.SpanID = newSpanID()
	return withCorrelation(ctx, values)
}

// ContinueIncomingTrace advances the correlation for one operation on this
// node: a valid trace context keeps its trace ID and flags and receives a
// freshly minted span ID; a bare WithTraceID value is bridged into a W3C
// context; otherwise a new trace is minted. A request ID is minted when the
// context carries none.
//
// It is the identity step TracingMiddleware performs, exposed for a server
// boundary that has no endpoint chain to hang the middleware on — a handler
// mounted raw on the mux. TracingMiddleware stays the right call for an
// endpoint.
//
// Stable: endpoint.trace-id-derives-from-context — an ID extracted from an
// inbound traceparent is visible to TraceIDFromContext without running
// TracingMiddleware, and SpanIDFromContext reports this node's span only
// after the correlation was continued here.
// Covered by: TestTraceIDFromContextDerivesFromExtractedContext,
// TestContinueIncomingTraceMintsAChildSpan, TestSpanIDIsEmptyForARemoteParent
func ContinueIncomingTrace(ctx context.Context) context.Context {
	values := correlationFromContext(ctx)
	switch {
	case values.trace.Valid():
		values.trace.SpanID = newSpanID()
	case values.traceID != "":
		values.trace = TraceContext{TraceID: string(values.traceID), SpanID: newSpanID(), Flags: "00"}
		if !values.trace.Valid() {
			// A non-W3C string keeps its published semantics — it stays
			// what TraceIDFromContext reports for logging — while the
			// propagated trace is minted fresh, because a string that is
			// not a W3C trace ID cannot be one. The two identifiers
			// diverging in that one case is the documented cost of the
			// compatibility entrance, recorded in the roadmap.
			values.trace = NewTraceContext()
		}
	default:
		values.trace = NewTraceContext()
	}
	if values.traceID == "" {
		values.traceID = TraceID(values.trace.TraceID)
	}
	if values.requestID == "" {
		values.requestID = newID()
	}
	values.spanOwned = true
	return withCorrelation(ctx, values)
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
	return b.UseNamed("tracing", TracingMiddleware())
}
