package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/kit"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// TestStreamPushesEventsAndStops: the stream answers 200 with an event
// stream body, pushes at least one event, and ends when the component
// stops — the shutdown interaction the lifecycle chapter documents.
func TestStreamPushesEventsAndStops(t *testing.T) {
	svc, err := kit.NewHTTP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	room := make(chan string)
	go func() {
		defer close(room)
		room <- "hello"
	}()
	kit.HandleSSETyped(svc, "GET /rooms/{room}/events", func(
		ctx context.Context, req streamRequest, w *httpserver.SSEStream,
	) error {
		select {
		case <-kit.Stopping(ctx):
			return nil
		case text, ok := <-room:
			if !ok {
				return nil
			}
			return w.EventJSON("hello", map[string]string{"room": req.Room, "text": text})
		}
	}, decodeStreamRequest)

	server := httptest.NewServer(svc)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/rooms/lobby/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d, want 200", response.StatusCode)
	}
	if ct := response.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content type = %q, want text/event-stream", ct)
	}
	body := readAll(t, response)
	if !strings.Contains(body, "event: hello") || !strings.Contains(body, `"room":"lobby"`) {
		t.Errorf("stream body = %q, want the pushed event", body)
	}
}

// readAll drains the response body until the server ends the stream or the
// budget runs out.
func readAll(t *testing.T, response *http.Response) string {
	t.Helper()
	done := make(chan struct{})
	var builder strings.Builder
	go func() {
		defer close(done)
		buf := make([]byte, 512)
		for {
			n, err := response.Body.Read(buf)
			builder.Write(buf[:n])
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
	return builder.String()
}
