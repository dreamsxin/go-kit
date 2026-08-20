package kit_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/kit"
	"github.com/dreamsxin/go-kit/v2/transport/http/server"
)

type apiEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func encodeAPIResponse(_ context.Context, w http.ResponseWriter, response any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(apiEnvelope{Code: 0, Message: "ok", Data: response})
}

func encodeAPIError(_ context.Context, err error, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(apiEnvelope{Code: 500, Message: "internal error"})
		return
	}
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(apiEnvelope{Code: 400, Message: appErr.PublicMessage()})
}

func TestWithJSONServerOptions_AppliesToAllRoutes(t *testing.T) {
	svc := kit.MustNew(":0",
		kit.WithJSONServerOptions(
			server.ServerResponseEncoder(encodeAPIResponse),
			server.ServerErrorEncoder(encodeAPIError),
		),
	)

	type greetRequest struct{ Name string }
	kit.HandleJSONTyped(svc, "/greet", func(_ context.Context, req greetRequest) (string, error) {
		return "Hello, " + req.Name, nil
	})
	type failRequest struct{}
	kit.HandleJSONTyped(svc, "/fail", func(_ context.Context, _ failRequest) (string, error) {
		return "", apperror.New(apperror.KindInvalidArgument, "demo.invalid", "name is required")
	})

	srv := httptest.NewServer(svc)
	defer srv.Close()

	// Success path is wrapped by the envelope on both routes.
	resp, err := http.Post(srv.URL+"/greet", "application/json", strings.NewReader(`{"Name":"kit"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body apiEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 0 || body.Message != "ok" || body.Data != "Hello, kit" {
		t.Errorf("envelope: got %+v", body)
	}

	// Error path uses the custom encoder.
	failResp, err := http.Post(srv.URL+"/fail", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer failResp.Body.Close()
	var failBody apiEnvelope
	if err := json.NewDecoder(failResp.Body).Decode(&failBody); err != nil {
		t.Fatal(err)
	}
	if failResp.StatusCode != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", failResp.StatusCode)
	}
	if failBody.Code != 400 || failBody.Message != "name is required" {
		t.Errorf("error envelope: got %+v", failBody)
	}
}

func TestWithJSONServerOptions_RouteOptionsTakePrecedence(t *testing.T) {
	svc := kit.MustNew(":0",
		kit.WithJSONServerOptions(
			server.ServerResponseEncoder(encodeAPIResponse),
		),
	)

	type pingRequest struct{}
	kit.HandleJSONTyped(svc, "/ping", func(_ context.Context, _ pingRequest) (string, error) {
		return "pong", nil
	})
	kit.HandleJSONTyped(svc, "/raw", func(_ context.Context, _ pingRequest) (string, error) {
		return "raw", nil
	},
		// A per-route option overrides the service-level encoder.
		server.ServerResponseEncoder(server.EncodeJSONResponse),
	)

	srv := httptest.NewServer(svc)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/ping", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var body apiEnvelope
	_ = json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	if body.Data != "pong" {
		t.Errorf("service-level envelope missing: %+v", body)
	}

	rawResp, err := http.Post(srv.URL+"/raw", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var raw string
	_ = json.NewDecoder(rawResp.Body).Decode(&raw)
	rawResp.Body.Close()
	if raw != "raw" {
		t.Errorf("route-level encoder should win, got %q", raw)
	}
}
