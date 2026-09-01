package grpc

import (
	"errors"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestKindNameForCode(t *testing.T) {
	tests := []struct {
		code codes.Code
		want string
	}{
		{codes.InvalidArgument, "invalid_argument"},
		{codes.OutOfRange, "invalid_argument"},
		{codes.Unauthenticated, "unauthenticated"},
		{codes.PermissionDenied, "permission_denied"},
		{codes.NotFound, "not_found"},
		{codes.AlreadyExists, "already_exists"},
		{codes.Aborted, "conflict"},
		{codes.FailedPrecondition, "failed_precondition"},
		{codes.ResourceExhausted, "resource_exhausted"},
		{codes.Unavailable, "unavailable"},
		{codes.DeadlineExceeded, "deadline_exceeded"},
		{codes.Canceled, "canceled"},
		{codes.Unimplemented, "unimplemented"},
		{codes.Internal, "internal"},
		{codes.DataLoss, "internal"},
	}
	for _, test := range tests {
		if got := KindNameForCode(test.code); got != test.want {
			t.Errorf("KindNameForCode(%v) = %q, want %q", test.code, got, test.want)
		}
	}
}

func TestClassifyErrorKeepsGRPCStatus(t *testing.T) {
	err := ClassifyError(status.Error(codes.Unavailable, "backend down"))

	statusErr, ok := status.FromError(err)
	if !ok {
		t.Fatal("status.FromError lost the status")
	}
	if statusErr.Code() != codes.Unavailable {
		t.Errorf("code = %v, want Unavailable", statusErr.Code())
	}
	if got := status.Code(err); got != codes.Unavailable {
		t.Errorf("status.Code = %v, want Unavailable", got)
	}

	var namer interface{ ErrorKindName() string }
	if !errors.As(err, &namer) {
		t.Fatal("error does not report a kind name")
	}
	if got := namer.ErrorKindName(); got != "unavailable" {
		t.Errorf("kind name = %q, want unavailable", got)
	}

	var classified interface{ Retryable() bool }
	if !errors.As(err, &classified) || !classified.Retryable() {
		t.Error("an Unavailable status must classify itself as retryable")
	}
}

func TestClassifyErrorPassesThroughNonStatusErrors(t *testing.T) {
	if got := ClassifyError(nil); got != nil {
		t.Errorf("ClassifyError(nil) = %v, want nil", got)
	}
	plain := errors.New("not a status")
	if got := ClassifyError(plain); got != plain {
		t.Errorf("ClassifyError returned %v, want the original error", got)
	}
}

func TestClassifyErrorReadsRetryInfo(t *testing.T) {
	st, err := status.New(codes.ResourceExhausted, "slow down").WithDetails(&errdetails.RetryInfo{
		RetryDelay: durationpb.New(2 * time.Second),
	})
	if err != nil {
		t.Fatalf("WithDetails: %v", err)
	}

	var reporter interface{ RetryAfter() time.Duration }
	if !errors.As(ClassifyError(st.Err()), &reporter) {
		t.Fatal("error does not report a retry delay")
	}
	if got := reporter.RetryAfter(); got != 2*time.Second {
		t.Errorf("RetryAfter = %v, want 2s", got)
	}
}

func TestClassifyErrorWithoutRetryInfoReportsNoDelay(t *testing.T) {
	var reporter interface{ RetryAfter() time.Duration }
	if !errors.As(ClassifyError(status.Error(codes.Unavailable, "down")), &reporter) {
		t.Fatal("error does not report a retry delay")
	}
	if got := reporter.RetryAfter(); got != 0 {
		t.Errorf("RetryAfter = %v, want 0", got)
	}
}
