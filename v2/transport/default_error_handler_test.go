package transport_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/transport"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// TestUnhandledDecodeErrorIsLogged: a server constructed without an explicit
// error handler records the errors it answers — the default is no longer
// silence, because an operator cannot fix what no log ever mentioned.
func TestUnhandledDecodeErrorIsLogged(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(previous)

	handler := httpserver.NewJSONEndpoint[map[string]any](
		endpoint.TypedEndpoint[map[string]any, string](func(_ context.Context, _ map[string]any) (string, error) {
			return "ok", nil
		}).Wrap(),
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"broken`))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("malformed body = %d, want 400", recorder.Code)
	}

	logged := buf.String()
	if !strings.Contains(logged, "go-kit transport error") || !strings.Contains(logged, "WARN") {
		t.Errorf("the decode failure was not logged at warn level: %q", logged)
	}
}

// TestNopErrorHandlerSilences: silence is available, but it has to be asked
// for.
func TestNopErrorHandlerSilences(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(previous)

	handler := httpserver.NewJSONEndpoint[map[string]any](
		endpoint.TypedEndpoint[map[string]any, string](func(_ context.Context, _ map[string]any) (string, error) {
			return "", apperror.New(apperror.KindInvalidArgument, "test.rejected", "rejected")
		}).Wrap(),
		httpserver.ServerErrorHandler(transport.NopErrorHandler),
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("rejected request = %d, want 400", recorder.Code)
	}
	if buf.String() != "" {
		t.Errorf("an explicit NopErrorHandler still logged: %q", buf.String())
	}
}
