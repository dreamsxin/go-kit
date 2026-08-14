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
}

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
