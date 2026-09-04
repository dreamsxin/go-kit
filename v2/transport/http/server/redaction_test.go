package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// secretCause stands in for anything an upstream failure drags along that a
// client must never read.
var secretCause = errors.New("dial tcp 10.0.0.7:5432: password=hunter2")

func encodedBody(t *testing.T, encoder server.ErrorEncoder, err error) (int, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	encoder(context.Background(), err, recorder)
	return recorder.Code, recorder.Body.String()
}

// WrapCause promises the cause stays internal. Below 500 the encoders used to
// fall back to err.Error(), which included it.
func TestEncodersDoNotLeakWrapCauseBelow500(t *testing.T) {
	err := apperror.WrapCause(apperror.KindNotFound, "user.not_found", secretCause)

	encoders := map[string]server.ErrorEncoder{
		"default": server.DefaultErrorEncoder,
		"json":    server.JSONErrorEncoder,
		"text":    server.TextErrorEncoder("text/plain; charset=utf-8"),
	}
	for name, encoder := range encoders {
		t.Run(name, func(t *testing.T) {
			status, body := encodedBody(t, encoder, err)
			if status != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", status)
			}
			if strings.Contains(body, "hunter2") || strings.Contains(body, "10.0.0.7") {
				t.Fatalf("body leaked the cause: %q", body)
			}
			if !strings.Contains(body, "Not Found") {
				t.Fatalf("body should fall back to the status text, got %q", body)
			}
		})
	}
}

// A non-empty public message is still forwarded: redaction must not swallow what
// the application chose to say.
func TestEncodersForwardPublicMessage(t *testing.T) {
	err := apperror.New(apperror.KindNotFound, "user.not_found", "user does not exist")

	_, body := encodedBody(t, server.JSONErrorEncoder, err)
	var payload server.ErrorResponse
	if decodeErr := json.Unmarshal([]byte(body), &payload); decodeErr != nil {
		t.Fatalf("body %q: %v", body, decodeErr)
	}
	if payload.Message != "user does not exist" {
		t.Fatalf("message = %q, want the public message", payload.Message)
	}
	if payload.Code != "user.not_found" {
		t.Fatalf("code = %q, want the application code", payload.Code)
	}
}

// An error outside the PublicMessager contract keeps the err.Error() fallback:
// a validation failure is written for the caller to read.
func TestEncodersKeepErrorTextForPlainClientErrors(t *testing.T) {
	err := &statusError{status: http.StatusBadRequest, message: "email is required"}

	_, body := encodedBody(t, server.DefaultErrorEncoder, err)
	if body != "email is required" {
		t.Fatalf("body = %q, want the error text", body)
	}
}

// TextErrorEncoder used to apply PublicMessager unconditionally, so a 500 could
// carry an application message the other encoders redacted.
func TestTextErrorEncoderRedactsAt500(t *testing.T) {
	err := apperror.New(apperror.KindInternal, "db.write", "shard 7 credentials rejected")

	status, body := encodedBody(t, server.TextErrorEncoder(""), err)
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", status)
	}
	if body != "Internal Server Error" {
		t.Fatalf("body = %q, want the redacted status text", body)
	}
}

// 499 is not a net/http status, so the encoder must use the package's own
// status text table rather than http.StatusText.
func TestTextErrorEncoderNamesClientClosedRequest(t *testing.T) {
	err := apperror.WrapCause(apperror.KindCanceled, "call.canceled", secretCause)

	status, body := encodedBody(t, server.TextErrorEncoder(""), err)
	if status != server.StatusClientClosedRequest {
		t.Fatalf("status = %d, want 499", status)
	}
	if body != "Client Closed Request" {
		t.Fatalf("body = %q, want the 499 status text", body)
	}
}

type statusError struct {
	status  int
	message string
}

func (e *statusError) Error() string   { return e.message }
func (e *statusError) StatusCode() int { return e.status }
