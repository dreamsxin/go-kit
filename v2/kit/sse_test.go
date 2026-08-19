package kit_test

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/kit"
)

func newSSEServer(t *testing.T, stream func(ctx context.Context, w *kit.SSEWriter) error) *httptest.Server {
	t.Helper()
	svc := kit.MustNew(":0")
	kit.HandleSSE(svc, "GET /events", stream)
	return httptest.NewServer(svc)
}

func TestHandleSSE_WritesEventsAndHeaders(t *testing.T) {
	srv := newSSEServer(t, func(_ context.Context, w *kit.SSEWriter) error {
		if err := w.Event("greeting", "hello"); err != nil {
			return err
		}
		return w.EventJSON("progress", map[string]int{"step": 1})
	})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type: got %q", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control: got %q", cc)
	}

	want := "event: greeting\ndata: hello\n\n" +
		"event: progress\ndata: {\"step\":1}\n\n"
	body, _ := io.ReadAll(resp.Body)
	if string(body) != want {
		t.Errorf("body:\n got %q\nwant %q", body, want)
	}
}

func TestHandleSSE_MultiLineDataSplitsIntoDataLines(t *testing.T) {
	srv := newSSEServer(t, func(_ context.Context, w *kit.SSEWriter) error {
		return w.Data("first\nsecond")
	})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	want := "data: first\ndata: second\n\n"
	body, _ := io.ReadAll(resp.Body)
	if string(body) != want {
		t.Errorf("body:\n got %q\nwant %q", body, want)
	}
}

func TestHandleSSE_CommentAndRetryLines(t *testing.T) {
	srv := newSSEServer(t, func(_ context.Context, w *kit.SSEWriter) error {
		if err := w.Comment("keep-alive"); err != nil {
			return err
		}
		return w.Retry(1500)
	})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	want := ": keep-alive\nretry: 1500\n"
	body, _ := io.ReadAll(resp.Body)
	if string(body) != want {
		t.Errorf("body:\n got %q\nwant %q", body, want)
	}
}

func TestHandleSSE_ClientDisconnectCancelsStream(t *testing.T) {
	done := make(chan struct{})
	srv := newSSEServer(t, func(ctx context.Context, w *kit.SSEWriter) error {
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
	})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/events")
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
