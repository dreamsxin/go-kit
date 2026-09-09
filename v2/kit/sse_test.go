package kit_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
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
	svc := kit.MustNewHTTP("127.0.0.1:0", kit.WithEndpointMiddleware(counter.middleware()))
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
	svc := kit.MustNewHTTP("127.0.0.1:0",
		kit.WithEndpointMiddleware(reject),
		kit.WithJSONServerOptions(httpserver.ServerErrorEncoder(httpserver.JSONErrorEncoder)),
	)
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

// TestHandleSSETyped_RejectionUsesTheComponentErrorEncoder pins the rejection
// path to the encoder the route itself answers with.
//
// The rejection used to be hardcoded to JSONErrorEncoder while the route's own
// decode failures went through whatever the options resolved to — so one route
// answered a 400 as text and a 401 as JSON, and a deployment that installed
// ProblemJSONErrorEncoder got problem+json for one and a bare envelope for the
// other. The component here installs a recognisable encoder and the test asserts
// both paths went through it.
func TestHandleSSETyped_RejectionUsesTheComponentErrorEncoder(t *testing.T) {
	marker := func(_ context.Context, err error, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/vnd.test+json")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte(`{"encoded_by":"component"}`))
	}
	reject := func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(_ context.Context, _ any) (any, error) {
			return nil, apperror.Unauthenticated("auth.required", "credentials required")
		}
	}
	svc := kit.MustNewHTTP("127.0.0.1:0",
		kit.WithEndpointMiddleware(reject),
		kit.WithJSONServerOptions(httpserver.ServerErrorEncoder(marker)),
	)
	kit.HandleSSETyped(svc, "GET /events",
		func(_ context.Context, _ eventsRequest, _ *httpserver.SSEStream) error { return nil },
		decodeEvents,
	)
	srv := httptest.NewServer(svc)
	defer srv.Close()

	// A rejection, and a decode failure on the same route: both must be rendered
	// by the component's encoder.
	for _, target := range []string{"/events?channel=builds", "/events"} {
		resp, err := http.Get(srv.URL + target)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if got := resp.Header.Get("Content-Type"); got != "application/vnd.test+json" {
			t.Fatalf("%s: Content-Type = %q, want the component encoder's", target, got)
		}
		if resp.StatusCode != http.StatusTeapot {
			t.Fatalf("%s: status = %d, want the component encoder's 418", target, resp.StatusCode)
		}
		if string(body) != `{"encoded_by":"component"}` {
			t.Fatalf("%s: body = %q", target, body)
		}
	}
}

// TestHandleSSETyped_StreamFailureReachesEndpointMiddleware asserts that a stream
// that fails partway is not reported as a completed request.
//
// The bridge endpoint used to return (struct{}{}, nil) unconditionally, so every
// stream — including one that died on its third event — recorded as a success in
// whatever middleware counts requests. The response cannot change after 200, so
// what is asserted is that the error reaches the middleware and that no error
// body is appended to the events already flushed.
func TestHandleSSETyped_StreamFailureReachesEndpointMiddleware(t *testing.T) {
	streamErr := apperror.Internal("stream.broken", "the source went away")
	observed := make(chan error, 1)
	watch := func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			response, err := next(ctx, request)
			observed <- err
			return response, err
		}
	}
	svc := kit.MustNewHTTP("127.0.0.1:0", kit.WithEndpointMiddleware(watch))
	kit.HandleSSETyped(svc, "GET /events",
		func(_ context.Context, _ eventsRequest, w *httpserver.SSEStream) error {
			if err := w.Event("tick", "1"); err != nil {
				return err
			}
			return streamErr
		},
		decodeEvents,
	)
	srv := httptest.NewServer(svc)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/events?channel=builds")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200 — the stream had already started", resp.StatusCode)
	}
	if string(body) != "event: tick\ndata: 1\n\n" {
		t.Fatalf("body = %q, want the flushed events with no error body appended", body)
	}
	select {
	case seen := <-observed:
		if seen == nil {
			t.Fatal("the middleware saw no error: a failed stream recorded as a success")
		}
		if !errors.Is(seen, streamErr) {
			t.Fatalf("middleware saw %v, want the stream's own error", seen)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the middleware never returned for the stream")
	}
}

func TestHandleSSETyped_DecodeFailureBeforeStream(t *testing.T) {
	svc := kit.MustNewHTTP("127.0.0.1:0")
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

func TestRawStreamHandlerBypassesEndpointMiddleware(t *testing.T) {
	counter := &countingMiddleware{}
	svc := kit.MustNewHTTP("127.0.0.1:0", kit.WithEndpointMiddleware(counter.middleware()))
	// Handle is the escape hatch, and it says so. There is deliberately no
	// HandleSSE alias for it: a name promising SSE behaviour while skipping the
	// chain is how a stream loses its middleware by accident.
	svc.Handle("GET /raw", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	svc := kit.MustNewHTTP("127.0.0.1:0")
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
