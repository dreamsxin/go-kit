package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/transport/http/server"
)

func TestValidationErrorEncodesAs400(t *testing.T) {
	verr := endpoint.NewValidationError("name", "is required").
		Add("count", "must not be negative")

	rec := httptest.NewRecorder()
	server.JSONErrorEncoder(context.Background(), verr, rec)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", rec.Code)
	}

	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "bad_request.validation" {
		t.Errorf("code: got %q, want bad_request.validation", body.Code)
	}
	if body.Message != "invalid request: name: is required; count: must not be negative" {
		t.Errorf("message: got %q", body.Message)
	}
}

func TestWrappedValidationErrorEncodesAs400(t *testing.T) {
	wrapped := errors.Join(
		endpoint.NewValidationError("name", "is required"),
	)

	rec := httptest.NewRecorder()
	server.JSONErrorEncoder(context.Background(), wrapped, rec)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", rec.Code)
	}
}

func TestRejectionErrorsEncodeAs429(t *testing.T) {
	cases := []error{
		endpoint.ErrBackpressure,
		endpoint.ErrBulkheadFull,
	}
	for _, err := range cases {
		rec := httptest.NewRecorder()
		server.JSONErrorEncoder(context.Background(), err, rec)
		if rec.Code != http.StatusTooManyRequests {
			t.Errorf("%v: status %d, want 429", err, rec.Code)
		}
	}
}
