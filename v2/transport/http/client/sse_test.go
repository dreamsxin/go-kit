package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/transport/http/client"
)

type tickEvent struct {
	Room string `json:"room"`
	Seq  int    `json:"seq"`
}

// TestHandleSSEDecodesEvents: each framed event is delivered with its name
// and the JSON data decoded into T; the Accept header requests the stream
// media type.
func TestHandleSSEDecodesEvents(t *testing.T) {
	var gotAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		_, _ = w.Write([]byte("event: tick\ndata: {\"room\":\"lobby\",\"seq\":1}\n\n"))
		f.Flush()
		_, _ = w.Write([]byte("data: {\"room\":\"lobby\",\"seq\":2}\n\n")) // unnamed event
		f.Flush()
	}))
	defer server.Close()

	var got []client.SSEEvent[tickEvent]
	err := client.HandleSSE(context.Background(), server.URL+"/rooms/lobby/events",
		func(_ context.Context, event client.SSEEvent[tickEvent]) error {
			got = append(got, event)
			return nil
		})
	if err != nil {
		t.Fatalf("HandleSSE: %v", err)
	}
	if gotAccept != "text/event-stream" {
		t.Errorf("accept = %q, want text/event-stream", gotAccept)
	}
	if len(got) != 2 {
		t.Fatalf("events = %d, want 2", len(got))
	}
	if got[0].Name != "tick" || got[0].Data.Seq != 1 || got[0].Data.Room != "lobby" {
		t.Errorf("event[0] = %+v", got[0])
	}
	if got[1].Name != "" || got[1].Data.Seq != 2 {
		t.Errorf("event[1] = %+v", got[1])
	}
}

// TestHandleSSEEndsWhenServerCloses: the server ending the stream ends the
// call without error.
func TestHandleSSEEndsWhenServerCloses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {}\n\n"))
	}))
	defer server.Close()

	if err := client.HandleSSE[tickEvent](context.Background(), server.URL,
		func(context.Context, client.SSEEvent[tickEvent]) error { return nil }); err != nil {
		t.Fatalf("HandleSSE: %v", err)
	}
}

// TestHandleSSECancel: cancelling the context ends the call with the
// context's error.
func TestHandleSSECancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for {
			select {
			case <-r.Context().Done():
				return
			default:
			}
			_, _ = w.Write([]byte(": keepalive\n\n"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(50 * time.Millisecond)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := client.HandleSSE[tickEvent](ctx, server.URL,
		func(context.Context, client.SSEEvent[tickEvent]) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "context") {
		t.Errorf("error = %v, want a context cancellation", err)
	}
}

// TestHandleSSENon200: a non-2xx answer surfaces as the classified status
// error.
func TestHandleSSENon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	defer server.Close()

	err := client.HandleSSE[tickEvent](context.Background(), server.URL,
		func(context.Context, client.SSEEvent[tickEvent]) error { return nil })
	statusErr, ok := err.(*client.HTTPStatusError)
	if !ok {
		t.Fatalf("error = %v, want HTTPStatusError", err)
	}
	if statusErr.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", statusErr.StatusCode)
	}
}

// TestHandleSSEClientBeforeHeaders: ClientBefore flows through, which is how
// an authenticated stream carries its credentials.
func TestHandleSSEClientBeforeHeaders(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {}\n\n"))
	}))
	defer server.Close()

	err := client.HandleSSE[tickEvent](context.Background(), server.URL,
		func(context.Context, client.SSEEvent[tickEvent]) error { return nil },
		client.ClientBefore(func(ctx context.Context, r *http.Request) context.Context {
			r.Header.Set("Authorization", "Bearer stream-token")
			return ctx
		}))
	if err != nil {
		t.Fatalf("HandleSSE: %v", err)
	}
	if gotAuth != "Bearer stream-token" {
		t.Errorf("authorization = %q, want the token from ClientBefore", gotAuth)
	}
}

// TestHandleSSEConsumesCommentsAndRetry: comment lines and retry fields
// never reach onEvent, and a multi-line data field joins into one payload.
func TestHandleSSEConsumesCommentsAndRetry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(": keepalive\nretry: 100\ndata: line one\ndata: line two\n\n"))
	}))
	defer server.Close()

	var events []client.SSEEvent[string]
	err := client.HandleSSE[string](context.Background(), server.URL,
		func(_ context.Context, event client.SSEEvent[string]) error {
			events = append(events, event)
			return nil
		})
	if err != nil {
		t.Fatalf("HandleSSE: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1 (comments and retry consumed)", len(events))
	}
	if events[0].Data != "line one\nline two" {
		t.Errorf("data = %q, want the joined multi-line payload", events[0].Data)
	}
}
