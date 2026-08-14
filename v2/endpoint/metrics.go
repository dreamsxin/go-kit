package endpoint

import (
	"context"
	"sync"
	"time"
)

// Metrics holds counters and timing data collected by MetricsMiddleware.
// The exported fields remain for v2 source compatibility. Concurrent readers
// must use Snapshot rather than reading the fields directly.
type Metrics struct {
	mu sync.Mutex

	RequestCount    int64
	ErrorCount      int64
	SuccessCount    int64
	TotalDuration   time.Duration
	LastRequestTime time.Time
}

// Snapshot returns a point-in-time copy that is safe to read without a lock.
func (m *Metrics) Snapshot() Metrics {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Metrics{
		RequestCount:    m.RequestCount,
		ErrorCount:      m.ErrorCount,
		SuccessCount:    m.SuccessCount,
		TotalDuration:   m.TotalDuration,
		LastRequestTime: m.LastRequestTime,
	}
}

// MetricsMiddleware returns a Middleware that records per-endpoint metrics
// into the provided Metrics struct.  It increments RequestCount on every
// call, SuccessCount when the next Endpoint returns nil error, and
// ErrorCount otherwise.  All operations are goroutine-safe.
func MetricsMiddleware(metrics *Metrics) Middleware {
	return func(next Endpoint) Endpoint {
		return func(ctx context.Context, request interface{}) (interface{}, error) {
			start := time.Now()

			response, err := next(ctx, request)

			duration := time.Since(start)

			metrics.mu.Lock()
			metrics.RequestCount++
			metrics.LastRequestTime = time.Now()
			metrics.TotalDuration += duration
			if err != nil {
				metrics.ErrorCount++
			} else {
				metrics.SuccessCount++
			}
			metrics.mu.Unlock()

			return response, err
		}
	}
}
