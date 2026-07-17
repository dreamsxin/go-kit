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

	idl "example.com/gen_fromdb_sqlite"
	transporthttp "github.com/dreamsxin/go-kit/v2/transport/http"
)

// ─────────────────────────── HTTP Client ───────────────────────────

// CatalogServiceHTTPClient CatalogService HTTP 客户端
type CatalogServiceHTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewCatalogServiceHTTPClient 创建 HTTP 客户端，baseURL 如 "http://localhost:8080"
func NewCatalogServiceHTTPClient(baseURL string) *CatalogServiceHTTPClient {
	return &CatalogServiceHTTPClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *CatalogServiceHTTPClient) do(ctx context.Context, method, path string, req, resp interface{}) error {
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

func buildGETPath(path string, req interface{}) (string, error) {
	return transporthttp.EncodePathAndQuery(path, req)
}

func buildRequestPath(path string, req interface{}) (string, error) {
	return transporthttp.EncodePath(path, req)
}

// CreateUser 通过 HTTP 调用 CreateUser
func (c *CatalogServiceHTTPClient) CreateUser(ctx context.Context, req idl.CreateUserRequest) (idl.CreateUserResponse, error) {
	var resp idl.CreateUserResponse
	path, err := buildRequestPath("/user", req)
	if err != nil {
		return resp, fmt.Errorf("encode request path: %w", err)
	}
	return resp, c.do(ctx, "POST", path, req, &resp)
}

// GetUser 通过 HTTP 调用 GetUser
func (c *CatalogServiceHTTPClient) GetUser(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error) {
	var resp idl.GetUserResponse
	path, err := buildGETPath("/user/{id}", req)
	if err != nil {
		return resp, fmt.Errorf("encode GET query: %w", err)
	}
	return resp, c.do(ctx, "GET", path, nil, &resp)
}

// UpdateUser 通过 HTTP 调用 UpdateUser
func (c *CatalogServiceHTTPClient) UpdateUser(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error) {
	var resp idl.UpdateUserResponse
	path, err := buildRequestPath("/user/{id}", req)
	if err != nil {
		return resp, fmt.Errorf("encode request path: %w", err)
	}
	return resp, c.do(ctx, "PUT", path, req, &resp)
}

// DeleteUser 通过 HTTP 调用 DeleteUser
func (c *CatalogServiceHTTPClient) DeleteUser(ctx context.Context, req idl.DeleteUserRequest) (idl.DeleteUserResponse, error) {
	var resp idl.DeleteUserResponse
	path, err := buildRequestPath("/user/{id}", req)
	if err != nil {
		return resp, fmt.Errorf("encode request path: %w", err)
	}
	return resp, c.do(ctx, "DELETE", path, req, &resp)
}

// ListUsers 通过 HTTP 调用 ListUsers
func (c *CatalogServiceHTTPClient) ListUsers(ctx context.Context, req idl.ListUsersRequest) (idl.ListUsersResponse, error) {
	var resp idl.ListUsersResponse
	path, err := buildGETPath("/users", req)
	if err != nil {
		return resp, fmt.Errorf("encode GET query: %w", err)
	}
	return resp, c.do(ctx, "GET", path, nil, &resp)
}

// ─────────────────────────── 通用接口 ───────────────────────────

// CatalogServiceClient 统一客户端接口（HTTP 和 gRPC 均实现该接口）
type CatalogServiceClient interface {
	CreateUser(ctx context.Context, req idl.CreateUserRequest) (idl.CreateUserResponse, error)
	GetUser(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error)
	UpdateUser(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error)
	DeleteUser(ctx context.Context, req idl.DeleteUserRequest) (idl.DeleteUserResponse, error)
	ListUsers(ctx context.Context, req idl.ListUsersRequest) (idl.ListUsersResponse, error)
}

// ─────────────────────────── Demo logic ───────────────────────────

func runDemo(client CatalogServiceClient, logger *log.Logger) {
	ctx := context.Background()

	logger.Println(">>> CreateUser")
	createuserResp, err := client.CreateUser(ctx, idl.CreateUserRequest{})
	if err != nil {
		logger.Printf("    FAIL: %v", err)
	} else {
		logger.Printf("    OK  : %+v", createuserResp)
	}

	logger.Println(">>> GetUser")
	getuserResp, err := client.GetUser(ctx, idl.GetUserRequest{})
	if err != nil {
		logger.Printf("    FAIL: %v", err)
	} else {
		logger.Printf("    OK  : %+v", getuserResp)
	}

	logger.Println(">>> UpdateUser")
	updateuserResp, err := client.UpdateUser(ctx, idl.UpdateUserRequest{})
	if err != nil {
		logger.Printf("    FAIL: %v", err)
	} else {
		logger.Printf("    OK  : %+v", updateuserResp)
	}

	logger.Println(">>> DeleteUser")
	deleteuserResp, err := client.DeleteUser(ctx, idl.DeleteUserRequest{})
	if err != nil {
		logger.Printf("    FAIL: %v", err)
	} else {
		logger.Printf("    OK  : %+v", deleteuserResp)
	}

	logger.Println(">>> ListUsers")
	listusersResp, err := client.ListUsers(ctx, idl.ListUsersRequest{})
	if err != nil {
		logger.Printf("    FAIL: %v", err)
	} else {
		logger.Printf("    OK  : %+v", listusersResp)
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
		logger.Printf("=== CatalogService HTTP Client Demo  addr=%s ===", *httpAddr)
		runDemo(NewCatalogServiceHTTPClient(*httpAddr), logger)
	default:
		logger.Fatalf("unknown mode %q, use -mode=http", *mode)
	}

	logger.Println("=== Demo completed ===")
}
