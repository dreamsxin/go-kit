package endpoint

import (
	"context"
	"time"
)

// Metrics 端点指标
type Metrics struct {
	RequestCount    int64
	ErrorCount      int64
	SuccessCount    int64
	TotalDuration   time.Duration
	LastRequestTime time.Time
}

// MetricsMiddleware 指标收集中间件
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
