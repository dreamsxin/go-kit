package server

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	transportgrpc "github.com/dreamsxin/go-kit/v2/integrations/grpc"
)

// codedError is an application error the way apperror.Error presents itself to
// this package: a kind name, a stable code, and a public message.
type codedError struct {
	kind    string
	code    string
	message string
	after   time.Duration
}

func (e codedError) Error() string         { return e.message }
func (e codedError) ErrorKindName() string { return e.kind }
func (e codedError) ErrorCode() string     { return e.code }
func (e codedError) PublicMessage() string { return e.message }

func (e codedError) RetryAfter() time.Duration { return e.after }

// The stable application code must survive the hop: over HTTP it travels in the
// body, over gRPC in a google.rpc.ErrorInfo detail.
func TestDefaultErrorEncoderAttachesErrorInfo(t *testing.T) {
	err := DefaultErrorEncoder(context.Background(), codedError{
		kind:    "not_found",
		code:    "user.missing",
		message: "no such user",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("not a status error: %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("code = %v, want NotFound", st.Code())
	}
	if st.Message() != "no such user" {
		t.Errorf("message = %q, want the public message", st.Message())
	}

	statusErr, ok := transportgrpc.ClassifyError(err).(*transportgrpc.StatusError)
	if !ok {
		t.Fatalf("ClassifyError did not classify %v", err)
	}
	if got := statusErr.ErrorCode(); got != "user.missing" {
		t.Errorf("ErrorCode = %q, want user.missing", got)
	}
	if got := statusErr.ErrorDomain(); got != "" {
		t.Errorf("ErrorDomain = %q, want empty without WithErrorDomain", got)
	}
	if got := statusErr.ErrorKindName(); got != "not_found" {
		t.Errorf("ErrorKindName = %q, want not_found", got)
	}
}

func TestNewErrorEncoderReportsDomain(t *testing.T) {
	encoder := NewErrorEncoder(WithErrorDomain("users.example.com"))
	err := encoder(context.Background(), codedError{
		kind: "conflict",
		code: "user.duplicate",
	})

	statusErr, ok := transportgrpc.ClassifyError(err).(*transportgrpc.StatusError)
	if !ok {
		t.Fatalf("ClassifyError did not classify %v", err)
	}
	if got := statusErr.Code(); got != codes.Aborted {
		t.Errorf("code = %v, want Aborted", got)
	}
	if got := statusErr.ErrorCode(); got != "user.duplicate" {
		t.Errorf("ErrorCode = %q, want user.duplicate", got)
	}
	if got := statusErr.ErrorDomain(); got != "users.example.com" {
		t.Errorf("ErrorDomain = %q, want users.example.com", got)
	}
}

// Both details travel together: the code identifies the failure, the retry hint
// tells the caller when to come back.
func TestDefaultErrorEncoderAttachesCodeAndRetryInfo(t *testing.T) {
	err := DefaultErrorEncoder(context.Background(), codedError{
		kind:  "unavailable",
		code:  "storage.down",
		after: 3 * time.Second,
	})

	statusErr, ok := transportgrpc.ClassifyError(err).(*transportgrpc.StatusError)
	if !ok {
		t.Fatalf("ClassifyError did not classify %v", err)
	}
	if got := statusErr.ErrorCode(); got != "storage.down" {
		t.Errorf("ErrorCode = %q, want storage.down", got)
	}
	if got := statusErr.RetryAfter(); got != 3*time.Second {
		t.Errorf("RetryAfter = %v, want 3s", got)
	}
	if !statusErr.Retryable() {
		t.Error("an Unavailable status must be retryable")
	}
}

func TestDefaultErrorEncoderRedactsUnclassifiedErrors(t *testing.T) {
	err := DefaultErrorEncoder(context.Background(), errPlain{})

	st, _ := status.FromError(err)
	if st.Code() != codes.Internal {
		t.Errorf("code = %v, want Internal", st.Code())
	}
	if st.Message() != "internal error" {
		t.Errorf("message = %q, want the redacted message", st.Message())
	}
	if len(st.Details()) != 0 {
		t.Errorf("details = %v, want none", st.Details())
	}
}

type errPlain struct{}

func (errPlain) Error() string { return "database password is hunter2" }
