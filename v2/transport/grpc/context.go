package grpc

type contextKey int

const (
	ContextKeyRequestMethod contextKey = iota

	// Its value is of type metadata.MD.
	ContextKeyResponseHeaders

	// Its value is of type metadata.MD.
	ContextKeyResponseTrailers
)
