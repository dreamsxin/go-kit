package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
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

func (c *OrderServiceHTTPClient) do(ctx context.Context, method, path string, req, resp interface{}) error {
	var body *bytes.Reader
	if req != nil {
		raw, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("marshal: %w", err)
		}
		body = bytes.NewReader(raw)
	} else {
		body = bytes.NewReader(nil)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	if req != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
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

func buildGETPath(path string, req interface{}) string {
	b, _ := json.Marshal(req)
	var params map[string]interface{}
	_ = json.Unmarshal(b, &params)
	if len(params) == 0 {
		return path
	}
	query := url.Values{}
	for k, v := range params {
		if v == nil {
			continue
		}
		token := "{" + k + "}"
		value := fmt.Sprint(v)
		if strings.Contains(path, token) {
			path = strings.ReplaceAll(path, token, url.PathEscape(value))
			continue
		}
		query.Set(k, value)
	}
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return path
}

// PlaceOrder 通过 HTTP 调用 PlaceOrder
func (c *OrderServiceHTTPClient) PlaceOrder(ctx context.Context, req idl.PlaceOrderRequest) (idl.PlaceOrderResponse, error) {
	var resp idl.PlaceOrderResponse
	return resp, c.do(ctx, "POST", "/placeorder", req, &resp)
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
