package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/transport"
	"github.com/dreamsxin/go-kit/transport/http/server"
)

func nopDecode(_ context.Context, r *http.Request) (interface{}, error) {
	return nil, nil
}

func TestServer_OK(t *testing.T) {
	ep := endpoint.Endpoint(func(_ context.Context, _ interface{}) (interface{}, error) {
		return map[string]string{"hello": "world"}, nil
	})
	s := server.NewServer(ep, nopDecode, server.EncodeJSONResponse)

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["hello"] != "world" {
		t.Errorf("unexpected body: %v", body)
	}
}

func TestServer_DecodeError(t *testing.T) {
	s := server.NewServer(endpoint.Nop,
		func(_ context.Context, _ *http.Request) (interface{}, error) {
			return nil, errors.New("bad request")
		},
		server.EncodeJSONResponse,
	)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestServer_EndpointError(t *testing.T) {
	ep := endpoint.Endpoint(func(_ context.Context, _ interface{}) (interface{}, error) {
		return nil, errors.New("endpoint fail")
	})
	s := server.NewServer(ep, nopDecode, server.EncodeJSONResponse)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
}

func TestServer_BeforeHook(t *testing.T) {
	type ctxKey struct{}
	var sawValue string
	ep := endpoint.Endpoint(func(ctx context.Context, _ interface{}) (interface{}, error) {
		sawValue, _ = ctx.Value(ctxKey{}).(string)
		return struct{}{}, nil
	})
	s := server.NewServer(ep, nopDecode, server.NopResponseEncoder,
		server.ServerBefore(func(ctx context.Context, r *http.Request) context.Context {
			return context.WithValue(ctx, ctxKey{}, "injected")
		}),
	)
	s.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if sawValue != "injected" {
		t.Errorf("before hook: want 'injected', got %q", sawValue)
	}
}

func TestServer_CustomErrorEncoder(t *testing.T) {
	ep := endpoint.Endpoint(func(_ context.Context, _ interface{}) (interface{}, error) {
		return nil, errors.New("oops")
	})
	s := server.NewServer(ep, nopDecode, server.EncodeJSONResponse,
		server.ServerErrorEncoder(func(_ context.Context, _ error, w http.ResponseWriter) {
			w.WriteHeader(http.StatusTeapot)
		}),
	)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusTeapot {
		t.Errorf("want 418, got %d", rec.Code)
	}
}

func TestServer_NilPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewServer with nil endpoint should panic")
		}
	}()
	server.NewServer(nil, nopDecode, server.EncodeJSONResponse)
}

func TestServer_ErrorHandler(t *testing.T) {
	var handled error
	ep := endpoint.Endpoint(func(_ context.Context, _ interface{}) (interface{}, error) {
		return nil, errors.New("handler error")
	})
	s := server.NewServer(ep, nopDecode, server.EncodeJSONResponse,
		server.ServerErrorHandler(transport.ErrorHandlerFunc(func(_ context.Context, err error) {
			handled = err
		})),
	)
	s.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if handled == nil {
		t.Error("error handler should have been called")
	}
}

func TestServer_NilErrorEncoderOption_FallsBackToDefault(t *testing.T) {
	ep := endpoint.Endpoint(func(_ context.Context, _ interface{}) (interface{}, error) {
		return nil, errors.New("endpoint fail")
	})
	s := server.NewServer(ep, nopDecode, server.EncodeJSONResponse,
		server.ServerErrorEncoder(nil),
	)

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "endpoint fail") {
		t.Errorf("want body to contain endpoint error, got %q", rec.Body.String())
	}
}

func TestServer_NilHooks_DoNotPanicAtRequestTime(t *testing.T) {
	s := server.NewServer(
		func(context.Context, any) (any, error) { return map[string]string{"ok": "true"}, nil },
		nopDecode,
		server.EncodeJSONResponse,
		server.ServerBefore(nil),
		server.ServerAfter(nil),
		server.ServerFinalizer(nil),
	)

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

// ── NewJSONServer ─────────────────────────────────────────────────────────────

func TestNewJSONServer_OK(t *testing.T) {
	type req struct{ Name string `json:"name"` }
	type resp struct{ Msg string `json:"msg"` }

	h := server.NewJSONServer[req](func(_ context.Context, r req) (any, error) {
		return resp{Msg: "hello " + r.Name}, nil
	})

	body := `{"name":"world"}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
	var got resp
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Msg != "hello world" {
		t.Errorf("want 'hello world', got %q", got.Msg)
	}
}

func TestNewJSONServer_ErrorUsesJSONErrorEncoder(t *testing.T) {
	h := server.NewJSONServer[struct{}](func(_ context.Context, _ struct{}) (any, error) {
		return nil, errors.New("something failed")
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("want JSON content-type, got %q", ct)
	}
	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if body["error"] == "" {
		t.Error("want non-empty error field in JSON body")
	}
}

// ── NewJSONServerWithMiddleware ───────────────────────────────────────────────

func TestNewJSONServerWithMiddleware(t *testing.T) {
	type req struct{ V int `json:"v"` }

	var mwCalled bool
	h := server.NewJSONServerWithMiddleware[req](
		func(_ context.Context, r req) (any, error) {
			return map[string]int{"doubled": r.V * 2}, nil
		},
		func(b *endpoint.Builder) *endpoint.Builder {
			return b.Use(func(next endpoint.Endpoint) endpoint.Endpoint {
				return func(ctx context.Context, req any) (any, error) {
					mwCalled = true
					return next(ctx, req)
				}
			})
		},
	)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"v":21}`)))

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
	if !mwCalled {
		t.Error("middleware should have been called")
	}
	var body map[string]int
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if body["doubled"] != 42 {
		t.Errorf("want 42, got %d", body["doubled"])
	}
}

// ── JSONErrorEncoder ──────────────────────────────────────────────────────────

func TestJSONErrorEncoder_DefaultStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	server.JSONErrorEncoder(context.Background(), errors.New("boom"), rec)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("want JSON content-type, got %q", ct)
	}
	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if body["error"] != "boom" {
		t.Errorf("want 'boom', got %q", body["error"])
	}
}

type statusErr struct{ code int }

func (e statusErr) Error() string  { return "status error" }
func (e statusErr) StatusCode() int { return e.code }

func TestJSONErrorEncoder_CustomStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	server.JSONErrorEncoder(context.Background(), statusErr{http.StatusTeapot}, rec)
	if rec.Code != http.StatusTeapot {
		t.Errorf("want 418, got %d", rec.Code)
	}
}

// ── DecodeJSONRequest ─────────────────────────────────────────────────────────

func TestDecodeJSONRequest(t *testing.T) {
	type payload struct{ X int `json:"x"` }
	dec := server.DecodeJSONRequest[payload]()

	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"x":99}`))
	got, err := dec(context.Background(), r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.(payload).X != 99 {
		t.Errorf("want 99, got %v", got)
	}
}
