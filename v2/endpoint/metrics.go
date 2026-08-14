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

// MetricsSnapshot is a detached point-in-time view of Metrics. It contains no
// synchronization state and is safe to copy or pass by value.
type MetricsSnapshot struct {
	RequestCount    int64
	ErrorCount      int64
	SuccessCount    int64
	TotalDuration   time.Duration
	LastRequestTime time.Time
}

// AverageDuration returns the mean duration of recorded requests.
func (m MetricsSnapshot) AverageDuration() time.Duration {
	if m.RequestCount == 0 {
		return 0
	}
	return m.TotalDuration / time.Duration(m.RequestCount)
}

// Snapshot returns a point-in-time value that is safe to read and copy.
func (m *Metrics) Snapshot() MetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return MetricsSnapshot{
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
