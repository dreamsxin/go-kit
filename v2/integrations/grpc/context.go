package grpc

type contextKey int

const (
	// ContextKeyRequestMethod names the RPC method a call is invoking; its
	// value is the full method name, a string.
	ContextKeyRequestMethod contextKey = iota

	// ContextKeyResponseHeaders carries the headers a call answered with; its
	// value is of type metadata.MD.
	ContextKeyResponseHeaders

	// ContextKeyResponseTrailers carries the trailers a call answered with;
	// its value is of type metadata.MD.
	ContextKeyResponseTrailers
)
