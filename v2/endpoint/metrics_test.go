package endpoint_test

import (
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
