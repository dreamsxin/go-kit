package server_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// Test a custom binary format over HTTP: length-prefixed text, standing in for
// protobuf/MessagePack without external dependencies.
func unmarshalLenPrefix(body []byte) (any, error) {
	return strings.ToUpper(string(body)), nil
}

func marshalLenPrefix(resp any) ([]byte, error) {
	switch v := resp.(type) {
	case string:
		return []byte(v), nil
	case statusHeaderResponse:
		return []byte(v.Body), nil
	default:
		return nil, errors.New("unsupported response type")
	}
}

func TestRawBodyCodec_RoundTrip(t *testing.T) {
	decode, encode := server.RawBodyCodec(unmarshalLenPrefix, marshalLenPrefix, "application/x-custom")

	ep := func(_ context.Context, req any) (any, error) {
		return req.(string) + "!", nil
	}
	h := server.NewServer(ep, decode, encode)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/shout", strings.NewReader("hello")))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/x-custom" {
		t.Errorf("Content-Type: got %q", ct)
	}
	if body := rec.Body.String(); body != "HELLO!" {
		t.Errorf("body: got %q, want HELLO!", body)
	}
}

func TestRawBodyCodec_BodyLimit(t *testing.T) {
	decode, _ := server.RawBodyCodecWithMaxBytes(unmarshalLenPrefix, marshalLenPrefix, "application/x-custom", 4)

	req := httptest.NewRequest(http.MethodPost, "/shout", strings.NewReader("this is longer than four bytes"))
	_, err := decode(context.Background(), req)
	if err == nil {
		t.Fatal("expected body limit error")
	}
	if !errors.Is(err, server.ErrJSONBodyTooLarge) {
		t.Errorf("limit error: got %v", err)
	}
}

type statusHeaderResponse struct{ Body string }

func (r statusHeaderResponse) StatusCode() int { return http.StatusCreated }
func (r statusHeaderResponse) Headers() http.Header {
	return http.Header{"X-Custom": []string{"v"}}
}

func TestRawBodyCodec_PreservesStatusAndHeaders(t *testing.T) {
	_, encode := server.RawBodyCodec(unmarshalLenPrefix, marshalLenPrefix, "application/x-custom")

	rec := httptest.NewRecorder()
	if err := encode(context.Background(), rec, statusHeaderResponse{Body: "made"}); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("status: got %d, want 201", rec.Code)
	}
	if got := rec.Header().Get("X-Custom"); got != "v" {
		t.Errorf("header: got %q", got)
	}
	if rec.Body.String() != "made" {
		t.Errorf("body: got %q", rec.Body.String())
	}
}

func TestTextErrorEncoder_MatchesFormat(t *testing.T) {
	encoder := server.TextErrorEncoder("application/x-custom")

	rec := httptest.NewRecorder()
	err := apperror.New(apperror.KindNotFound, "todo.not_found", "todo not found")
	encoder(context.Background(), err, rec)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/x-custom" {
		t.Errorf("Content-Type: got %q", ct)
	}
	if body := rec.Body.String(); body != "todo not found" {
		t.Errorf("body: got %q", body)
	}

	// Unclassified 5xx stays opaque.
	rec2 := httptest.NewRecorder()
	encoder(context.Background(), errors.New("db connection refused"), rec2)
	if rec2.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", rec2.Code)
	}
	if strings.Contains(rec2.Body.String(), "connection") {
		t.Errorf("5xx body should be opaque, got %q", rec2.Body.String())
	}
}
