package client_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	httpclient "github.com/dreamsxin/go-kit/transport/http/client"
)

type echoReq struct {
	Message string `json:"message"`
}
type echoResp struct {
	Echo string `json:"echo"`
}

func newEchoServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req echoReq
		json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(echoResp{Echo: "echo: " + req.Message}) //nolint:errcheck
	}))
}

// ── NewJSONClient ─────────────────────────────────────────────────────────────

func TestNewJSONClient_Success(t *testing.T) {
	srv := newEchoServer(t)
	defer srv.Close()

	ep, err := httpclient.NewJSONClient[echoResp](http.MethodPost, srv.URL)
	if err != nil {
		t.Fatalf("NewJSONClient: %v", err)
	}
	resp, err := ep(context.Background(), echoReq{Message: "hello"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if resp.(echoResp).Echo != "echo: hello" {
		t.Errorf("echo: got %q, want %q", resp.(echoResp).Echo, "echo: hello")
	}
}

func TestNewJSONClient_InvalidURL(t *testing.T) {
	_, err := httpclient.NewJSONClient[echoResp](http.MethodPost, "://bad-url")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

// ── NewJSONClientWithRetry ────────────────────────────────────────────────────

func TestNewJSONClientWithRetry_Success(t *testing.T) {
	srv := newEchoServer(t)
	defer srv.Close()

	ep, err := httpclient.NewJSONClientWithRetry[echoResp](http.MethodPost, srv.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("NewJSONClientWithRetry: %v", err)
	}
	resp, err := ep(context.Background(), echoReq{Message: "retry"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if resp.(echoResp).Echo != "echo: retry" {
		t.Errorf("echo: got %q, want %q", resp.(echoResp).Echo, "echo: retry")
	}
}

func TestNewJSONClientWithRetry_Timeout(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.Write([]byte(`{}`)) //nolint:errcheck
	}))
	defer slow.Close()

	ep, err := httpclient.NewJSONClientWithRetry[echoResp](http.MethodPost, slow.URL, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("NewJSONClientWithRetry: %v", err)
	}
	_, err = ep(context.Background(), echoReq{Message: "slow"})
	if err == nil {
		t.Error("expected timeout error")
	}
}

// ── NewClient (low-level) ─────────────────────────────────────────────────────

func TestNewClient_EncodeDecodeRoundTrip(t *testing.T) {
	srv := newEchoServer(t)
	defer srv.Close()

	tgt, _ := url.Parse(srv.URL)
	enc := httpclient.EncodeJSONRequest
	dec := func(_ context.Context, r *http.Response) (any, error) {
		var resp echoResp
		json.NewDecoder(r.Body).Decode(&resp) //nolint:errcheck
		return resp, nil
	}
	ep := httpclient.NewClient(http.MethodPost, tgt, enc, dec).Endpoint()
	resp, err := ep(context.Background(), echoReq{Message: "low-level"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if resp.(echoResp).Echo != "echo: low-level" {
		t.Errorf("echo: got %q, want %q", resp.(echoResp).Echo, "echo: low-level")
	}
}

// ── ClientBefore ─────────────────────────────────────────────────────────────

func TestClientBefore_InjectsHeader(t *testing.T) {
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Get("X-Token")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"echo":"ok"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	ep, _ := httpclient.NewJSONClient[echoResp](
		http.MethodPost, srv.URL,
		httpclient.ClientBefore(func(ctx context.Context, r *http.Request) context.Context {
			r.Header.Set("X-Token", "secret")
			return ctx
		}),
	)
	ep(context.Background(), echoReq{}) //nolint:errcheck
	if captured != "secret" {
		t.Errorf("X-Token: got %q, want %q", captured, "secret")
	}
}

// ── ClientAfter ──────────────────────────────────────────────────────────────

func TestClientAfter_ReadsHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Resp", "resp-value")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"echo":"ok"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	var gotHeader string
	ep, _ := httpclient.NewJSONClient[echoResp](
		http.MethodPost, srv.URL,
		httpclient.ClientAfter(func(ctx context.Context, r *http.Response, _ error) context.Context {
			gotHeader = r.Header.Get("X-Resp")
			return ctx
		}),
	)
	ep(context.Background(), echoReq{}) //nolint:errcheck
	if gotHeader != "resp-value" {
		t.Errorf("X-Resp: got %q, want %q", gotHeader, "resp-value")
	}
}

// ── ClientFinalizer ───────────────────────────────────────────────────────────

func TestClientFinalizer_RunsOnSuccess(t *testing.T) {
	srv := newEchoServer(t)
	defer srv.Close()

	done := make(chan struct{}, 1)
	ep, _ := httpclient.NewJSONClient[echoResp](
		http.MethodPost, srv.URL,
		httpclient.ClientFinalizer(func(_ context.Context, _ error) {
			done <- struct{}{}
		}),
	)
	ep(context.Background(), echoReq{Message: "fin"}) //nolint:errcheck
	select {
	case <-done:
	default:
		t.Error("finalizer was not called")
	}
}

func TestClient_NilHooks_DoNotPanicAtRequestTime(t *testing.T) {
	srv := newEchoServer(t)
	defer srv.Close()

	ep, err := httpclient.NewJSONClient[echoResp](
		http.MethodPost, srv.URL,
		httpclient.ClientBefore(nil),
		httpclient.ClientAfter(nil),
		httpclient.ClientFinalizer(nil),
	)
	if err != nil {
		t.Fatalf("NewJSONClient: %v", err)
	}

	resp, err := ep(context.Background(), echoReq{Message: "nil-hooks"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if resp.(echoResp).Echo != "echo: nil-hooks" {
		t.Errorf("echo: got %q, want %q", resp.(echoResp).Echo, "echo: nil-hooks")
	}
}

// ── SetClient ─────────────────────────────────────────────────────────────────

func TestSetClient_UsesCustomClient(t *testing.T) {
	srv := newEchoServer(t)
	defer srv.Close()

	custom := &http.Client{}
	ep, _ := httpclient.NewJSONClient[echoResp](
		http.MethodPost, srv.URL,
		httpclient.SetClient(custom),
	)
	resp, err := ep(context.Background(), echoReq{Message: "custom"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if resp.(echoResp).Echo != "echo: custom" {
		t.Errorf("echo: got %q, want %q", resp.(echoResp).Echo, "echo: custom")
	}
}

func TestSetClient_NilFallsBackToDefaultClient(t *testing.T) {
	srv := newEchoServer(t)
	defer srv.Close()

	ep, err := httpclient.NewJSONClient[echoResp](
		http.MethodPost, srv.URL,
		httpclient.SetClient(nil),
	)
	if err != nil {
		t.Fatalf("NewJSONClient: %v", err)
	}
	resp, err := ep(context.Background(), echoReq{Message: "fallback"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if resp.(echoResp).Echo != "echo: fallback" {
		t.Errorf("echo: got %q, want %q", resp.(echoResp).Echo, "echo: fallback")
	}
}

// ── EncodeJSONRequest ─────────────────────────────────────────────────────────

func TestEncodeJSONRequest_SetsBody(t *testing.T) {
	tgt, _ := url.Parse("http://example.com/test")
	r, _ := http.NewRequest(http.MethodPost, tgt.String(), nil)
	req, err := httpclient.EncodeJSONRequest(context.Background(), r, echoReq{Message: "encode"})
	if err != nil {
		t.Fatalf("EncodeJSONRequest: %v", err)
	}
	var decoded echoReq
	json.NewDecoder(req.Body).Decode(&decoded) //nolint:errcheck
	if decoded.Message != "encode" {
		t.Errorf("message: got %q, want %q", decoded.Message, "encode")
	}
}

// ── BufferedStream ────────────────────────────────────────────────────────────

func TestBufferedStream(t *testing.T) {
	srv := newEchoServer(t)
	defer srv.Close()

	ep, _ := httpclient.NewJSONClient[echoResp](
		http.MethodPost, srv.URL,
		httpclient.BufferedStream(false),
	)
	resp, err := ep(context.Background(), echoReq{Message: "stream"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if resp.(echoResp).Echo != "echo: stream" {
		t.Errorf("echo: got %q, want %q", resp.(echoResp).Echo, "echo: stream")
	}
}

// ── NewExplicitClient ─────────────────────────────────────────────────────────

func TestNewExplicitClient(t *testing.T) {
	srv := newEchoServer(t)
	defer srv.Close()

	tgt, _ := url.Parse(srv.URL)
	createReq := func(ctx context.Context, _ *http.Request, request any) (*http.Request, error) {
		body, _ := json.Marshal(request)
		r, _ := http.NewRequestWithContext(ctx, http.MethodPost, tgt.String(), bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		return r, nil
	}
	dec := func(_ context.Context, r *http.Response) (any, error) {
		var resp echoResp
		json.NewDecoder(r.Body).Decode(&resp) //nolint:errcheck
		return resp, nil
	}
	ep := httpclient.NewExplicitClient(createReq, dec).Endpoint()
	resp, err := ep(context.Background(), echoReq{Message: "explicit"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if resp.(echoResp).Echo != "echo: explicit" {
		t.Errorf("echo: got %q, want %q", resp.(echoResp).Echo, "echo: explicit")
	}
}

func TestNewExplicitClient_PanicsOnNilEssentialParameters(t *testing.T) {
	tests := []struct {
		name string
		req  httpclient.EncodeRequestFunc
		dec  httpclient.DecodeResponseFunc
	}{
		{
			name: "nil request encoder",
			dec: func(context.Context, *http.Response) (any, error) {
				return nil, nil
			},
		},
		{
			name: "nil response decoder",
			req: func(context.Context, *http.Request, any) (*http.Request, error) {
				return http.NewRequest(http.MethodGet, "http://example.com", nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected panic for nil essential parameter")
				}
			}()
			httpclient.NewExplicitClient(tt.req, tt.dec)
		})
	}
}

func TestNewClient_PanicsOnNilEssentialParameters(t *testing.T) {
	tgt, _ := url.Parse("http://example.com")
	dec := func(context.Context, *http.Response) (any, error) { return nil, nil }
	enc := func(context.Context, *http.Request, any) (*http.Request, error) {
		return http.NewRequest(http.MethodGet, tgt.String(), nil)
	}

	tests := []struct {
		name string
		tgt  *url.URL
		enc  httpclient.EncodeRequestFunc
	}{
		{name: "nil target", enc: enc},
		{name: "nil encoder", tgt: tgt},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected panic for nil essential parameter")
				}
			}()
			httpclient.NewClient(http.MethodGet, tt.tgt, tt.enc, dec)
		})
	}
}
