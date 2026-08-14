package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sony/gobreaker"
	"golang.org/x/time/rate"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/integrations/circuitbreaker"
	"github.com/dreamsxin/go-kit/v2/integrations/ratelimit"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

func newManualCompositionServer(t *testing.T) (*httptest.Server, *endpoint.Metrics) {
	t.Helper()
	var metrics endpoint.Metrics
	handler := httpserver.NewTypedJSONServerWithMiddleware(
		hello,
		func(b *endpoint.Builder) *endpoint.Builder {
			return b.
				WithMetrics(&metrics).
				WithErrorHandling("hello").
				WithTimeout(5 * time.Second).
				Use(circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(
					gobreaker.Settings{Name: "hello-test"},
				))).
				Use(ratelimit.NewErroringLimiter(
					rate.NewLimiter(rate.Every(time.Second), 100),
				))
		},
		httpserver.ServerErrorEncoder(httpserver.JSONErrorEncoder),
	)

	mux := http.NewServeMux()
	mux.Handle("/hello", handler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	return httptest.NewServer(mux), &metrics
}

func TestHello_Success(t *testing.T) {
	resp, err := hello(context.Background(), HelloRequest{Name: "World"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message != "Hello, World!" {
		t.Errorf("got %q, want %q", resp.Message, "Hello, World!")
	}
}

func TestHello_EmptyName(t *testing.T) {
	_, err := hello(context.Background(), HelloRequest{})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestHTTP_Hello_Success(t *testing.T) {
	srv, _ := newManualCompositionServer(t)
	defer srv.Close()

	body, _ := json.Marshal(HelloRequest{Name: "Manual"})
	resp, err := http.Post(srv.URL+"/hello", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result HelloResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Message != "Hello, Manual!" {
		t.Errorf("message: got %q, want %q", result.Message, "Hello, Manual!")
	}
}

func TestHTTP_Hello_EmptyName_Returns4xx(t *testing.T) {
	srv, _ := newManualCompositionServer(t)
	defer srv.Close()

	body, _ := json.Marshal(HelloRequest{Name: ""})
	resp, err := http.Post(srv.URL+"/hello", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	var result struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Code != "hello.name_required" || result.Message != "name is required" {
		t.Fatalf("error response = %#v", result)
	}
}

func TestHTTP_Health(t *testing.T) {
	srv, _ := newManualCompositionServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestHTTP_MetricsTracked(t *testing.T) {
	srv, metrics := newManualCompositionServer(t)
	defer srv.Close()

	for i := 0; i < 3; i++ {
		body, _ := json.Marshal(HelloRequest{Name: "test"})
		http.Post(srv.URL+"/hello", "application/json", bytes.NewReader(body)) //nolint:errcheck
	}

	snapshot := metrics.Snapshot()
	if snapshot.RequestCount != 3 {
		t.Errorf("RequestCount: got %d, want 3", snapshot.RequestCount)
	}
	if snapshot.SuccessCount != 3 {
		t.Errorf("SuccessCount: got %d, want 3", snapshot.SuccessCount)
	}
}

func TestHTTP_InvalidJSON(t *testing.T) {
	srv, _ := newManualCompositionServer(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/hello", "application/json", bytes.NewReader([]byte("not-json")))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 400 {
		t.Errorf("expected 4xx for invalid JSON, got %d", resp.StatusCode)
	}
}
