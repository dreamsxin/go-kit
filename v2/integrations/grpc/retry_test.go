package grpc

import (
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRetryable(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{err: status.Error(codes.Unavailable, "unavailable"), want: true},
		{err: status.Error(codes.ResourceExhausted, "busy"), want: true},
		{err: status.Error(codes.Aborted, "aborted"), want: true},
		{err: status.Error(codes.InvalidArgument, "invalid"), want: false},
		{err: errors.New("domain error"), want: false},
	}
	for _, test := range tests {
		if got := Retryable(test.err); got != test.want {
			t.Errorf("Retryable(%v) = %v, want %v", test.err, got, test.want)
		}
	}
}
