package server_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/transport"
	"github.com/dreamsxin/go-kit/v2/transport/http/server"
)

func newSSETestServer(t *testing.T, stream server.SSEStreamHandler, opts ...server.ServerOption) *httptest.Server {
	t.Helper()
	sse := server.NewSSEServer(stream, server.NopRequestDecoder, opts...)
	srv := httptest.NewServer(sse)
	t.Cleanup(srv.Close)
	return srv
}

func TestSSEServer_WritesEventsAndHeaders(t *testing.T) {
	srv := newSSETestServer(t, func(_ context.Context, _ any, s *server.SSEStream) error {
		if err := s.Event("greeting", "hello"); err != nil {
			return err
		}
		return s.EventJSON("progress", map[string]int{"step": 1})
	})

	resp, err := http.Get(srv.URL)
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
	if ab := resp.Header.Get("X-Accel-Buffering"); ab != "no" {
		t.Errorf("X-Accel-Buffering: got %q", ab)
	}

	want := "event: greeting\ndata: hello\n\n" +
		"event: progress\ndata: {\"step\":1}\n\n"
	body, _ := io.ReadAll(resp.Body)
	if string(body) != want {
		t.Errorf("body:\n got %q\nwant %q", body, want)
	}
}

func TestSSEServer_MultiLineDataSplitsIntoDataLines(t *testing.T) {
	srv := newSSETestServer(t, func(_ context.Context, _ any, s *server.SSEStream) error {
		return s.Data("first\nsecond")
	})

	resp, err := http.Get(srv.URL)
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

func TestSSEServer_CommentAndRetryLines(t *testing.T) {
	srv := newSSETestServer(t, func(_ context.Context, _ any, s *server.SSEStream) error {
		if err := s.Comment("keep-alive"); err != nil {
			return err
		}
		return s.Retry(1500)
	})

	resp, err := http.Get(srv.URL)
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

func TestSSEServer_DecodeFailureBeforeHeaders(t *testing.T) {
	dec := func(context.Context, *http.Request) (any, error) {
		return nil, apperror.InvalidArgument("stream.bad_cursor", "cursor is required")
	}
	sse := server.NewSSEServer(
		func(_ context.Context, _ any, _ *server.SSEStream) error { return nil },
		dec,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)
	srv := httptest.NewServer(sse)
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type: got %q, want JSON error response", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "stream.bad_cursor") {
		t.Fatalf("body = %q, want error code", body)
	}
}

// plainWriter is a ResponseWriter without http.Flusher support.
type plainWriter struct {
	headers http.Header
	code    int
	body    bytes.Buffer
}

func newPlainWriter() *plainWriter {
	return &plainWriter{headers: http.Header{}, code: http.StatusOK}
}

func (w *plainWriter) Header() http.Header         { return w.headers }
func (w *plainWriter) Write(p []byte) (int, error) { return w.body.Write(p) }
func (w *plainWriter) WriteHeader(code int)        { w.code = code }

func TestSSEServer_FlusherUnsupported(t *testing.T) {
	sse := server.NewSSEServer(
		func(_ context.Context, _ any, _ *server.SSEStream) error { return nil },
		server.NopRequestDecoder,
	)
	recorder := newPlainWriter()
	sse.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events", nil))

	if recorder.code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", recorder.code)
	}
}

func TestSSEServer_HooksRunAroundStream(t *testing.T) {
	type markerKey struct{}
	var order []string

	sse := server.NewSSEServer(
		func(ctx context.Context, _ any, s *server.SSEStream) error {
			if ctx.Value(markerKey{}) == nil {
				t.Error("before hook did not enrich context")
			}
			order = append(order, "stream")
			return errors.New("stream aborted")
		},
		server.NopRequestDecoder,
		server.ServerBefore(func(ctx context.Context, _ *http.Request) context.Context {
			order = append(order, "before")
			return context.WithValue(ctx, markerKey{}, true)
		}),
		server.ServerErrorHandler(transport.ErrorHandlerFunc(func(_ context.Context, err error) {
			order = append(order, "error:"+err.Error())
		})),
		server.ServerFinalizer(func(context.Context, *http.Request, *server.InterceptingWriter) {
			order = append(order, "finalizer")
		}),
	)

	recorder := httptest.NewRecorder()
	sse.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events", nil))

	want := []string{"before", "stream", "error:stream aborted", "finalizer"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("hook order = %v, want %v", order, want)
	}
}

func TestSSEServer_TypedAssertsRequestType(t *testing.T) {
	type cursorRequest struct{ Cursor string }
	sse := server.NewSSEServerTyped(
		func(_ context.Context, req cursorRequest, s *server.SSEStream) error {
			return s.Data(req.Cursor)
		},
		func(r *http.Request) (cursorRequest, error) {
			return cursorRequest{Cursor: r.URL.Query().Get("cursor")}, nil
		},
	)
	srv := httptest.NewServer(sse)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/?cursor=abc")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "data: abc\n\n" {
		t.Fatalf("body = %q", body)
	}
}

func TestSSEServer_ClientDisconnectCancelsStream(t *testing.T) {
	done := make(chan struct{})
	srv := newSSETestServer(t, func(ctx context.Context, _ any, s *server.SSEStream) error {
		defer close(done)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				if err := s.Data("tick"); err != nil {
					return err
				}
			}
		}
	})

	resp, err := http.Get(srv.URL)
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
