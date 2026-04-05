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

	"github.com/sony/gobreaker"
	"golang.org/x/time/rate"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/endpoint/circuitbreaker"
	"github.com/dreamsxin/go-kit/endpoint/ratelimit"
	"github.com/dreamsxin/go-kit/transport/http/server"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "hello-test",
		MaxRequests: 5,
		Interval:    10 * time.Second,
		Timeout:     5 * time.Second,
		ReadyToTrip: func(c gobreaker.Counts) bool { return c.ConsecutiveFailures > 3 },
	})
	limiter := rate.NewLimiter(rate.Every(time.Second), 100)

	var metrics endpoint.Metrics
	base := endpoint.Endpoint(func(ctx context.Context, req any) (any, error) {
		return helloLogic(ctx, req.(helloRequest))
	})
	ep := endpoint.NewBuilder(base).
		WithMetrics(&metrics).
		WithErrorHandling("hello").
		Use(endpoint.TimeoutMiddleware(5 * time.Second)).
		Use(circuitbreaker.Gobreaker(cb)).
		Use(ratelimit.NewErroringLimiter(limiter)).
		Build()

	mux := http.NewServeMux()
	mux.Handle("/hello", server.NewJSONServer[helloRequest](
		func(ctx context.Context, req helloRequest) (any, error) {
			return ep(ctx, req)
		},
	))
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"requests": metrics.RequestCount,
			"success":  metrics.SuccessCount,
			"errors":   metrics.ErrorCount,
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

func TestAvgMs_ZeroRequests(t *testing.T) {
	m := &endpoint.Metrics{}
	if got := avgMs(m); got != 0 {
		t.Errorf("avgMs with 0 requests: got %f, want 0", got)
	}
}

func TestAvgMs_WithRequests(t *testing.T) {
	m := &endpoint.Metrics{
		RequestCount:  2,
		TotalDuration: 200 * time.Millisecond,
	}
	if got := avgMs(m); got != 100.0 {
		t.Errorf("avgMs: got %f, want 100.0", got)
	}
}

func TestRateLimit_Rejected(t *testing.T) {
	// burst=1: second call is rejected
	limiter := rate.NewLimiter(0, 1)
	ep := ratelimit.NewErroringLimiter(limiter)(endpoint.Nop)

	ep(context.Background(), nil) //nolint:errcheck — consumes token
	_, err := ep(context.Background(), nil)
	if !errors.Is(err, ratelimit.ErrLimited) {
		t.Errorf("expected ErrLimited, got %v", err)
	}
}

func TestCircuitBreaker_Opens(t *testing.T) {
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "test",
		ReadyToTrip: func(c gobreaker.Counts) bool { return c.ConsecutiveFailures >= 2 },
	})
	alwaysFail := endpoint.Endpoint(func(_ context.Context, _ any) (any, error) {
		return nil, errors.New("fail")
	})
	ep := circuitbreaker.Gobreaker(cb)(alwaysFail)

	// trigger open
	for i := 0; i < 3; i++ {
		ep(context.Background(), nil) //nolint:errcheck
	}
	_, err := ep(context.Background(), nil)
	if err == nil {
		t.Error("expected circuit breaker open error")
	}
}
