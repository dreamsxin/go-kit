package grpc

type contextKey int

const (
	// ContextKeyRequestMethod names the RPC method a call is invoking.
	ContextKeyRequestMethod contextKey = iota

	// Its value is of type metadata.MD.
	ContextKeyResponseHeaders

	// Its value is of type metadata.MD.
	ContextKeyResponseTrailers
)
