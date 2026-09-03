package server_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/transport/http/server"
)

type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, record.Clone())
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func (h *captureHandler) latestAttrs() map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.records) == 0 {
		return nil
	}
	attrs := make(map[string]any)
	h.records[len(h.records)-1].Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.Any()
		return true
	})
	return attrs
}

func TestAccessLogMiddleware_RecordsProtocolFacts(t *testing.T) {
	handler := &captureHandler{}
	logger := slog.New(handler)

	mw := server.AccessLogMiddleware(logger)
	served := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("short and stout"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/brew", nil)
	req.Header.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	served.ServeHTTP(httptest.NewRecorder(), req)

	attrs := handler.latestAttrs()
	if attrs == nil {
		t.Fatal("no access log record emitted")
	}
	if attrs["method"] != http.MethodPost || attrs["path"] != "/brew" {
		t.Errorf("method/path = %v/%v", attrs["method"], attrs["path"])
	}
	if attrs["status"] != int64(http.StatusTeapot) {
		t.Errorf("status = %v, want %d", attrs["status"], http.StatusTeapot)
	}
	if attrs["bytes"] != int64(len("short and stout")) {
		t.Errorf("bytes = %v, want %d", attrs["bytes"], len("short and stout"))
	}
	if attrs["trace_id"] != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("trace_id = %v", attrs["trace_id"])
	}
}

// The access line has to carry the same request ID the client got back, or
// correlating a client-reported failure with the server log means guessing.
// HTTP middleware runs outside the component's per-request context, so the ID
// is read from the response header the component already wrote.
func TestAccessLogMiddleware_RecordsRequestIDFromTheResponseHeader(t *testing.T) {
	handler := &captureHandler{}
	mw := server.AccessLogMiddleware(slog.New(handler))
	served := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Request-ID", "req-42")
		w.WriteHeader(http.StatusOK)
	}))

	served.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

	attrs := handler.latestAttrs()
	if attrs["request_id"] != "req-42" {
		t.Errorf("request_id = %v, want req-42", attrs["request_id"])
	}
}

// Installed inside a chain that already populated the context, the context wins.
func TestAccessLogMiddleware_PrefersTheContextRequestID(t *testing.T) {
	handler := &captureHandler{}
	mw := server.AccessLogMiddleware(slog.New(handler))
	served := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Request-ID", "from-header")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req = req.WithContext(endpoint.WithRequestID(req.Context(), "from-context"))
	served.ServeHTTP(httptest.NewRecorder(), req)

	attrs := handler.latestAttrs()
	if attrs["request_id"] != "from-context" {
		t.Errorf("request_id = %v, want from-context", attrs["request_id"])
	}
}

func TestAccessLogMiddleware_OmitsRequestIDWhenAbsent(t *testing.T) {
	handler := &captureHandler{}
	mw := server.AccessLogMiddleware(slog.New(handler))
	served := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	served.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

	if _, has := handler.latestAttrs()["request_id"]; has {
		t.Error("request_id should be absent when no ID was assigned")
	}
}

func TestAccessLogMiddleware_DefaultsMissingTraceparent(t *testing.T) {
	handler := &captureHandler{}
	mw := server.AccessLogMiddleware(slog.New(handler))
	served := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	served.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))

	attrs := handler.latestAttrs()
	if attrs == nil {
		t.Fatal("no access log record emitted")
	}
	if _, hasTrace := attrs["trace_id"]; hasTrace {
		t.Errorf("trace_id should be absent without traceparent, got %v", attrs["trace_id"])
	}
	if attrs["status"] != int64(http.StatusOK) {
		t.Errorf("status = %v, want %d", attrs["status"], http.StatusOK)
	}
}
