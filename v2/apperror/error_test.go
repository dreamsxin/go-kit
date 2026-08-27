package apperror_test

import (
	"errors"
	"testing"

	"github.com/dreamsxin/go-kit/v2/apperror"
)

func TestNew(t *testing.T) {
	err := apperror.New(apperror.KindInvalidArgument, "request.name_required", "name is required")
	if err.ErrorKind() != apperror.KindInvalidArgument {
		t.Fatalf("kind = %q", err.ErrorKind())
	}
	if err.ErrorCode() != "request.name_required" {
		t.Fatalf("code = %q", err.ErrorCode())
	}
	if err.PublicMessage() != "name is required" {
		t.Fatalf("message = %q", err.PublicMessage())
	}
	if err.ErrorKindName() != string(apperror.KindInvalidArgument) {
		t.Fatalf("kind name = %q", err.ErrorKindName())
	}
}

var _ apperror.KindNamer = (*apperror.Error)(nil)

func TestWrapPreservesCause(t *testing.T) {
	cause := errors.New("database unavailable")
	err := apperror.Wrap(apperror.KindUnavailable, "storage.unavailable", "try again later", cause)
	if !errors.Is(err, cause) {
		t.Fatal("wrapped error does not preserve cause")
	}
}

func TestEmptyKindDefaultsToInternal(t *testing.T) {
	err := apperror.New("", "internal", "")
	if err.ErrorKind() != apperror.KindInternal {
		t.Fatalf("kind = %q, want %q", err.ErrorKind(), apperror.KindInternal)
	}
}

func TestConvenienceConstructors(t *testing.T) {
	cases := []struct {
		err  *apperror.Error
		kind apperror.Kind
	}{
		{apperror.InvalidArgument("c", "m"), apperror.KindInvalidArgument},
		{apperror.Unauthenticated("c", "m"), apperror.KindUnauthenticated},
		{apperror.PermissionDenied("c", "m"), apperror.KindPermissionDenied},
		{apperror.NotFound("c", "m"), apperror.KindNotFound},
		{apperror.Conflict("c", "m"), apperror.KindConflict},
		{apperror.Unavailable("c", "m"), apperror.KindUnavailable},
	}
	for _, tc := range cases {
		if tc.err.ErrorKind() != tc.kind {
			t.Fatalf("kind = %q, want %q", tc.err.ErrorKind(), tc.kind)
		}
		if tc.err.ErrorCode() != "c" || tc.err.PublicMessage() != "m" {
			t.Fatalf("code/message = %q/%q", tc.err.ErrorCode(), tc.err.PublicMessage())
		}
	}
}

func TestWrapCauseKeepsCauseInternal(t *testing.T) {
	cause := errors.New("connection refused")
	err := apperror.WrapCause(apperror.KindUnavailable, "db.down", cause)
	if !errors.Is(err, cause) {
		t.Fatal("WrapCause does not preserve cause")
	}
	if err.PublicMessage() != "" {
		t.Fatalf("public message = %q, want empty", err.PublicMessage())
	}
	if err.Error() != "db.down: connection refused" {
		t.Fatalf("error string = %q", err.Error())
	}
}
