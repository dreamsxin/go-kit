package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	httpclient "github.com/dreamsxin/go-kit/transport/http/client"
	httpserver "github.com/dreamsxin/go-kit/transport/http/server"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	handler := httpserver.NewJSONServer[echoReq](
		func(_ context.Context, req echoReq) (any, error) {
			return echoResp{Echo: "echo: " + req.Message}, nil
		},
	)
	return httptest.NewServer(handler)
}

func TestNewJSONClient_Success(t *testing.T) {
	srv := newServer(t)
	defer srv.Close()

	ep, err := httpclient.NewJSONClient[echoResp](http.MethodPost, srv.URL)
	if err != nil {
		t.Fatalf("NewJSONClient: %v", err)
	}

	resp, err := ep(context.Background(), echoReq{Message: "hello"})
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	got := resp.(echoResp)
	if got.Echo != "echo: hello" {
		t.Errorf("got %q, want %q", got.Echo, "echo: hello")
	}
}

func TestClientBefore_InjectsHeader(t *testing.T) {
	var capturedHeader string
	inner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Get("X-Test")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"echo":"ok"}`)) //nolint:errcheck
	}))
	defer inner.Close()

	ep, err := httpclient.NewJSONClient[echoResp](
		http.MethodPost,
		inner.URL,
		httpclient.ClientBefore(func(ctx context.Context, r *http.Request) context.Context {
			r.Header.Set("X-Test", "injected")
			return ctx
		}),
	)
	if err != nil {
		t.Fatalf("NewJSONClient: %v", err)
	}

	ep(context.Background(), echoReq{Message: "test"}) //nolint:errcheck
	if capturedHeader != "injected" {
		t.Errorf("header: got %q, want %q", capturedHeader, "injected")
	}
}

func TestClientAfter_ReadsResponseHeader(t *testing.T) {
	inner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "custom-value")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"echo":"ok"}`)) //nolint:errcheck
	}))
	defer inner.Close()

	var gotHeader string
	ep, err := httpclient.NewJSONClient[echoResp](
		http.MethodPost,
		inner.URL,
		httpclient.ClientAfter(func(ctx context.Context, r *http.Response, _ error) context.Context {
			gotHeader = r.Header.Get("X-Custom")
			return ctx
		}),
	)
	if err != nil {
		t.Fatalf("NewJSONClient: %v", err)
	}

	ep(context.Background(), echoReq{Message: "test"}) //nolint:errcheck
	if gotHeader != "custom-value" {
		t.Errorf("header: got %q, want %q", gotHeader, "custom-value")
	}
}

func TestClientFinalizer_AlwaysRuns(t *testing.T) {
	srv := newServer(t)
	defer srv.Close()

	finalized := make(chan struct{}, 1)
	ep, err := httpclient.NewJSONClient[echoResp](
		http.MethodPost,
		srv.URL,
		httpclient.ClientFinalizer(func(_ context.Context, err error) {
			finalized <- struct{}{}
		}),
	)
	if err != nil {
		t.Fatalf("NewJSONClient: %v", err)
	}

	ep(context.Background(), echoReq{Message: "finalizer"}) //nolint:errcheck

	select {
	case <-finalized:
	default:
		t.Error("finalizer was not called")
	}
}

func TestSetClient_CustomHTTPClient(t *testing.T) {
	srv := newServer(t)
	defer srv.Close()

	custom := &http.Client{}
	ep, err := httpclient.NewJSONClient[echoResp](
		http.MethodPost,
		srv.URL,
		httpclient.SetClient(custom),
	)
	if err != nil {
		t.Fatalf("NewJSONClient: %v", err)
	}

	resp, err := ep(context.Background(), echoReq{Message: "custom"})
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	got := resp.(echoResp)
	if got.Echo != "echo: custom" {
		t.Errorf("got %q, want %q", got.Echo, "echo: custom")
	}
}

func TestNewEchoServer_RoundTrip(t *testing.T) {
	srv := newEchoServer()
	defer srv.Close()

	ep, err := httpclient.NewJSONClient[echoResp](http.MethodPost, srv.URL)
	if err != nil {
		t.Fatalf("NewJSONClient: %v", err)
	}

	resp, err := ep(context.Background(), echoReq{Message: "round-trip"})
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	got := resp.(echoResp)
	if got.Echo != "echo: round-trip" {
		t.Errorf("got %q, want %q", got.Echo, "echo: round-trip")
	}
}
