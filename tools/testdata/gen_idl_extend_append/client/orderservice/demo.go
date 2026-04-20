package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	idl "example.com/gen_idl_extend_append"
)

// ─────────────────────────── HTTP Client ───────────────────────────

// OrderServiceHTTPClient OrderService HTTP 客户端
type OrderServiceHTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewOrderServiceHTTPClient 创建 HTTP 客户端，baseURL 如 "http://localhost:8080"
func NewOrderServiceHTTPClient(baseURL string) *OrderServiceHTTPClient {
	return &OrderServiceHTTPClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *OrderServiceHTTPClient) do(ctx context.Context, path string, req, resp interface{}) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	r, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", r.StatusCode)
	}
	return json.NewDecoder(r.Body).Decode(resp)
}


// PlaceOrder 通过 HTTP 调用 PlaceOrder
func (c *OrderServiceHTTPClient) PlaceOrder(ctx context.Context, req idl.PlaceOrderRequest) (idl.PlaceOrderResponse, error) {
	var resp idl.PlaceOrderResponse
	return resp, c.do(ctx, "/placeorder", req, &resp)
}


// ─────────────────────────── 通用接口 ───────────────────────────

// OrderServiceClient 统一客户端接口（HTTP 和 gRPC 均实现该接口）
type OrderServiceClient interface {
	PlaceOrder(ctx context.Context, req idl.PlaceOrderRequest) (idl.PlaceOrderResponse, error)

}

// ─────────────────────────── Demo logic ───────────────────────────

func runDemo(client OrderServiceClient, logger *log.Logger) {
	ctx := context.Background()

	logger.Println(">>> PlaceOrder")
	placeorderResp, err := client.PlaceOrder(ctx, idl.PlaceOrderRequest{})
	if err != nil {
		logger.Printf("    FAIL: %v", err)
	} else {
		logger.Printf("    OK  : %+v", placeorderResp)
	}

}

// ─────────────────────────── main ───────────────────────────

func main() {
	var (
		mode     = flag.String("mode", "http", "client mode: http")
		httpAddr = flag.String("http.addr", "http://localhost:8080", "HTTP server address (mode=http)")
	)
	flag.Parse()

	logger := log.New(log.Writer(), "[demo] ", log.LstdFlags)

	switch *mode {
	case "http":
		logger.Printf("=== OrderService HTTP Client Demo  addr=%s ===", *httpAddr)
		runDemo(NewOrderServiceHTTPClient(*httpAddr), logger)
	default:
		logger.Fatalf("unknown mode %q, use -mode=http", *mode)
	}

	logger.Println("=== Demo completed ===")
}
