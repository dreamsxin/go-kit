package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/endpoint"
	httpclient "github.com/dreamsxin/go-kit/v2/transport/http/client"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

func TestHTTPStatusErrorClassifiesByStatus(t *testing.T) {
	tests := []struct {
		status int
		want   apperror.Kind
	}{
		{http.StatusBadRequest, apperror.KindInvalidArgument},
		{http.StatusUnauthorized, apperror.KindUnauthenticated},
		{http.StatusForbidden, apperror.KindPermissionDenied},
		{http.StatusNotFound, apperror.KindNotFound},
		{http.StatusConflict, apperror.KindConflict},
		{http.StatusPreconditionFailed, apperror.KindFailedPrecondition},
		{http.StatusTeapot, apperror.KindInvalidArgument},
		{http.StatusRequestTimeout, apperror.KindDeadlineExceeded},
		{http.StatusTooManyRequests, apperror.KindResourceExhausted},
		{499, apperror.KindCanceled},
		{http.StatusInternalServerError, apperror.KindInternal},
		{http.StatusNotImplemented, apperror.KindUnimplemented},
		{http.StatusBadGateway, apperror.KindUnavailable},
		{http.StatusServiceUnavailable, apperror.KindUnavailable},
		{http.StatusGatewayTimeout, apperror.KindDeadlineExceeded},
		{http.StatusInsufficientStorage, apperror.KindInternal},
	}
	for _, test := range tests {
		err := &httpclient.HTTPStatusError{StatusCode: test.status}
		if got := err.ErrorKind(); got != test.want {
			t.Errorf("status %d: ErrorKind = %q, want %q", test.status, got, test.want)
		}
		if got := err.ErrorKindName(); got != string(test.want) {
			t.Errorf("status %d: ErrorKindName = %q, want %q", test.status, got, test.want)
		}
		if got := httpclient.KindForStatus(test.status); got != test.want {
			t.Errorf("KindForStatus(%d) = %q, want %q", test.status, got, test.want)
		}
	}
}

// A classified client error keeps its meaning when the caller relays it, so a
// dependency's 503 is answered as 503 rather than degrading to 500.
func TestHTTPStatusErrorMapsBackToStatus(t *testing.T) {
	err := &httpclient.HTTPStatusError{StatusCode: http.StatusServiceUnavailable}
	if got := httpserver.HTTPStatusForError(err); got != http.StatusServiceUnavailable {
		t.Errorf("HTTPStatusForError = %d, want 503", got)
	}
}

// The stable code identifies the failure; the status code is only a coarse
// channel. Relaying an upstream error must therefore preserve the code as well
// as the status.
func TestClientErrorRelayPreservesCodeAndStatus(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpserver.JSONErrorEncoder(r.Context(), apperror.NotFound("user.missing", "no such user"), w)
	}))
	defer upstream.Close()

	call, err := httpclient.NewJSONClient[echoResp](http.MethodGet, upstream.URL)
	if err != nil {
		t.Fatalf("NewJSONClient: %v", err)
	}
	_, callErr := call(context.Background(), echoReq{})
	if callErr == nil {
		t.Fatal("expected an error")
	}

	var statusErr *httpclient.HTTPStatusError
	if !errors.As(callErr, &statusErr) {
		t.Fatalf("error is not *HTTPStatusError: %v", callErr)
	}
	if got := statusErr.ErrorCode(); got != "user.missing" {
		t.Errorf("ErrorCode = %q, want user.missing", got)
	}

	// Relaying the error unchanged reproduces both the status and the code.
	rec := httptest.NewRecorder()
	httpserver.JSONErrorEncoder(context.Background(), callErr, rec)
	if rec.Code != http.StatusNotFound {
		t.Errorf("relayed status = %d, want 404", rec.Code)
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode relayed body: %v", err)
	}
	if body.Code != "user.missing" {
		t.Errorf("relayed code = %q, want user.missing", body.Code)
	}
}

func TestHTTPStatusErrorCodeIgnoresNonJSONBodies(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"empty", ""},
		{"plain text", "Not Found"},
		{"json without code", `{"message":"nope"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := &httpclient.HTTPStatusError{StatusCode: http.StatusNotFound, Body: []byte(test.body)}
			if got := err.ErrorCode(); got != "" {
				t.Errorf("ErrorCode = %q, want empty", got)
			}
		})
	}
}

func TestHTTPStatusErrorRetryAfterHeader(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{"absent", "", 0},
		{"seconds", "3", 3 * time.Second},
		{"padded", " 12 ", 12 * time.Second},
		{"zero", "0", 0},
		{"negative", "-5", 0},
		{"malformed", "soon", 0},
		{"past date", "Mon, 02 Jan 2006 15:04:05 GMT", 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := &httpclient.HTTPStatusError{StatusCode: http.StatusTooManyRequests, Header: http.Header{}}
			if test.header != "" {
				err.Header.Set("Retry-After", test.header)
			}
			if got := err.RetryAfter(); got != test.want {
				t.Errorf("RetryAfter = %v, want %v", got, test.want)
			}
		})
	}
}

func TestHTTPStatusErrorRetryAfterHTTPDate(t *testing.T) {
	err := &httpclient.HTTPStatusError{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{"Retry-After": []string{time.Now().Add(time.Minute).UTC().Format(http.TimeFormat)}},
	}
	after := err.RetryAfter()
	if after <= 0 || after > time.Minute {
		t.Errorf("RetryAfter = %v, want a value within the next minute", after)
	}
}

// A client endpoint is an endpoint: the shared retry middleware must be able to
// retry a transient upstream failure with no custom classifier.
func TestClientEndpointRetriesTransientStatusWithSharedMiddleware(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"echo":"ok"}`))
	}))
	defer srv.Close()

	call, err := httpclient.NewJSONClient[echoResp](http.MethodPost, srv.URL)
	if err != nil {
		t.Fatalf("NewJSONClient: %v", err)
	}
	ep := endpoint.NewBuilder(call).WithRetry(3, endpoint.WithRetryBackoff(func(int) time.Duration { return 0 })).Build()

	resp, err := ep(context.Background(), echoReq{Message: "hi"})
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	if got := resp.(echoResp).Echo; got != "ok" {
		t.Errorf("echo = %q, want ok", got)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestClientEndpointDoesNotRetryClientError(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		http.Error(w, "nope", http.StatusBadRequest)
	}))
	defer srv.Close()

	call, err := httpclient.NewJSONClient[echoResp](http.MethodPost, srv.URL)
	if err != nil {
		t.Fatalf("NewJSONClient: %v", err)
	}
	ep := endpoint.NewBuilder(call).WithRetry(3, endpoint.WithRetryBackoff(func(int) time.Duration { return 0 })).Build()

	if _, err := ep(context.Background(), echoReq{Message: "hi"}); err == nil {
		t.Fatal("expected an error")
	} else {
		var statusErr *httpclient.HTTPStatusError
		if !errors.As(err, &statusErr) {
			t.Fatalf("error is not *HTTPStatusError: %v", err)
		}
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
}
