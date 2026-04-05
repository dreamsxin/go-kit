package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dreamsxin/go-kit/transport"
	"github.com/dreamsxin/go-kit/transport/http/interfaces"
	"github.com/dreamsxin/go-kit/transport/http/server"
)

// ── DecodeJSONRequest ─────────────────────────────────────────────────────────

type testReq struct {
	Name string `json:"name"`
}

func TestDecodeJSONRequest_Valid(t *testing.T) {
	body, _ := json.Marshal(testReq{Name: "Alice"})
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	dec := server.DecodeJSONRequest[testReq]()
	got, err := dec(context.Background(), r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.(testReq).Name != "Alice" {
		t.Errorf("Name: got %q, want %q", got.(testReq).Name, "Alice")
	}
}

func TestDecodeJSONRequest_Invalid(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("not-json")))
	dec := server.DecodeJSONRequest[testReq]()
	_, err := dec(context.Background(), r)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestNopRequestDecoder(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	got, err := server.NopRequestDecoder(context.Background(), r)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// ── EncodeJSONResponse ────────────────────────────────────────────────────────

func TestEncodeJSONResponse_Basic(t *testing.T) {
	w := httptest.NewRecorder()
	err := server.EncodeJSONResponse(context.Background(), w, map[string]string{"key": "val"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type: got %q", ct)
	}
}

type customStatusResp struct{ code int }

func (r customStatusResp) StatusCode() int { return r.code }

func TestEncodeJSONResponse_StatusCoder(t *testing.T) {
	w := httptest.NewRecorder()
	server.EncodeJSONResponse(context.Background(), w, customStatusResp{code: http.StatusCreated}) //nolint:errcheck
	if w.Code != http.StatusCreated {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusCreated)
	}
}

type headerResp struct{}

func (r headerResp) Headers() http.Header {
	h := http.Header{}
	h.Set("X-Custom", "custom-value")
	return h
}

func TestEncodeJSONResponse_Headerer(t *testing.T) {
	w := httptest.NewRecorder()
	server.EncodeJSONResponse(context.Background(), w, headerResp{}) //nolint:errcheck
	if got := w.Header().Get("X-Custom"); got != "custom-value" {
		t.Errorf("X-Custom: got %q, want %q", got, "custom-value")
	}
}

func TestNopResponseEncoder(t *testing.T) {
	w := httptest.NewRecorder()
	err := server.NopResponseEncoder(context.Background(), w, "anything")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if w.Body.Len() != 0 {
		t.Error("expected empty body")
	}
}

// ── JSONErrorEncoder ──────────────────────────────────────────────────────────

func TestJSONErrorEncoder_Default500(t *testing.T) {
	w := httptest.NewRecorder()
	server.JSONErrorEncoder(context.Background(), errors.New("oops"), w)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusInternalServerError)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type: got %q", ct)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body) //nolint:errcheck
	if body["error"] != "oops" {
		t.Errorf("error body: got %q, want %q", body["error"], "oops")
	}
}

type extraStatusErr struct{ code int }

func (e extraStatusErr) Error() string   { return "status error" }
func (e extraStatusErr) StatusCode() int { return e.code }

func TestJSONErrorEncoder_StatusCoder(t *testing.T) {
	w := httptest.NewRecorder()
	server.JSONErrorEncoder(context.Background(), extraStatusErr{code: http.StatusNotFound}, w)
	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusNotFound)
	}
}

// ── InterceptingWriter ────────────────────────────────────────────────────────

func TestInterceptingWriter_CapturesCode(t *testing.T) {
	w := httptest.NewRecorder()
	iw := &server.InterceptingWriter{}
	// Use via server finalizer to test indirectly
	called := false
	srv := server.NewServer(
		func(_ context.Context, _ any) (any, error) { return "ok", nil },
		server.NopRequestDecoder,
		server.EncodeJSONResponse,
		server.ServerFinalizer(func(_ context.Context, _ *http.Request, iw *server.InterceptingWriter) {
			called = true
			if iw.GetCode() != http.StatusOK {
				t.Errorf("code: got %d, want %d", iw.GetCode(), http.StatusOK)
			}
		}),
	)
	_ = iw
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	srv.ServeHTTP(w, r)
	if !called {
		t.Error("finalizer was not called")
	}
}

// ── NewJSONServer ─────────────────────────────────────────────────────────────

func TestNewJSONServer_Success(t *testing.T) {
	h := server.NewJSONServer[testReq](func(_ context.Context, req testReq) (any, error) {
		return map[string]string{"echo": req.Name}, nil
	})
	body, _ := json.Marshal(testReq{Name: "test"})
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if resp["echo"] != "test" {
		t.Errorf("echo: got %q, want %q", resp["echo"], "test")
	}
}

func TestNewJSONServer_HandlerError(t *testing.T) {
	h := server.NewJSONServer[testReq](func(_ context.Context, _ testReq) (any, error) {
		return nil, errors.New("handler error")
	})
	body, _ := json.Marshal(testReq{Name: "x"})
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ── ServerBefore / ServerAfter ────────────────────────────────────────────────

func TestServerBefore_RunsBeforeDecode(t *testing.T) {
	type ctxKey struct{}
	beforeRan := false
	h := server.NewJSONServer[testReq](
		func(ctx context.Context, _ testReq) (any, error) {
			if ctx.Value(ctxKey{}) != nil {
				beforeRan = true
			}
			return "ok", nil
		},
		server.ServerBefore(func(ctx context.Context, _ *http.Request) context.Context {
			return context.WithValue(ctx, ctxKey{}, true)
		}),
	)
	body, _ := json.Marshal(testReq{Name: "x"})
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !beforeRan {
		t.Error("ServerBefore hook did not run")
	}
}

func TestServerFinalizer_AlwaysRuns(t *testing.T) {
	finalized := false
	h := server.NewJSONServer[testReq](
		func(_ context.Context, _ testReq) (any, error) { return "ok", nil },
		server.ServerFinalizer(func(_ context.Context, _ *http.Request, _ *server.InterceptingWriter) {
			finalized = true
		}),
	)
	body, _ := json.Marshal(testReq{Name: "x"})
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	h.ServeHTTP(httptest.NewRecorder(), r)
	if !finalized {
		t.Error("finalizer was not called")
	}
}

// ── transport.DefaultErrorEncoder ────────────────────────────────────────────

func TestDefaultErrorEncoder_PlainText(t *testing.T) {
	w := httptest.NewRecorder()
	transport.DefaultErrorEncoder(context.Background(), errors.New("plain error"), w)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusInternalServerError)
	}
	if w.Body.String() != "plain error" {
		t.Errorf("body: got %q, want %q", w.Body.String(), "plain error")
	}
}

func TestDefaultErrorEncoder_StatusCoder(t *testing.T) {
	w := httptest.NewRecorder()
	transport.DefaultErrorEncoder(context.Background(), extraStatusErr{code: http.StatusBadRequest}, w)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ── interfaces.StatusCoder / Headerer ────────────────────────────────────────

type myResp struct{}

func (r myResp) StatusCode() int      { return http.StatusAccepted }
func (r myResp) Headers() http.Header { h := http.Header{}; h.Set("X-ID", "42"); return h }

func TestInterfaces_StatusCoderAndHeaderer(t *testing.T) {
	var _ interfaces.StatusCoder = myResp{}
	var _ interfaces.Headerer = myResp{}

	w := httptest.NewRecorder()
	server.EncodeJSONResponse(context.Background(), w, myResp{}) //nolint:errcheck
	if w.Code != http.StatusAccepted {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusAccepted)
	}
	if got := w.Header().Get("X-ID"); got != "42" {
		t.Errorf("X-ID: got %q, want %q", got, "42")
	}
}
