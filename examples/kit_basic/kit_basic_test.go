package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/kit"
)

func newTestService(t *testing.T) *httptest.Server {
	t.Helper()
	svc := kit.New(":0",
		kit.WithRequestID(),
		kit.WithTimeout(5*time.Second),
	)
	svc.Handle("/greet", kit.JSON(greet))
	return httptest.NewServer(svc)
}

func TestGreet_Logic(t *testing.T) {
	resp, err := greet(context.Background(), GreetRequest{Name: "World"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gr := resp.(GreetResponse)
	if gr.Message != "Hello, World!" {
		t.Errorf("got %q, want %q", gr.Message, "Hello, World!")
	}
}

func TestGreet_EmptyName(t *testing.T) {
	_, err := greet(context.Background(), GreetRequest{})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestHTTP_Greet(t *testing.T) {
	srv := newTestService(t)
	defer srv.Close()

	body, _ := json.Marshal(GreetRequest{Name: "Kit"})
	resp, err := http.Post(srv.URL+"/greet", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result GreetResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Message != "Hello, Kit!" {
		t.Errorf("message: got %q, want %q", result.Message, "Hello, Kit!")
	}

	// Verify request ID header was injected by WithRequestID.
	if id := resp.Header.Get("X-Request-ID"); id == "" {
		t.Error("expected X-Request-ID header from WithRequestID option")
	}
}

func TestHTTP_Greet_EmptyName(t *testing.T) {
	srv := newTestService(t)
	defer srv.Close()

	body, _ := json.Marshal(GreetRequest{Name: ""})
	resp, err := http.Post(srv.URL+"/greet", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 400 {
		t.Errorf("expected 4xx for empty name, got %d", resp.StatusCode)
	}
}

func TestHTTP_Health(t *testing.T) {
	srv := newTestService(t)
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
