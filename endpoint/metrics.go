package endpoint

import (
	"context"
	"time"
)

// Metrics holds counters and timing data collected by MetricsMiddleware.
// All fields are updated in-place; use atomic reads if you need to observe
// them from a different goroutine.
type Metrics struct {
	RequestCount    int64
	ErrorCount      int64
	SuccessCount    int64
	TotalDuration   time.Duration
	LastRequestTime time.Time
}

// MetricsMiddleware returns a Middleware that records per-endpoint metrics
// into the provided Metrics struct.  It increments RequestCount on every
// call, SuccessCount when the next Endpoint returns nil error, and
// ErrorCount otherwise.
func MetricsMiddleware(metrics *Metrics) Middleware {
	return func(next Endpoint) Endpoint {
		return func(ctx context.Context, request interface{}) (interface{}, error) {
			start := time.Now()
			metrics.RequestCount++
			metrics.LastRequestTime = time.Now()

			response, err := next(ctx, request)

			duration := time.Since(start)
			metrics.TotalDuration += duration

			if err != nil {
				metrics.ErrorCount++
			} else {
				metrics.SuccessCount++
			}

			return response, err
		}
	}
}
