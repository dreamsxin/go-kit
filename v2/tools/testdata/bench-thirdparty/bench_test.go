// Package benchthirdparty compares the same JSON POST route across the
// frameworks a Go service realistically picks between — each benchmark
// starts its own server in this one test process — over one keep-alive
// connection each. It is deliberately not
// part of the release: its dependencies are the heaviest in the workspace,
// and its numbers belong in docs/benchmarks.md with the environment spelled
// out. Run it with:
//
//	go test -bench . -benchmem -benchtime=2s
package benchthirdparty

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/kit"
	"github.com/gin-gonic/gin"
	"github.com/go-chi/chi/v5"
	"github.com/labstack/echo/v4"
)

const payload = `{"name":"benchmark","count":21}`

type echoRequest struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type echoResponse struct {
	Echo   string `json:"echo"`
	Double int    `json:"double"`
}

func bench(b *testing.B, url string) {
	client := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: 8}}
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

func BenchmarkStdlib(b *testing.B) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /echo", func(w http.ResponseWriter, r *http.Request) {
		var req echoRequest
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(echoResponse{req.Name, req.Count * 2})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	bench(b, server.URL+"/echo")
}

func BenchmarkKit(b *testing.B) {
	component, err := kit.NewHTTP("127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	kit.HandleJSON(component, "POST /echo", func(_ context.Context, req echoRequest) (echoResponse, error) {
		return echoResponse{req.Name, req.Count * 2}, nil
	})
	server := httptest.NewServer(component)
	defer server.Close()
	bench(b, server.URL+"/echo")
}

func BenchmarkChi(b *testing.B) {
	r := chi.NewRouter()
	r.Post("/echo", func(w http.ResponseWriter, r *http.Request) {
		var req echoRequest
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(echoResponse{req.Name, req.Count * 2})
	})
	server := httptest.NewServer(r)
	defer server.Close()
	bench(b, server.URL+"/echo")
}

func BenchmarkGin(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	g := gin.New()
	g.POST("/echo", func(c *gin.Context) {
		var req echoRequest
		_ = c.ShouldBindJSON(&req)
		c.JSON(http.StatusOK, echoResponse{req.Name, req.Count * 2})
	})
	server := httptest.NewServer(g)
	defer server.Close()
	bench(b, server.URL+"/echo")
}

func BenchmarkEcho(b *testing.B) {
	e := echo.New()
	e.HideBanner = true
	e.POST("/echo", func(c echo.Context) error {
		req := echoRequest{}
		_ = c.Bind(&req)
		return c.JSON(http.StatusOK, echoResponse{req.Name, req.Count * 2})
	})
	server := httptest.NewServer(e)
	defer server.Close()
	bench(b, server.URL+"/echo")
}
