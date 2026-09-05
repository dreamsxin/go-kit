package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/transport/http/server"
)

type mediaTypeRequest struct {
	Name string `json:"name"`
}

func mediaTypeHandler() http.Handler {
	return server.NewTypedJSONServer(func(_ context.Context, req mediaTypeRequest) (mediaTypeRequest, error) {
		return req, nil
	})
}

// TestUnsupportedMediaTypeIsAnswered415 covers the protocol question the server
// used to answer with a decode result: a caller naming a type the route does not
// speak was accepted whenever its bytes happened to parse.
func TestUnsupportedMediaTypeIsAnswered415(t *testing.T) {
	handler := mediaTypeHandler()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"alice"}`))
	request.Header.Set("Content-Type", "text/plain")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnsupportedMediaType)
	}
	if code := errorCodeOf(t, recorder); code != "unsupported_media_type" {
		t.Fatalf("code = %q, want %q", code, "unsupported_media_type")
	}
}

func TestJSONMediaTypesAreAccepted(t *testing.T) {
	accepted := []string{
		"application/json",
		"application/json; charset=utf-8",
		"application/json;charset=UTF-8",
		"application/merge-patch+json",
		"", // a request that names no type is not a request that named a wrong one
	}
	for _, contentType := range accepted {
		handler := mediaTypeHandler()
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"alice"}`))
		if contentType != "" {
			request.Header.Set("Content-Type", contentType)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Errorf("Content-Type %q: status = %d, want %d", contentType, recorder.Code, http.StatusOK)
		}
	}
}

// TestNonUTF8CharsetIsRejected: JSON is UTF-8 by RFC 8259, so a caller
// declaring another encoding is describing bytes this decoder will not read.
func TestNonUTF8CharsetIsRejected(t *testing.T) {
	handler := mediaTypeHandler()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"alice"}`))
	request.Header.Set("Content-Type", "application/json; charset=iso-8859-1")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnsupportedMediaType)
	}
}

// TestEmptyBodyReportsItself replaces the message a caller used to receive for
// this, which was the literal string "EOF" — the decoder's situation, not the
// caller's mistake.
func TestEmptyBodyReportsItself(t *testing.T) {
	handler := mediaTypeHandler()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	body := decodeErrorBody(t, recorder)
	if body.Code != "bad_request.empty_body" {
		t.Errorf("code = %q, want %q", body.Code, "bad_request.empty_body")
	}
	if body.Message == "EOF" || body.Message == "" {
		t.Errorf("message = %q, want it to describe the caller's mistake", body.Message)
	}
}

func TestMalformedBodyStillReportsInvalidJSON(t *testing.T) {
	handler := mediaTypeHandler()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if code := errorCodeOf(t, recorder); code != "bad_request.invalid_json" {
		t.Fatalf("code = %q, want %q", code, "bad_request.invalid_json")
	}
}

type errorEnvelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func decodeErrorBody(t *testing.T, recorder *httptest.ResponseRecorder) errorEnvelope {
	t.Helper()
	var body errorEnvelope
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	return body
}

func errorCodeOf(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	return decodeErrorBody(t, recorder).Code
}
