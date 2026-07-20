// Package slogadapter provides an optional log/slog adapter for endpoint
// middleware. It does not change the framework's zap-based log package.
package slogadapter

import (
	"context"
	"log/slog"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

const defaultLevel = slog.LevelInfo

// Options controls the fields emitted by LoggingMiddleware.
type Options struct {
	Level slog.Level
	Attrs func(context.Context) []slog.Attr
}

// Option configures Options.
type Option func(*Options)

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
			response, err = next(ctx, request)

			attrs := []slog.Attr{
				slog.String("operation", operation),
				slog.Duration("duration", time.Since(start)),
				slog.Bool("success", err == nil),
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
			if err != nil {
				attrs = append(attrs, slog.Any("error", err))
				logger.LogAttrs(ctx, cfg.Level, "endpoint call failed", attrs...)
			} else {
				logger.LogAttrs(ctx, cfg.Level, "endpoint call succeeded", attrs...)
			}
			return response, err
		}
	}
}
