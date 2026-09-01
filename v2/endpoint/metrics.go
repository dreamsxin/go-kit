package endpoint

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Observation is one endpoint call outcome handed to a Recorder.
type Observation struct {
	// Operation names the endpoint, for example the route pattern. It is the
	// label dimension a metrics backend groups by. Empty means unlabeled.
	Operation string
	// Duration is how long the call took, including the middleware inside the
	// recording middleware.
	Duration time.Duration
	// Err is the endpoint error, or nil on success.
	Err error
}

// Recorder receives one Observation per endpoint call. Implement it to bridge
// endpoint calls to Prometheus, OpenTelemetry, or any other backend; the
// framework itself only ships the in-memory Metrics collector.
//
// Observe runs on the request path and must not block.
type Recorder interface {
	Observe(ctx context.Context, obs Observation)
}

// RecorderFunc adapts a function to Recorder.
type RecorderFunc func(ctx context.Context, obs Observation)

// Observe implements Recorder.
func (f RecorderFunc) Observe(ctx context.Context, obs Observation) { f(ctx, obs) }

// counters holds the raw tallies for one operation, or for the total.
type counters struct {
	requestCount    int64
	errorCount      int64
	successCount    int64
	totalDuration   time.Duration
	lastRequestTime time.Time
}

func (c *counters) add(duration time.Duration, at time.Time, err error) {
	c.requestCount++
	c.lastRequestTime = at
	c.totalDuration += duration
	if err != nil {
		c.errorCount++
		return
	}
	c.successCount++
}

func (c *counters) snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		RequestCount:    c.requestCount,
		ErrorCount:      c.errorCount,
		SuccessCount:    c.successCount,
		TotalDuration:   c.totalDuration,
		LastRequestTime: c.lastRequestTime,
	}
}

// Metrics is the in-memory collector RecordingMiddleware writes into. It keeps
// a total plus one tally per operation, so a service can report per-route
// numbers without an external backend. Its state is guarded internally; read it
// through Snapshot or SnapshotFor.
//
// Metrics implements Recorder. It stores one entry per distinct operation, so
// operation labels must come from a bounded set such as route patterns, never
// from request data. For latency distributions or exported time series,
// implement Recorder against a metrics backend instead.
type Metrics struct {
	mu          sync.Mutex
	total       counters
	byOperation map[string]*counters
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

// Snapshot returns a point-in-time total across every operation.
func (m *Metrics) Snapshot() MetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.total.snapshot()
}

// SnapshotFor returns a point-in-time view of one operation. Unknown
// operations report a zero snapshot.
func (m *Metrics) SnapshotFor(operation string) MetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.byOperation[operation]
	if !ok {
		return MetricsSnapshot{}
	}
	return entry.snapshot()
}

// Operations returns the recorded operation labels in sorted order.
func (m *Metrics) Operations() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	operations := make([]string, 0, len(m.byOperation))
	for operation := range m.byOperation {
		operations = append(operations, operation)
	}
	sort.Strings(operations)
	return operations
}

// Observe implements Recorder.
func (m *Metrics) Observe(_ context.Context, obs Observation) {
	at := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.total.add(obs.Duration, at, obs.Err)
	if obs.Operation == "" {
		return
	}
	if m.byOperation == nil {
		m.byOperation = make(map[string]*counters)
	}
	entry, ok := m.byOperation[obs.Operation]
	if !ok {
		entry = &counters{}
		m.byOperation[obs.Operation] = entry
	}
	entry.add(obs.Duration, at, obs.Err)
}

// RecordingMiddleware times every call and reports it to each recorder under
// the given operation label. Recorders are called after the wrapped endpoint
// returns, in the order they were passed.
//
// Place it outermost so it measures the whole chain, including rejections from
// rate limiting or a circuit breaker:
//
//	ep := endpoint.NewBuilder(createUser).
//	    WithRecording("POST /users", promRecorder, &metrics).
//	    WithRateLimit(limiter).
//	    Build()
//
// It panics when a recorder is nil so misassembly fails at startup rather than
// on the first request.
func RecordingMiddleware(operation string, recorders ...Recorder) Middleware {
	for _, recorder := range recorders {
		if recorder == nil {
			panic("endpoint: recorder cannot be nil")
		}
	}
	if len(recorders) == 0 {
		return func(next Endpoint) Endpoint { return next }
	}
	observers := append([]Recorder(nil), recorders...)
	return func(next Endpoint) Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			start := time.Now()
			response, err := next(ctx, request)
			obs := Observation{
				Operation: operation,
				Duration:  time.Since(start),
				Err:       err,
			}
			for _, recorder := range observers {
				recorder.Observe(ctx, obs)
			}
			return response, err
		}
	}
}

// WithRecording appends a RecordingMiddleware to the Builder.
func (b *Builder) WithRecording(operation string, recorders ...Recorder) *Builder {
	return b.UseNamed("recording:"+operation, RecordingMiddleware(operation, recorders...))
}

// MetricsMiddleware records unlabeled calls into the in-memory collector. It is
// RecordingMiddleware with no operation label; use RecordingMiddleware when the
// counters should be grouped per endpoint.
//
// It panics when metrics is nil so misassembly fails at startup rather than on
// the first request.
func MetricsMiddleware(metrics *Metrics) Middleware {
	if metrics == nil {
		panic("endpoint: metrics collector cannot be nil")
	}
	return RecordingMiddleware("", metrics)
}
