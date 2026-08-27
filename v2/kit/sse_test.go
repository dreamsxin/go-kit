package kit_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/kit"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

type eventsRequest struct {
	Channel string
}

func decodeEvents(r *http.Request) (eventsRequest, error) {
	channel := r.URL.Query().Get("channel")
	if channel == "" {
		return eventsRequest{}, apperror.InvalidArgument("stream.channel_required", "channel is required")
	}
	return eventsRequest{Channel: channel}, nil
}

type countingMiddleware struct {
	mu    sync.Mutex
	calls int
}

func (m *countingMiddleware) middleware() endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			m.mu.Lock()
			m.calls++
			m.mu.Unlock()
			return next(ctx, request)
		}
	}
}

func (m *countingMiddleware) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func TestHandleSSETyped_AppliesEndpointMiddleware(t *testing.T) {
	counter := &countingMiddleware{}
	svc := kit.MustNew("127.0.0.1:0", kit.WithEndpointMiddleware(counter.middleware()))
	kit.HandleSSETyped(svc, "GET /events",
		func(_ context.Context, req eventsRequest, w *httpserver.SSEStream) error {
			return w.Event("channel", req.Channel)
		},
		decodeEvents,
	)
	srv := httptest.NewServer(svc)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/events?channel=builds")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "event: channel\ndata: builds\n\n" {
		t.Fatalf("body = %q", body)
	}
	if counter.count() != 1 {
		t.Fatalf("middleware calls = %d, want one per stream", counter.count())
	}
}

func TestHandleSSETyped_MiddlewareRejectionBeforeStream(t *testing.T) {
	streamCalled := false
	reject := func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(_ context.Context, _ any) (any, error) {
			return nil, apperror.Unauthenticated("auth.required", "credentials required")
		}
	}
	svc := kit.MustNew("127.0.0.1:0", kit.WithEndpointMiddleware(reject))
	kit.HandleSSETyped(svc, "GET /events",
		func(_ context.Context, _ eventsRequest, _ *httpserver.SSEStream) error {
			streamCalled = true
			return nil
		},
		decodeEvents,
	)
	srv := httptest.NewServer(svc)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/events?channel=builds")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("error body is not JSON: %v", err)
	}
	if body["code"] != "auth.required" {
		t.Fatalf("code = %q", body["code"])
	}
	if streamCalled {
		t.Fatal("rejected request still started the stream")
	}
}

func TestHandleSSETyped_DecodeFailureBeforeStream(t *testing.T) {
	svc := kit.MustNew("127.0.0.1:0")
	kit.HandleSSETyped(svc, "GET /events",
		func(_ context.Context, _ eventsRequest, _ *httpserver.SSEStream) error { return nil },
		decodeEvents,
		httpserver.ServerErrorEncoder(httpserver.JSONErrorEncoder),
	)
	srv := httptest.NewServer(svc)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("error body is not JSON: %v", err)
	}
	if body["code"] != "stream.channel_required" {
		t.Fatalf("code = %q", body["code"])
	}
}

func TestHandleSSE_RawBypassesEndpointMiddleware(t *testing.T) {
	counter := &countingMiddleware{}
	svc := kit.MustNew("127.0.0.1:0", kit.WithEndpointMiddleware(counter.middleware()))
	kit.HandleSSE(svc, "GET /raw", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("raw"))
	}))
	srv := httptest.NewServer(svc)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/raw")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if counter.count() != 0 {
		t.Fatalf("middleware calls = %d, want none for raw streams", counter.count())
	}
}

func TestHandleSSETyped_ClientDisconnectCancelsStream(t *testing.T) {
	done := make(chan struct{})
	svc := kit.MustNew("127.0.0.1:0")
	kit.HandleSSETyped(svc, "GET /events",
		func(ctx context.Context, _ eventsRequest, w *httpserver.SSEStream) error {
			defer close(done)
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return nil
				case <-ticker.C:
					if err := w.Data("tick"); err != nil {
						return err
					}
				}
			}
		},
		decodeEvents,
	)
	srv := httptest.NewServer(svc)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/events?channel=builds")
	if err != nil {
		t.Fatal(err)
	}

	// Read one event, then drop the connection.
	reader := bufio.NewReader(resp.Body)
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("read first line: %v", err)
	}
	_ = resp.Body.Close()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not observe client disconnect")
	}
}
