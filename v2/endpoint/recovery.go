package endpoint

import (
	"context"

	"github.com/dreamsxin/go-kit/v2/apperror"
)

// PanicHandler decides how a recovered endpoint panic is reported. The
// recovered value is intentionally passed only to the handler so applications
// can log it without putting it on the wire. Returning nil is treated as an
// internal error.
type PanicHandler func(ctx context.Context, request any, recovered any) error

// DefaultPanicHandler converts a recovered panic to a redacted internal
// application error. Applications should wrap it when they need structured
// logging or an error reporter:
//
//	endpoint.RecoveryMiddleware(func(ctx context.Context, req, recovered any) error {
//		logger.ErrorContext(ctx, "endpoint panic",
//			"panic", recovered,
//			"stack", string(debug.Stack()), // the handler still runs on the panicking goroutine
//		)
//		return endpoint.DefaultPanicHandler(ctx, req, recovered)
//	})
//
// Capture the stack in the handler, not from the recovered value: the value is
// whatever was passed to panic and carries no stack of its own, and by the time
// the middleware returns the frames are gone.
func DefaultPanicHandler(context.Context, any, any) error {
	return apperror.Internal("endpoint.panic", "endpoint panic recovered")
}

// RecoveryMiddleware converts panics raised by the wrapped endpoint or inner
// middleware into an error. The default result is an apperror internal error,
// which the built-in transports redact as a generic 500 response. A custom
// handler may log or report the recovered value and return a classified error.
//
// Recovery must normally be the outermost endpoint middleware so it covers the
// complete chain. A nil handler selects DefaultPanicHandler.
func RecoveryMiddleware(handler PanicHandler) Middleware {
	if handler == nil {
		handler = DefaultPanicHandler
	}
	return func(next Endpoint) Endpoint {
		return func(ctx context.Context, request any) (response any, err error) {
			defer func() {
				if recovered := recover(); recovered != nil {
					response = nil
					err = callPanicHandler(handler, ctx, request, recovered)
					if err == nil {
						err = DefaultPanicHandler(ctx, request, recovered)
					}
				}
			}()
			return next(ctx, request)
		}
	}
}

func callPanicHandler(handler PanicHandler, ctx context.Context, request any, recovered any) (err error) {
	defer func() {
		if panicValue := recover(); panicValue != nil {
			err = DefaultPanicHandler(ctx, request, recovered)
		}
	}()
	return handler(ctx, request, recovered)
}

// WithRecovery appends RecoveryMiddleware to the Builder.
func (b *Builder) WithRecovery(handler PanicHandler) *Builder {
	return b.UseNamed("recovery", RecoveryMiddleware(handler))
}
