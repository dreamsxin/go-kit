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
func NewErrorHandler(logger *zap.Logger) transport.ErrorHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return transport.ErrorHandlerFunc(func(_ context.Context, err error) {
		logger.Error("transport error", zap.Error(err))
	})
}

// LoggingMiddleware records the operation outcome and elapsed duration.
func LoggingMiddleware(logger *zap.Logger, operation string) endpoint.Middleware {
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (resp any, err error) {
			start := time.Now()
			defer func() {
				fields := []zap.Field{
					zap.String("op", operation),
					zap.Duration("took", time.Since(start)),
				}
				if err != nil {
					fields = append(fields, zap.Error(err))
					logger.Info("endpoint call failed", fields...)
					return
				}
				logger.Info("endpoint call succeeded", fields...)
			}()
			return next(ctx, request)
		}
	}
}
