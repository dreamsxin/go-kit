package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	grpcserver "github.com/dreamsxin/go-kit/v2/integrations/grpc/server"
	"google.golang.org/grpc/codes"
)

// TestRecorderReportsPerMethodSeries is the point of the milestone: a scrape of a
// gRPC-only service says something, and it says it under the same names an HTTP
// service uses.
func TestRecorderReportsPerMethodSeries(t *testing.T) {
	collector := &endpoint.Metrics{}
	recorder := Recorder(collector)

	recorder.ObserveGRPC(context.Background(), grpcserver.Observation{
		Method:   "/users.Users/Get",
		Code:     codes.OK,
		Duration: 5 * time.Millisecond,
	})
	recorder.ObserveGRPC(context.Background(), grpcserver.Observation{
		Method:   "/users.Users/Get",
		Code:     codes.OK,
		Duration: 7 * time.Millisecond,
	})
	recorder.ObserveGRPC(context.Background(), grpcserver.Observation{
		Method:   "/users.Users/Watch",
		Code:     codes.OK,
		Stream:   true,
		Duration: time.Second,
	})

	get := collector.SnapshotFor("/users.Users/Get")
	if get.RequestCount != 2 {
		t.Errorf("Get requests = %d, want 2", get.RequestCount)
	}
	if get.ErrorCount != 0 {
		t.Errorf("Get errors = %d, want 0", get.ErrorCount)
	}
	watch := collector.SnapshotFor("/users.Users/Watch")
	if watch.RequestCount != 1 {
		t.Errorf("Watch requests = %d, want 1", watch.RequestCount)
	}

	operations := collector.Operations()
	if len(operations) != 2 {
		t.Fatalf("operations = %v, want one per method", operations)
	}
}

func TestRecorderCountsServerErrorsOnly(t *testing.T) {
	cases := []struct {
		code      codes.Code
		wantError bool
	}{
		{codes.OK, false},
		{codes.NotFound, false},
		{codes.InvalidArgument, false},
		{codes.PermissionDenied, false},
		{codes.Canceled, false},
		{codes.Internal, true},
		{codes.Unavailable, true},
		{codes.DeadlineExceeded, true},
		{codes.Unknown, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.code.String(), func(t *testing.T) {
			collector := &endpoint.Metrics{}
			Recorder(collector).ObserveGRPC(context.Background(), grpcserver.Observation{
				Method:   "/users.Users/Get",
				Code:     testCase.code,
				Duration: time.Millisecond,
			})
			snapshot := collector.SnapshotFor("/users.Users/Get")
			if got := snapshot.ErrorCount > 0; got != testCase.wantError {
				t.Errorf("%s recorded as error = %v, want %v", testCase.code, got, testCase.wantError)
			}
			if snapshot.RequestCount != 1 {
				t.Errorf("requests = %d, want the call counted either way", snapshot.RequestCount)
			}

		})
	}
}

func TestRecorderRejectsANilCollector(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("a nil collector was accepted; misassembly should fail at startup")
		}
	}()
	Recorder(nil)
}
