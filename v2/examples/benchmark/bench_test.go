// Package benchmark is the fair, in-repository comparison: the same JSON
// POST route served from one process twice, once by the standard library
// alone and once by kit, measured with the same client over loopback.
//
// Run it with:
//
//	go -C examples test ./benchmark -bench . -benchmem
//
// It answers the only question this repository can answer honestly — what
// the framework costs over the platform. Numbers against other frameworks
// live in docs/benchmarks.md and come from the harness under
// tools/testdata/bench-thirdparty, which carries its own caveats.
package benchmark

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/kit"
)

type echoRequest struct {
	Name  string `json:"name" validate:"minLen=1,maxLen=64"`
	Count int    `json:"count" validate:"min=0,max=1000"`
}

type echoResponse struct {
	Echo   string `json:"echo"`
	Double int    `json:"double"`
}

const payload = `{"name":"benchmark","count":21}`

func stdlibEcho(w http.ResponseWriter, r *http.Request) {
	var req echoRequest
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || json.Unmarshal(body, &req) != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(echoResponse{Echo: req.Name, Double: req.Count * 2})
}

// benchEcho drives one keep-alive connection as fast as the server answers
// and discards responses, which is what ns/op measures here.
func benchEcho(b *testing.B, url string) {
	client := &http.Client{Transport: &http.Transport{
		MaxIdleConnsPerHost: 8,
	}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		response, err := client.Post(url, "application/json", strings.NewReader(payload))
		if err != nil {
			b.Fatalf("post: %v", err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			b.Fatalf("status %d", response.StatusCode)
		}
	}
}

func BenchmarkStdlibJSON(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(stdlibEcho))
	defer server.Close()
	benchEcho(b, server.URL)
}

func BenchmarkKitJSON(b *testing.B) {
	component, err := kit.NewHTTP("127.0.0.1:0")
	if err != nil {
		b.Fatalf("kit.NewHTTP: %v", err)
	}
	kit.HandleJSON(component, "POST /echo", func(_ context.Context, req echoRequest) (echoResponse, error) {
		return echoResponse{Echo: req.Name, Double: req.Count * 2}, nil
	})
	server := httptest.NewServer(component)
	defer server.Close()
	benchEcho(b, server.URL+"/echo")
}
