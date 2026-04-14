package endpoint

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/dreamsxin/go-kit/log"
)

// LoggingMiddleware returns a Middleware that logs each call to the wrapped
// Endpoint using the provided zap logger.  It records:
//   - the operation name
//   - whether the call succeeded or failed
//   - the elapsed duration
//
// Example:
//
//	logger, _ := log.NewDevelopment()
//	ep = endpoint.LoggingMiddleware(logger, "CreateUser")(ep)
func LoggingMiddleware(logger *log.Logger, operation string) Middleware {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return func(next Endpoint) Endpoint {
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
				} else {
					logger.Info("endpoint call succeeded", fields...)
				}
			}()
			return next(ctx, request)
		}
	}
}
