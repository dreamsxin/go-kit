package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dreamsxin/go-kit/v2/apperror"
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
		endpoint.ErrCircuitOpen,
		endpoint.ErrRateLimited,
	}
	for _, err := range cases {
		rec := httptest.NewRecorder()
		server.JSONErrorEncoder(context.Background(), err, rec)
		if rec.Code != http.StatusTooManyRequests {
			t.Errorf("%v: status %d, want 429", err, rec.Code)
		}
	}
}

func TestHTTPStatusForErrorReusesFrameworkMapping(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{apperror.New(apperror.KindNotFound, "x", "missing"), http.StatusNotFound},
		{apperror.New(apperror.KindInvalidArgument, "x", "bad"), http.StatusBadRequest},
		{endpoint.NewValidationError("name", "required"), http.StatusBadRequest},
		{endpoint.ErrRateLimited, http.StatusTooManyRequests},
		{endpoint.ErrCircuitOpen, http.StatusTooManyRequests},
		{errors.New("plain"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		if got := server.HTTPStatusForError(tc.err); got != tc.want {
			t.Errorf("%v: got %d, want %d", tc.err, got, tc.want)
		}
	}
}

func TestJSONErrorEncoderWithKindMapper(t *testing.T) {
	encoder := server.JSONErrorEncoderWithKindMapper(func(k apperror.Kind) int {
		if k == "payment_failed" {
			return http.StatusPaymentRequired
		}
		return 0
	})

	// Custom kind gets the custom status.
	rec := httptest.NewRecorder()
	err := apperror.New(apperror.Kind("payment_failed"), "payment.failed", "payment failed")
	encoder(context.Background(), err, rec)
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("status: got %d, want 402", rec.Code)
	}

	// Built-in kinds fall back to the default mapping.
	rec2 := httptest.NewRecorder()
	encoder(context.Background(), apperror.New(apperror.KindNotFound, "x", "missing"), rec2)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("built-in kind: got %d, want 404", rec2.Code)
	}

	// Unclassified errors stay 500 and opaque.
	rec3 := httptest.NewRecorder()
	encoder(context.Background(), errors.New("plain"), rec3)
	if rec3.Code != http.StatusInternalServerError {
		t.Fatalf("unclassified: got %d, want 500", rec3.Code)
	}
}

func TestJSONErrorEncoderWithKindMapper_NilMapper(t *testing.T) {
	encoder := server.JSONErrorEncoderWithKindMapper(nil)
	rec := httptest.NewRecorder()
	encoder(context.Background(), apperror.New(apperror.KindNotFound, "x", "missing"), rec)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("nil mapper should fall back, got %d", rec.Code)
	}
}
