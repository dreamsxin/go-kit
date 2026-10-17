// Package zapadapter integrates the core endpoint abstraction with the
// Zap logger.
package zapadapter

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/transport"
)

// NewErrorHandler adapts a Zap logger to transport.ErrorHandler.
//
// Stable: zap.error-handler-nil-logger — NewErrorHandler accepts a nil logger
// and returns a usable handler.
// Covered by: TestNewErrorHandlerAcceptsNilLogger
func NewErrorHandler(logger *zap.Logger) transport.ErrorHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return transport.ErrorHandlerFunc(func(_ context.Context, err error) {
		logger.Error("transport error", zap.Error(err))
	})
}

// LoggingMiddleware records the operation outcome, elapsed duration, and the
// correlation identifiers the context carries, using the same attribute
// vocabulary as observability/slog's LoggingMiddleware — operation, duration,
// success, trace_id, span_id, request_id — so a deployment mixing the two can
// query one schema.
//
// A failure and a panic are logged at Error because an error-only-at-Info
// record never reaches an error alert. The panic record is emitted from a
// defer while the stack unwinds, where err is still nil, so the success field
// is computed from whether the endpoint returned. Install
// endpoint.RecoveryMiddleware inside this one if the panic should become an
// error instead.
//
// Stable: zap.middleware-transparency — LoggingMiddleware returns the
// endpoint's response and error unchanged, and a nil logger becomes a no-op
// logger.
// Covered by: TestLoggingMiddlewareSuccess, TestLoggingMiddlewareError, TestLoggingMiddlewareNilLogger
//
// Stable: zap.record-fields — every endpoint record carries operation,
// duration and success; trace_id/span_id/request_id are added when present;
// a failed or panicking call logs at Error.
// Covered by: TestLoggingMiddlewareRecordsCorrelationAndLevel
func LoggingMiddleware(logger *zap.Logger, operation string) endpoint.Middleware {
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (resp any, err error) {
			start := time.Now()
			returned := false
			defer func() {
				fields := []zap.Field{
					zap.String("operation", operation),
					zap.Duration("duration", time.Since(start)),
				}
				if traceID := endpoint.TraceIDFromContext(ctx); traceID != "" {
					fields = append(fields, zap.String("trace_id", string(traceID)))
				}
				if spanID := endpoint.SpanIDFromContext(ctx); spanID != "" {
					fields = append(fields, zap.String("span_id", string(spanID)))
				}
				if requestID := endpoint.RequestIDFromContext(ctx); requestID != "" {
					fields = append(fields, zap.String("request_id", requestID))
				}
				switch {
				case !returned:
					fields = append(fields, zap.Bool("success", false))
					logger.Error("endpoint call panicked", fields...)
				case err != nil:
					fields = append(fields, zap.Bool("success", false), zap.Error(err))
					logger.Error("endpoint call failed", fields...)
				default:
					fields = append(fields, zap.Bool("success", true))
					logger.Info("endpoint call succeeded", fields...)
				}
			}()
			resp, err = next(ctx, request)
			returned = true
			return resp, err
		}
	}
}
