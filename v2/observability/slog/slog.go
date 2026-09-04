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
// A panic is logged at Error level as a panic and left to propagate. The record
// is emitted from a defer, so a panicking call is never silently unlogged.
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
