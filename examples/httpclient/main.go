// Package main demonstrates the HTTP client components:
//
//   - transport/http/client.NewJSONClient  — zero-boilerplate typed client
//   - transport/http/client.NewClient      — full-control client
//   - ClientBefore / ClientAfter hooks
//   - ClientFinalizer
//   - Round-trip test: NewJSONServer (server) + NewJSONClient (client)
//
// The example starts an in-process httptest server so no external service
// is needed.
//
// Run:
//
//	go run ./examples/httpclient
package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	httpclient "github.com/dreamsxin/go-kit/transport/http/client"
	httpserver "github.com/dreamsxin/go-kit/transport/http/server"
)

// ── Shared types ──────────────────────────────────────────────────────────────

type echoReq  struct{ Message string `json:"message"` }
type echoResp struct{ Echo    string `json:"echo"`    }

// ── Server side ───────────────────────────────────────────────────────────────

// newEchoServer creates an httptest.Server that echoes the request message.
func newEchoServer() *httptest.Server {
	handler := httpserver.NewJSONServer[echoReq](
		func(_ context.Context, req echoReq) (any, error) {
			return echoResp{Echo: "echo: " + req.Message}, nil
		},
	)
	return httptest.NewServer(handler)
}

// ── Client demos ──────────────────────────────────────────────────────────────

// demo1_NewJSONClient shows the simplest typed client.
func demo1_NewJSONClient(baseURL string) {
	fmt.Println("=== 1. NewJSONClient[echoResp] ===")

	ep, err := httpclient.NewJSONClient[echoResp](
		http.MethodPost,
		baseURL,
	)
	if err != nil {
		fmt.Printf("  NewJSONClient error: %v\n", err)
		return
	}

	resp, err := ep(context.Background(), echoReq{Message: "hello"})
	if err != nil {
		fmt.Printf("  call error: %v\n", err)
		return
	}
	fmt.Printf("  response: %+v\n", resp.(echoResp))
}

// demo2_ClientBefore shows how to inject headers before the request is sent.
func demo2_ClientBefore(baseURL string) {
	fmt.Println("\n=== 2. ClientBefore — inject Authorization header ===")

	ep, _ := httpclient.NewJSONClient[echoResp](
		http.MethodPost,
		baseURL,
		httpclient.ClientBefore(func(ctx context.Context, r *http.Request) context.Context {
			r.Header.Set("Authorization", "Bearer demo-token")
			fmt.Printf("  injected header: %s\n", r.Header.Get("Authorization"))
			return ctx
		}),
	)

	ep(context.Background(), echoReq{Message: "auth test"}) //nolint:errcheck
}

// demo3_ClientAfter shows how to read response headers after the call.
func demo3_ClientAfter(baseURL string) {
	fmt.Println("\n=== 3. ClientAfter — read response headers ===")

	ep, _ := httpclient.NewJSONClient[echoResp](
		http.MethodPost,
		baseURL,
		httpclient.ClientAfter(func(ctx context.Context, r *http.Response, _ error) context.Context {
			fmt.Printf("  response Content-Type: %s\n", r.Header.Get("Content-Type"))
			return ctx
		}),
	)

	ep(context.Background(), echoReq{Message: "after test"}) //nolint:errcheck
}

// demo4_ClientFinalizer shows the finalizer hook (always runs, even on error).
func demo4_ClientFinalizer(baseURL string) {
	fmt.Println("\n=== 4. ClientFinalizer — always runs ===")

	finalized := make(chan struct{}, 1)
	ep, _ := httpclient.NewJSONClient[echoResp](
		http.MethodPost,
		baseURL,
		httpclient.ClientFinalizer(func(_ context.Context, err error) {
			fmt.Printf("  finalizer called, err=%v\n", err)
			finalized <- struct{}{}
		}),
	)

	ep(context.Background(), echoReq{Message: "finalizer test"}) //nolint:errcheck
	<-finalized
}

// demo5_SetClient shows how to swap in a custom *http.Client (e.g. with TLS).
func demo5_SetClient(baseURL string) {
	fmt.Println("\n=== 5. SetClient — custom http.Client ===")

	custom := &http.Client{Timeout: 0} // no timeout for demo
	ep, _ := httpclient.NewJSONClient[echoResp](
		http.MethodPost,
		baseURL,
		httpclient.SetClient(custom),
	)

	resp, err := ep(context.Background(), echoReq{Message: "custom client"})
	if err != nil {
		fmt.Printf("  error: %v\n", err)
		return
	}
	fmt.Printf("  response: %+v\n", resp.(echoResp))
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	srv := newEchoServer()
	defer srv.Close()

	fmt.Printf("in-process server: %s\n\n", srv.URL)

	demo1_NewJSONClient(srv.URL)
	demo2_ClientBefore(srv.URL)
	demo3_ClientAfter(srv.URL)
	demo4_ClientFinalizer(srv.URL)
	demo5_SetClient(srv.URL)

	fmt.Println("\nDone.")
}
