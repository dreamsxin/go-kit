package endpoint

import (
	"context"
	"sync"
	"time"
)

// Metrics is the mutable collector MetricsMiddleware writes into. Its state is
// guarded internally; read it through Snapshot, which returns a detached value
// that is safe to copy and to read field by field.
type Metrics struct {
	mu              sync.Mutex
	requestCount    int64
	errorCount      int64
	successCount    int64
	totalDuration   time.Duration
	lastRequestTime time.Time
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
		RequestCount:    m.requestCount,
		ErrorCount:      m.errorCount,
		SuccessCount:    m.successCount,
		TotalDuration:   m.totalDuration,
		LastRequestTime: m.lastRequestTime,
	}
}

func (m *Metrics) record(duration time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestCount++
	m.lastRequestTime = time.Now()
	m.totalDuration += duration
	if err != nil {
		m.errorCount++
		return
	}
	m.successCount++
}

// MetricsMiddleware returns a Middleware that records per-endpoint metrics
// into the provided collector. It counts every call, separates successes from
// errors, and accumulates duration. All operations are goroutine-safe.
//
// It panics when metrics is nil so misassembly fails at startup rather than on
// the first request.
func MetricsMiddleware(metrics *Metrics) Middleware {
	if metrics == nil {
		panic("endpoint: metrics collector cannot be nil")
	}
	return func(next Endpoint) Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			start := time.Now()
			response, err := next(ctx, request)
			metrics.record(time.Since(start), err)
			return response, err
		}
	}
}
