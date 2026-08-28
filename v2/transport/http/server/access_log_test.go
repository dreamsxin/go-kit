package server_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

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
