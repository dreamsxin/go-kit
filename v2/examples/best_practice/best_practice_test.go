package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/transport/http/server"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	breaker := endpoint.NewCircuitBreaker(
		endpoint.WithBreakerFailureThreshold(3),
		endpoint.WithBreakerOpenTimeout(5*time.Second),
	)
	limiter := newFixedRateLimiter(100)

	var metrics endpoint.Metrics
	ep := endpoint.NewTypedBuilder(endpoint.TypedEndpoint[helloRequest, helloResponse](helloLogic)).
		WithMetrics(&metrics).
		WithErrorHandling("hello").
		Use(endpoint.TimeoutMiddleware(5 * time.Second)).
		Use(breaker.Middleware()).
		Use(endpoint.RateLimitMiddleware(limiter)).
		Build()

	mux := http.NewServeMux()
	mux.Handle("/hello", server.NewTypedJSONServer(
		endpoint.Unwrap[helloRequest, helloResponse](ep),
	))
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		snapshot := metrics.Snapshot()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"requests": snapshot.RequestCount,
			"success":  snapshot.SuccessCount,
			"errors":   snapshot.ErrorCount,
		})
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	return httptest.NewServer(mux)
}

func TestHelloLogic_Success(t *testing.T) {
	resp, err := helloLogic(context.Background(), helloRequest{Name: "Alice"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message != "Hello, Alice!" {
		t.Errorf("got %q, want %q", resp.Message, "Hello, Alice!")
	}
}

func TestHelloLogic_EmptyName(t *testing.T) {
	_, err := helloLogic(context.Background(), helloRequest{})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestHTTP_HelloEndpoint_Success(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	body, _ := json.Marshal(helloRequest{Name: "Bob"})
	resp, err := http.Post(srv.URL+"/hello", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result helloResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Message != "Hello, Bob!" {
		t.Errorf("message: got %q, want %q", result.Message, "Hello, Bob!")
	}
}

func TestHTTP_HelloEndpoint_EmptyName(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	body, _ := json.Marshal(helloRequest{Name: ""})
	resp, err := http.Post(srv.URL+"/hello", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Error("expected non-200 for empty name")
	}
}

func TestHTTP_HealthEndpoint(t *testing.T) {
	srv := newTestServer(t)
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

func TestHTTP_MetricsEndpoint(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// make a successful request first
	body, _ := json.Marshal(helloRequest{Name: "Test"})
	http.Post(srv.URL+"/hello", "application/json", bytes.NewReader(body)) //nolint:errcheck

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}
	if _, ok := m["requests"]; !ok {
		t.Error("metrics response missing 'requests' field")
	}
}

func TestRateLimit_Rejected(t *testing.T) {
	// burst=1: second call is rejected
	limiter := newFixedRateLimiter(1)
	ep := endpoint.RateLimitMiddleware(limiter)(endpoint.Nop)

	ep(context.Background(), nil) //nolint:errcheck — consumes token
	_, err := ep(context.Background(), nil)
	if !errors.Is(err, endpoint.ErrRateLimited) {
		t.Errorf("expected ErrRateLimited, got %v", err)
	}
}

func TestCircuitBreaker_Opens(t *testing.T) {
	cb := endpoint.NewCircuitBreaker(endpoint.WithBreakerFailureThreshold(2))
	alwaysFail := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		return nil, errors.New("fail")
	})
	ep := cb.Middleware()(alwaysFail)

	// trigger open
	for i := 0; i < 3; i++ {
		ep(context.Background(), nil) //nolint:errcheck
	}
	_, err := ep(context.Background(), nil)
	if err == nil {
		t.Error("expected circuit breaker open error")
	}
}
