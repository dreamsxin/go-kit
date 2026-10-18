package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/transport/http/server"
)

type envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func envelopeEncoder(_ context.Context, w http.ResponseWriter, response any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(envelope{Code: 0, Message: "ok", Data: response})
}

func TestServerResponseEncoder_AssemblesEnvelope(t *testing.T) {
	h := server.NewTypedJSON(
		func(_ context.Context, req struct{ Name string }) (struct{ Greeting string }, error) {
			return struct{ Greeting string }{"Hello, " + req.Name}, nil
		},
		server.DefaultJSONDecodeOptions(),
		server.ServerResponseEncoder(envelopeEncoder),
	)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/greet", strings.NewReader(`{"Name":"kit"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	var body envelope
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 0 || body.Message != "ok" {
		t.Errorf("envelope: got %+v", body)
	}
	data, _ := body.Data.(map[string]any)
	if data["Greeting"] != "Hello, kit" {
		t.Errorf("data: got %v", body.Data)
	}
}

// TestServerResponseEncoder_OverridesConstructorArgument pins the ordering the
// generated transports rely on: options passed to NewServer apply after the
// constructor's encode argument, so a user-owned customServerOptions seam can
// replace the default encoder on a server built with NewServer.
func TestServerResponseEncoder_OverridesConstructorArgument(t *testing.T) {
	h := server.NewServer(
		func(context.Context, any) (any, error) { return "raw", nil },
		func(_ context.Context, _ *http.Request) (any, error) { return struct{}{}, nil },
		server.EncodeJSONResponse,
		server.ServerResponseEncoder(envelopeEncoder),
	)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`)))

	var body envelope
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Message != "ok" || body.Data != "raw" {
		t.Errorf("envelope option must win over the constructor argument: got %+v", body)
	}
}

func TestServerResponseEncoder_ErrorPathUnchanged(t *testing.T) {
	h := server.NewTypedJSON(
		func(_ context.Context, _ struct{}) (struct{}, error) {
			return struct{}{}, apperror.New(apperror.KindNotFound, "demo.missing", "not found")
		},
		server.DefaultJSONDecodeOptions(),
		server.ServerResponseEncoder(envelopeEncoder),
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/demo", strings.NewReader(`{}`)))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", rec.Code)
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "demo.missing" {
		t.Errorf("error code: got %q", body.Code)
	}
}

func TestServerResponseEncoder_NilOptionIsIgnored(t *testing.T) {
	h := server.NewTypedJSON(
		func(_ context.Context, _ struct{ Name string }) (struct{ Greeting string }, error) {
			return struct{ Greeting string }{"hi"}, nil
		},
		server.DefaultJSONDecodeOptions(),
		server.ServerResponseEncoder(nil),
	)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/greet", strings.NewReader(`{"Name":"x"}`)))

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if _, wrapped := body["data"]; wrapped {
		t.Errorf("nil encoder must keep the default encoding, got %v", body)
	}
}
