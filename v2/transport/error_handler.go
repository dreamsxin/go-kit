package transport

import (
	"context"
	"log/slog"
)

// NopErrorHandler is an ErrorHandler that discards all errors silently. Use
// it to restore silence deliberately; it is no longer the default, because an
// error the transport answers is an error the operator can see.
var NopErrorHandler ErrorHandler = ErrorHandlerFunc(func(_ context.Context, _ error) {})

// DefaultErrorHandler logs the error through slog's default logger at Warn
// level. It is what both serving transports fall back to when no
// ServerErrorHandler is set: an error the transport answered — a rejected
// body, a failed decode — is recorded where the deployment's logs go, instead
// of vanishing behind a correct status code. It carries only the error
// itself; observability/slog.NewErrorHandler is the rich adapter (request
// correlation, level and attribute control) for a service that wants more
// than the default.
//
// Stable: transport.default-error-handler — a transport server constructed
// without an explicit error handler logs errors through slog's default
// logger at Warn level, on both the request/response server and the SSE
// server.
// Covered by: TestUnhandledDecodeErrorIsLogged, TestNopErrorHandlerSilences,
// TestSSEServerDefaultsToTheLoggingErrorHandler
var DefaultErrorHandler ErrorHandler = ErrorHandlerFunc(func(ctx context.Context, err error) {
	slog.WarnContext(ctx, "go-kit transport error", "err", err)
})

// ErrorHandler observes errors a transport encounters outside the endpoint
// result — while decoding, encoding, relaying, or after an SSE stream
// started. Both serving transports default to DefaultErrorHandler; pass
// NopErrorHandler to restore deliberate silence, or wire a richer adapter
// such as observability/slog.NewErrorHandler for request correlation,
// level and attribute control.
type ErrorHandler interface {
	Handle(ctx context.Context, err error)
}

// ErrorHandlerFunc adapts a function to ErrorHandler.
type ErrorHandlerFunc func(ctx context.Context, err error)

func (f ErrorHandlerFunc) Handle(ctx context.Context, err error) {
	f(ctx, err)
}
