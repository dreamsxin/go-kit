package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/endpoint"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

type problemRetryAfterError struct {
	after time.Duration
}

func (problemRetryAfterError) Error() string { return "slow down" }

func (problemRetryAfterError) ErrorKindName() string {
	return string(apperror.KindResourceExhausted)
}

func (e problemRetryAfterError) RetryAfter() time.Duration { return e.after }

func encodeProblem(t *testing.T, err error, resolve httpserver.ProblemTypeResolver) (*http.Response, httpserver.ProblemDetails) {
	t.Helper()
	recorder := httptest.NewRecorder()
	httpserver.ProblemJSONErrorEncoder(resolve)(context.Background(), err, recorder)
	response := recorder.Result()
	var problem httpserver.ProblemDetails
	if decodeErr := json.NewDecoder(response.Body).Decode(&problem); decodeErr != nil {
		t.Fatalf("decode problem document: %v", decodeErr)
	}
	return response, problem
}

func TestProblemJSONErrorEncoderWritesTheRegisteredMediaType(t *testing.T) {
	response, _ := encodeProblem(t, apperror.NotFound("user.missing", "no such user"), nil)
	if got := response.Header.Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", got)
	}
}

func TestProblemFromErrorAlwaysHasTypeTitleAndStatus(t *testing.T) {
	problem := httpserver.ProblemFromError(context.Background(), apperror.NotFound("user.missing", "no such user"), nil)
	if problem.Type != httpserver.ProblemTypeAboutBlank {
		t.Fatalf("Type = %q, want %q", problem.Type, httpserver.ProblemTypeAboutBlank)
	}
	if problem.Title != "Not Found" {
		t.Fatalf("Title = %q, want Not Found", problem.Title)
	}
	if problem.Status != http.StatusNotFound {
		t.Fatalf("Status = %d, want %d", problem.Status, http.StatusNotFound)
	}
	if problem.Detail != "no such user" {
		t.Fatalf("Detail = %q, want the public message", problem.Detail)
	}
}

func TestProblemFromErrorUsesTheResolvedTypeURI(t *testing.T) {
	var seen string
	problem := httpserver.ProblemFromError(context.Background(), apperror.NotFound("user.missing", "no such user"), func(code string) string {
		seen = code
		return "https://errors.example.com/" + code
	})
	if seen != "user.missing" {
		t.Fatalf("resolver saw code %q, want user.missing", seen)
	}
	if problem.Type != "https://errors.example.com/user.missing" {
		t.Fatalf("Type = %q", problem.Type)
	}
}

func TestProblemFromErrorKeepsTheEnvelopeCode(t *testing.T) {
	err := apperror.NotFound("user.missing", "no such user")
	recorder := httptest.NewRecorder()
	httpserver.JSONErrorEncoder(context.Background(), err, recorder)
	var envelope httpserver.ErrorResponse
	if decodeErr := json.NewDecoder(recorder.Result().Body).Decode(&envelope); decodeErr != nil {
		t.Fatalf("decode envelope: %v", decodeErr)
	}

	problem := httpserver.ProblemFromError(context.Background(), err, nil)
	if problem.Code != envelope.Code {
		t.Fatalf("problem code %q, envelope code %q", problem.Code, envelope.Code)
	}
}

func TestProblemFromErrorRedactsAt500(t *testing.T) {
	problem := httpserver.ProblemFromError(context.Background(), apperror.Internal("boom", "connection string postgres://user:secret@db"), nil)
	if problem.Status != http.StatusInternalServerError {
		t.Fatalf("Status = %d, want 500", problem.Status)
	}
	if problem.Title != "Internal Server Error" {
		t.Fatalf("Title = %q", problem.Title)
	}
	if problem.Detail != "" {
		t.Fatalf("Detail = %q, want empty so the redacted title is not repeated", problem.Detail)
	}
}

func TestProblemFromErrorListsEveryInvalidField(t *testing.T) {
	err := &endpoint.ValidationError{Fields: []endpoint.FieldError{
		{Field: "email", Reason: "must be an address"},
		{Field: "age", Reason: "must be positive"},
	}}
	problem := httpserver.ProblemFromError(context.Background(), err, nil)
	if problem.Status != http.StatusBadRequest {
		t.Fatalf("Status = %d, want 400", problem.Status)
	}
	if len(problem.Errors) != 2 {
		t.Fatalf("Errors = %#v, want two entries", problem.Errors)
	}
	if problem.Errors[0].Field != "email" || problem.Errors[0].Detail != "must be an address" {
		t.Fatalf("first entry = %#v", problem.Errors[0])
	}
	if problem.Errors[1].Field != "age" || problem.Errors[1].Detail != "must be positive" {
		t.Fatalf("second entry = %#v", problem.Errors[1])
	}
}

func TestProblemJSONErrorEncoderMatchesTheEnvelopeStatusAndCode(t *testing.T) {
	for _, err := range []error{
		apperror.NotFound("user.missing", "no such user"),
		apperror.PermissionDenied("forbidden", "not yours"),
		&endpoint.ValidationError{Fields: []endpoint.FieldError{{Field: "email", Reason: "required"}}},
		apperror.Internal("boom", "secret"),
		context.DeadlineExceeded,
	} {
		envelopeRecorder := httptest.NewRecorder()
		httpserver.JSONErrorEncoder(context.Background(), err, envelopeRecorder)
		var envelope httpserver.ErrorResponse
		if decodeErr := json.NewDecoder(envelopeRecorder.Result().Body).Decode(&envelope); decodeErr != nil {
			t.Fatalf("decode envelope for %v: %v", err, decodeErr)
		}

		response, problem := encodeProblem(t, err, nil)
		if problem.Status != envelopeRecorder.Code || response.StatusCode != envelopeRecorder.Code {
			t.Fatalf("status for %v: problem %d/%d, envelope %d", err, problem.Status, response.StatusCode, envelopeRecorder.Code)
		}
		if problem.Code != envelope.Code {
			t.Fatalf("code for %v: problem %q, envelope %q", err, problem.Code, envelope.Code)
		}
	}
}

func TestProblemJSONErrorEncoderEmitsRetryAfter(t *testing.T) {
	response, problem := encodeProblem(t, problemRetryAfterError{after: 1500 * time.Millisecond}, nil)
	if got := response.Header.Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After = %q, want 2", got)
	}
	if problem.Status != http.StatusTooManyRequests {
		t.Fatalf("Status = %d, want 429", problem.Status)
	}
}

func TestWriteProblemJSONRejectsAnImpossibleStatus(t *testing.T) {
	recorder := httptest.NewRecorder()
	httpserver.WriteProblemJSON(recorder, httpserver.ProblemDetails{Status: 0, Title: "x", Type: "about:blank"})
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
}
