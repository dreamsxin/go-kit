package server

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/dreamsxin/go-kit/v2/apperror"
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

// apperror.Kinder is the typed classification contract, and an error that
// implements only it must map to the same code here as it does over HTTP.
func TestDefaultErrorEncoderReadsTheTypedKindContract(t *testing.T) {
	err := DefaultErrorEncoder(context.Background(), kinderError{kind: apperror.KindNotFound})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("not a status error: %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("code = %v, want NotFound", st.Code())
	}
}

type kinderError struct {
	kind apperror.Kind
}

func (kinderError) Error() string              { return "no such user" }
func (e kinderError) ErrorKind() apperror.Kind { return e.kind }

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

// Explicit classification wins over a context error in the chain, so one
// apperror maps to the same failure class over HTTP and gRPC. Checking the
// context first made a not_found wrapping context.Canceled arrive as 404 over
// HTTP and Canceled over gRPC, and dropped the stable application code.
func TestDefaultErrorEncoderPrefersTheKindOverAWrappedContextError(t *testing.T) {
	err := DefaultErrorEncoder(context.Background(), apperror.Wrap(
		apperror.KindNotFound, "user.missing", "no such user", context.Canceled,
	))

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("not a status error: %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Fatalf("code = %v, want NotFound: the error classified itself", st.Code())
	}
	statusErr, ok := transportgrpc.ClassifyError(err).(*transportgrpc.StatusError)
	if !ok {
		t.Fatalf("ClassifyError did not classify %v", err)
	}
	if got := statusErr.ErrorCode(); got != "user.missing" {
		t.Errorf("ErrorCode = %q, want user.missing", got)
	}
}

// A cancelled request must not rewrite a classified error either.
func TestDefaultErrorEncoderKeepsTheKindOnACancelledRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := DefaultErrorEncoder(ctx, codedError{kind: "not_found", code: "user.missing"})

	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", st.Code())
	}
}

// An unclassified error still falls back to the context status.
func TestDefaultErrorEncoderFallsBackToTheContextStatus(t *testing.T) {
	err := DefaultErrorEncoder(context.Background(), context.DeadlineExceeded)

	st, _ := status.FromError(err)
	if st.Code() != codes.DeadlineExceeded {
		t.Fatalf("code = %v, want DeadlineExceeded", st.Code())
	}
}

// codes.OK is the mapper's fallback sentinel, not a valid mapping: it is the
// zero value a forgetful switch returns, and status.New(codes.OK, ...).Err() is
// nil, so accepting it would answer a failed call with no error at all.
func TestErrorEncoderWithKindMapperTreatsOKAsNoOpinion(t *testing.T) {
	encoder := ErrorEncoderWithKindMapper(func(string) codes.Code { return codes.OK })

	err := encoder(context.Background(), codedError{kind: "not_found", code: "user.missing"})
	if err == nil {
		t.Fatal("a failure was encoded as success")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("not a status error: %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Fatalf("code = %v, want NotFound from the built-in mapping", st.Code())
	}
}

// codes.Unauthenticated is the highest code gRPC defines, so a mapper must be
// able to return it. Bounding the range at codes.DataLoss would have rejected
// the code the built-in mapping emits for an unauthenticated error.
func TestErrorEncoderWithKindMapperAcceptsTheHighestCode(t *testing.T) {
	encoder := ErrorEncoderWithKindMapper(func(kind string) codes.Code {
		if kind == "token_expired" {
			return codes.Unauthenticated
		}
		return codes.OK
	})

	err := encoder(context.Background(), codedError{kind: "token_expired", code: "auth.expired"})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", st.Code())
	}
}

// CodeForError is the gRPC counterpart of HTTPStatusForError: custom encoders
// reuse it instead of re-deriving the order and drifting apart from the built-in
// one, which is exactly how the context-before-kind bug arose.
func TestCodeForErrorFollowsTheEncoderOrder(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"nil", nil, codes.OK},
		{"classified", codedError{kind: "not_found"}, codes.NotFound},
		{
			"classification beats the wrapped context error",
			apperror.Wrap(apperror.KindNotFound, "user.missing", "no such user", context.Canceled),
			codes.NotFound,
		},
		{"unclassified context error", context.DeadlineExceeded, codes.DeadlineExceeded},
		{"unclassified", errPlain{}, codes.Internal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CodeForError(tc.err); got != tc.want {
				t.Fatalf("CodeForError = %v, want %v", got, tc.want)
			}
		})
	}
}
