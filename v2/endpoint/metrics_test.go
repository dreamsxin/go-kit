package endpoint_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

func TestMetricsSnapshotAverageDuration(t *testing.T) {
	tests := []struct {
		name     string
		snapshot endpoint.MetricsSnapshot
		want     time.Duration
	}{
		{name: "empty", snapshot: endpoint.MetricsSnapshot{}, want: 0},
		{
			name: "recorded requests",
			snapshot: endpoint.MetricsSnapshot{
				RequestCount:  2,
				TotalDuration: 200 * time.Millisecond,
			},
			want: 100 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.snapshot.AverageDuration(); got != tt.want {
				t.Fatalf("AverageDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRecordingMiddlewareSeparatesOperations(t *testing.T) {
	var metrics endpoint.Metrics
	ok := endpoint.RecordingMiddleware("POST /users", &metrics)(
		func(context.Context, any) (any, error) { return "ok", nil },
	)
	failing := endpoint.RecordingMiddleware("GET /users", &metrics)(
		func(context.Context, any) (any, error) { return nil, errors.New("boom") },
	)

	_, _ = ok(context.Background(), nil)
	_, _ = ok(context.Background(), nil)
	_, _ = failing(context.Background(), nil)

	total := metrics.Snapshot()
	if total.RequestCount != 3 || total.SuccessCount != 2 || total.ErrorCount != 1 {
		t.Fatalf("total: %+v", total)
	}

	created := metrics.SnapshotFor("POST /users")
	if created.RequestCount != 2 || created.ErrorCount != 0 {
		t.Errorf("POST /users: %+v", created)
	}
	listed := metrics.SnapshotFor("GET /users")
	if listed.RequestCount != 1 || listed.ErrorCount != 1 {
		t.Errorf("GET /users: %+v", listed)
	}
	if unknown := metrics.SnapshotFor("DELETE /users"); unknown.RequestCount != 0 {
		t.Errorf("unknown operation should be zero: %+v", unknown)
	}

	operations := metrics.Operations()
	if len(operations) != 2 || operations[0] != "GET /users" || operations[1] != "POST /users" {
		t.Errorf("Operations() = %v", operations)
	}
}

func TestMetricsMiddlewareRecordsWithoutOperationLabel(t *testing.T) {
	var metrics endpoint.Metrics
	ep := endpoint.MetricsMiddleware(&metrics)(
		func(context.Context, any) (any, error) { return "ok", nil },
	)

	_, _ = ep(context.Background(), nil)

	if got := metrics.Snapshot().RequestCount; got != 1 {
		t.Fatalf("total requests: got %d, want 1", got)
	}
	if operations := metrics.Operations(); len(operations) != 0 {
		t.Errorf("unlabeled calls must not create operations: %v", operations)
	}
}

func TestRecordingMiddlewareFansOutToEveryRecorder(t *testing.T) {
	var observed []endpoint.Observation
	first := endpoint.RecorderFunc(func(_ context.Context, obs endpoint.Observation) {
		observed = append(observed, obs)
	})
	var metrics endpoint.Metrics
	ep := endpoint.RecordingMiddleware("GET /ping", first, &metrics)(
		func(context.Context, any) (any, error) { return "pong", nil },
	)

	_, _ = ep(context.Background(), nil)

	if len(observed) != 1 {
		t.Fatalf("recorder calls: got %d, want 1", len(observed))
	}
	if observed[0].Operation != "GET /ping" || observed[0].Err != nil {
		t.Errorf("observation: %+v", observed[0])
	}
	if got := metrics.SnapshotFor("GET /ping").RequestCount; got != 1 {
		t.Errorf("collector requests: got %d, want 1", got)
	}
}

func TestRecordingMiddlewareRejectsNilRecorder(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("want panic on nil recorder")
		}
	}()
	endpoint.RecordingMiddleware("GET /ping", nil)
}
