package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	transportclient "github.com/dreamsxin/go-kit/transport/http/client"
)

// ─────────────────────────── helpers ───────────────────────────

type echoPayload struct {
	Msg string `json:"msg"`
}

// echoServer mirrors back the request body as-is.
func echoServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.Copy(w, r.Body) //nolint:errcheck
	}))
}

// ─────────────────────────── TestHttpClient ───────────────────────────

func TestHttpClient(t *testing.T) {
	var capturedHeader http.Header
	var capturedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedHeader = r.Header
		capturedBody = strings.TrimSpace(string(b))
		w.Write(b) //nolint:errcheck
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)

	client := transportclient.NewClient(
		"POST",
		serverURL,
		transportclient.EncodeJSONRequest,
		func(ctx context.Context, r *http.Response) (interface{}, error) { return nil, nil },
	).Endpoint()

	type payload struct {
		Foo string `json:"foo"`
	}

	// EncodeJSONRequest sets Content-Type automatically
	if _, err := client(context.Background(), &payload{Foo: "bar"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedBody != `{"foo":"bar"}` {
		t.Errorf("body: want %q, got %q", `{"foo":"bar"}`, capturedBody)
	}
	if ct := capturedHeader.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: want application/json prefix, got %q", ct)
	}
}

// ─────────────────────────── TestHttpClient_Headerer ───────────────────────────

// headerPayload implements transport/http/interfaces.Headerer
type headerPayload struct {
	Value string `json:"value"`
}

func (h *headerPayload) Headers() http.Header {
	return http.Header{"X-Custom": []string{"my-value"}}
}

func TestHttpClient_Headerer(t *testing.T) {
	var capturedHeader http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	client := transportclient.NewClient(
		"POST",
		serverURL,
		transportclient.EncodeJSONRequest,
		func(ctx context.Context, r *http.Response) (interface{}, error) { return nil, nil },
	).Endpoint()

	if _, err := client(context.Background(), &headerPayload{Value: "test"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedHeader.Get("X-Custom") != "my-value" {
		t.Errorf("X-Custom header: want 'my-value', got %q", capturedHeader.Get("X-Custom"))
	}
}

// ─────────────────────────── TestHttpClient_Before ───────────────────────────

func TestHttpClient_Before(t *testing.T) {
	var capturedHeader http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	client := transportclient.NewClient(
		"POST",
		serverURL,
		transportclient.EncodeJSONRequest,
		func(ctx context.Context, r *http.Response) (interface{}, error) { return nil, nil },
		transportclient.ClientBefore(func(ctx context.Context, r *http.Request) context.Context {
			r.Header.Set("X-Before", "injected")
			return ctx
		}),
	).Endpoint()

	if _, err := client(context.Background(), struct{}{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedHeader.Get("X-Before") != "injected" {
		t.Errorf("X-Before header: want 'injected', got %q", capturedHeader.Get("X-Before"))
	}
}

// ─────────────────────────── TestHttpClient_After ───────────────────────────

func TestHttpClient_After(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Server-Tag", "pong")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var capturedResponseHeader http.Header
	serverURL, _ := url.Parse(server.URL)
	client := transportclient.NewClient(
		"GET",
		serverURL,
		transportclient.EncodeJSONRequest,
		func(ctx context.Context, r *http.Response) (interface{}, error) { return nil, nil },
		transportclient.ClientAfter(func(ctx context.Context, r *http.Response, _ error) context.Context {
			capturedResponseHeader = r.Header
			return ctx
		}),
	).Endpoint()

	if _, err := client(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedResponseHeader.Get("X-Server-Tag") != "pong" {
		t.Errorf("X-Server-Tag: want 'pong', got %q", capturedResponseHeader.Get("X-Server-Tag"))
	}
}

// ─────────────────────────── TestHttpClient_DecodeResponse ───────────────────────────

func TestHttpClient_DecodeResponse(t *testing.T) {
	want := echoPayload{Msg: "hello world"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want) //nolint:errcheck
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	client := transportclient.NewClient(
		"GET",
		serverURL,
		transportclient.EncodeJSONRequest,
		func(ctx context.Context, r *http.Response) (interface{}, error) {
			var p echoPayload
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				return nil, err
			}
			return p, nil
		},
	).Endpoint()

	resp, err := client(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := resp.(echoPayload)
	if !ok {
		t.Fatalf("expected echoPayload, got %T", resp)
	}
	if got.Msg != want.Msg {
		t.Errorf("Msg: want %q, got %q", want.Msg, got.Msg)
	}
}

// ─────────────────────────── TestHttpClient_Non200 ───────────────────────────

func TestHttpClient_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)

	var decodeCalled bool
	client := transportclient.NewClient(
		"GET",
		serverURL,
		transportclient.EncodeJSONRequest,
		func(ctx context.Context, r *http.Response) (interface{}, error) {
			decodeCalled = true
			return r.StatusCode, nil
		},
	).Endpoint()

	resp, err := client(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decodeCalled {
		t.Error("decode func should have been called even for non-200")
	}
	if resp.(int) != http.StatusNotFound {
		t.Errorf("status: want %d, got %v", http.StatusNotFound, resp)
	}
}

// ─────────────────────────── TestHttpClient_Finalizer ───────────────────────────

func TestHttpClient_Finalizer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	finalized := make(chan struct{}, 1)

	client := transportclient.NewClient(
		"GET",
		serverURL,
		transportclient.EncodeJSONRequest,
		func(ctx context.Context, r *http.Response) (interface{}, error) { return nil, nil },
		transportclient.ClientFinalizer(func(ctx context.Context, err error) {
			finalized <- struct{}{}
		}),
	).Endpoint()

	if _, err := client(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-finalized:
		// expected
	default:
		t.Error("finalizer was not called")
	}
}
