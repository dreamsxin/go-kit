// Package slogadapter provides an optional log/slog adapter for endpoint
// middleware without depending on the framework's Zap adapter.
package slogadapter

import (
	"context"
	"log/slog"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/transport"
)

const defaultLevel = slog.LevelInfo

// Options controls the fields emitted by LoggingMiddleware.
type Options struct {
	Level slog.Level
	Attrs func(context.Context) []slog.Attr
}

// Option configures Options.
type Option func(*Options)

// NewErrorHandler adapts a standard-library slog logger to
// transport.ErrorHandler. Logger and handler setup remain application owned.
//
// Stable: slog.error-handler — NewErrorHandler logs a transport error at Error
// level with the error attached, and a nil logger falls back to slog.Default().
// Covered by: TestNewErrorHandlerRecordsError
func NewErrorHandler(logger *slog.Logger) transport.ErrorHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return transport.ErrorHandlerFunc(func(ctx context.Context, err error) {
		logger.ErrorContext(ctx, "transport error", slog.Any("error", err))
	})
}

// WithLevel changes the level used for both success and failure records.
func WithLevel(level slog.Level) Option {
	return func(options *Options) { options.Level = level }
}

// WithAttrs adds application-owned attributes without logging request or
// response payloads. Keep the returned attributes bounded and non-sensitive.
func WithAttrs(attrs func(context.Context) []slog.Attr) Option {
	return func(options *Options) { options.Attrs = attrs }
}

// LoggingMiddleware records endpoint outcome, duration, and correlation IDs
// using the standard library slog API. Logger setup and handler selection stay
// under application control.
//
// Stable: slog.record-span-id — a record carrying a trace context this node
// continued names that operation's span_id; an inbound-only context (a remote
// parent this node did not continue) logs the trace_id without a span_id, so a
// log replay never sees the caller's span misnamed as this node's.
// Covered by: TestLoggingMiddlewareRecordsSpanID, TestLoggingMiddlewareDoesNotMintARemoteSpanID
//
// A panic is logged at Error level as a panic and left to propagate. The record
// is emitted from a defer, so a panicking call is never silently unlogged.
//
// Stable: slog.record-fields — every endpoint record carries operation,
// duration, success, and trace_id/span_id/request_id when present; WithAttrs
// adds application-owned attributes without logging request or response
// payloads. span_id names this node's span only, so nested calls sharing one
// trace are distinguishable in a log replay without misnaming a remote parent.
// Covered by: TestLoggingMiddlewareRecordsBoundedContext, TestLoggingMiddlewareRecordsSpanID
//
// Stable: slog.panic-logged — a panicking call is logged at Error level as a
// panic, and the panic still propagates.
// Covered by: TestLoggingMiddlewareLogsAPanic
//
// Stable: slog.nil-logger-default — a nil logger falls back to slog.Default().
// Covered by: TestLoggingMiddlewareUsesDefaultLoggerWhenNil
func LoggingMiddleware(logger *slog.Logger, operation string, options ...Option) endpoint.Middleware {
	if logger == nil {
		logger = slog.Default()
	}
	cfg := Options{Level: defaultLevel}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}

	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (response any, err error) {
			start := time.Now()
			returned := false
			defer func() {
				attrs := []slog.Attr{
					slog.String("operation", operation),
					slog.Duration("duration", time.Since(start)),
					slog.Bool("success", returned && err == nil),
				}
				if traceID := endpoint.TraceIDFromContext(ctx); traceID != "" {
					attrs = append(attrs, slog.String("trace_id", string(traceID)))
				}
				if spanID := endpoint.SpanIDFromContext(ctx); spanID != "" {
					attrs = append(attrs, slog.String("span_id", string(spanID)))
				}
				if requestID := endpoint.RequestIDFromContext(ctx); requestID != "" {
					attrs = append(attrs, slog.String("request_id", requestID))
				}
				if cfg.Attrs != nil {
					attrs = append(attrs, cfg.Attrs(ctx)...)
				}
				switch {
				case !returned:
					logger.LogAttrs(ctx, slog.LevelError, "endpoint call panicked", attrs...)
				case err != nil:
					attrs = append(attrs, slog.Any("error", err))
					logger.LogAttrs(ctx, cfg.Level, "endpoint call failed", attrs...)
				default:
					logger.LogAttrs(ctx, cfg.Level, "endpoint call succeeded", attrs...)
				}
			}()

			response, err = next(ctx, request)
			returned = true
			return response, err
		}
	}
}
