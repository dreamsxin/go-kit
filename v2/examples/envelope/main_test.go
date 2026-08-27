package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dreamsxin/go-kit/v2/kit"
	"github.com/dreamsxin/go-kit/v2/transport/http/server"
)

func newEnvelopeServer(t *testing.T) *httptest.Server {
	t.Helper()
	svc := kit.MustNewHTTP(":0",
		kit.WithJSONServerOptions(
			server.ServerResponseEncoder(encodeAPIResponse),
			server.ServerErrorEncoder(encodeAPIError),
		),
	)
	kit.HandleJSONTyped(svc, "/hello", hello)
	kit.HandleJSONTyped(svc, "/raw", hello,
		server.ServerResponseEncoder(server.EncodeJSONResponse),
	)
	return httptest.NewServer(svc)
}

func TestSuccessIsWrappedInEnvelope(t *testing.T) {
	srv := newEnvelopeServer(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/hello", "application/json",
		bytes.NewReader([]byte(`{"name":"world"}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	var body struct {
		Code    int                      `json:"code"`
		Message string                   `json:"message"`
		Data    struct{ Message string } `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 0 || body.Message != "ok" {
		t.Errorf("envelope: got %+v", body)
	}
	if body.Data.Message != "Hello, world!" {
		t.Errorf("data: got %+v", body.Data)
	}
}

func TestBusinessErrorKeepsEnvelopeAndStableCode(t *testing.T) {
	srv := newEnvelopeServer(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/hello", "application/json",
		bytes.NewReader([]byte(`{"name":""}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != http.StatusBadRequest || body.Message != "name is required" {
		t.Errorf("error envelope: got %+v", body)
	}
}

func TestRawRouteOptsOutOfEnvelope(t *testing.T) {
	srv := newEnvelopeServer(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/raw", "application/json",
		bytes.NewReader([]byte(`{"name":"world"}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var body HelloResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Message != "Hello, world!" {
		t.Errorf("raw response: got %+v", body)
	}
}

func TestBusinessLogicIsEnvelopeFree(t *testing.T) {
	// The handler itself stays transport-agnostic: it returns domain types
	// and classified errors only.
	ctx := t.Context()
	resp, err := hello(ctx, HelloRequest{Name: "kit"})
	if err != nil || resp.Message != "Hello, kit!" {
		t.Fatalf("hello: %v %v", resp, err)
	}
	if _, err := hello(ctx, HelloRequest{}); err == nil {
		t.Fatal("empty name should fail")
	}
}
