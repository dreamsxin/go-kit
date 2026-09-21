package transport

import "context"

// NopErrorHandler is an ErrorHandler that discards all errors silently.
var NopErrorHandler ErrorHandler = ErrorHandlerFunc(func(_ context.Context, _ error) {})

// ErrorHandler observes errors a transport encounters outside the endpoint
// result — while decoding, encoding, or relaying. The default NopErrorHandler
// discards them: the transport still encodes the response, and nothing records
// why. Nothing wires one for you; pass one to the server option explicitly —
// observability/slog.NewErrorHandler is a ready implementation.
type ErrorHandler interface {
	Handle(ctx context.Context, err error)
}

// ErrorHandlerFunc adapts a function to ErrorHandler.
type ErrorHandlerFunc func(ctx context.Context, err error)

func (f ErrorHandlerFunc) Handle(ctx context.Context, err error) {
	f(ctx, err)
}
