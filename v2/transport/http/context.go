package http

// The HTTP client's finalizer publishes what a response answered with, and a
// ResponseFunc reads the values back through these keys. Request-header keys
// have no equivalent: a RequestFunc receives the *http.Request itself, so a
// hook that needs a header reads it there — the old PopulateRequestContext,
// which copied request headers (credentials included) into the context for
// any layer to read, was removed as the dead surface it was.
type contextKey int

const (
	// Its value is of type http.Header
	ContextKeyResponseHeaders contextKey = iota

	// Its value is of type int64.
	ContextKeyResponseSize
)
