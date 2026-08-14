package transport

import "context"

// NopErrorHandler is an ErrorHandler that discards all errors silently.
var NopErrorHandler ErrorHandler = ErrorHandlerFunc(func(_ context.Context, _ error) {})

type ErrorHandler interface {
	Handle(ctx context.Context, err error)
}

type ErrorHandlerFunc func(ctx context.Context, err error)

func (f ErrorHandlerFunc) Handle(ctx context.Context, err error) {
	f(ctx, err)
}
