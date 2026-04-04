package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
