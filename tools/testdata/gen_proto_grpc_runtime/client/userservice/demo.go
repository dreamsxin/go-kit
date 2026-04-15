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

	idl "example.com/gen_proto_grpc_runtime/pb"
	genTransport "example.com/gen_proto_grpc_runtime/transport/userservice"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ─────────────────────────── HTTP Client ───────────────────────────

// UserServiceHTTPClient UserService HTTP 客户端
type UserServiceHTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewUserServiceHTTPClient 创建 HTTP 客户端，baseURL 如 "http://localhost:8080"
func NewUserServiceHTTPClient(baseURL string) *UserServiceHTTPClient {
	return &UserServiceHTTPClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *UserServiceHTTPClient) do(ctx context.Context, path string, req, resp interface{}) error {
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


// GetUser 通过 HTTP 调用 GetUser
func (c *UserServiceHTTPClient) GetUser(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error) {
	var resp idl.GetUserResponse
	return resp, c.do(ctx, "/getuser", req, &resp)
}

// CreateUser 通过 HTTP 调用 CreateUser
func (c *UserServiceHTTPClient) CreateUser(ctx context.Context, req idl.CreateUserRequest) (idl.CreateUserResponse, error) {
	var resp idl.CreateUserResponse
	return resp, c.do(ctx, "/createuser", req, &resp)
}

// ─────────────────────────── gRPC Client ───────────────────────────

// UserServiceGRPCClient gRPC 客户端
type UserServiceGRPCClient struct {
	conn *grpc.ClientConn
	getuser func(ctx context.Context, request interface{}) (interface{}, error)
	createuser func(ctx context.Context, request interface{}) (interface{}, error)

}

// NewUserServiceGRPCClient 创建 gRPC 客户端，addr 格式如 "localhost:8081"
func NewUserServiceGRPCClient(addr string, opts ...grpc.DialOption) (*UserServiceGRPCClient, error) {
	if len(opts) == 0 {
		opts = []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithTimeout(5 * time.Second), //nolint:staticcheck
		}
	}
	conn, err := grpc.Dial(addr, opts...) //nolint:staticcheck
	if err != nil {
		return nil, fmt.Errorf("grpc dial %s: %w", addr, err)
	}
	return &UserServiceGRPCClient{
		conn: conn,
		getuser: genTransport.NewGRPCGetUserClient(conn),
		createuser: genTransport.NewGRPCCreateUserClient(conn),

	}, nil
}

// Close 关闭 gRPC 连接
func (c *UserServiceGRPCClient) Close() error {
	return c.conn.Close()
}


// GetUser 通过 gRPC 调用 GetUser
func (c *UserServiceGRPCClient) GetUser(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error) {
	resp, err := c.getuser(ctx, req)
	if err != nil {
		return idl.GetUserResponse{}, err
	}
	return resp.(idl.GetUserResponse), nil
}

// CreateUser 通过 gRPC 调用 CreateUser
func (c *UserServiceGRPCClient) CreateUser(ctx context.Context, req idl.CreateUserRequest) (idl.CreateUserResponse, error) {
	resp, err := c.createuser(ctx, req)
	if err != nil {
		return idl.CreateUserResponse{}, err
	}
	return resp.(idl.CreateUserResponse), nil
}


// ─────────────────────────── 通用接口 ───────────────────────────

// UserServiceClient 统一客户端接口（HTTP 和 gRPC 均实现该接口）
type UserServiceClient interface {
	GetUser(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error)
	CreateUser(ctx context.Context, req idl.CreateUserRequest) (idl.CreateUserResponse, error)

}

// ─────────────────────────── Demo logic ───────────────────────────

func runDemo(client UserServiceClient, logger *log.Logger) {
	ctx := context.Background()

	logger.Println(">>> GetUser")
	getuserResp, err := client.GetUser(ctx, idl.GetUserRequest{})
	if err != nil {
		logger.Printf("    FAIL: %v", err)
	} else {
		logger.Printf("    OK  : %+v", getuserResp)
	}

	logger.Println(">>> CreateUser")
	createuserResp, err := client.CreateUser(ctx, idl.CreateUserRequest{})
	if err != nil {
		logger.Printf("    FAIL: %v", err)
	} else {
		logger.Printf("    OK  : %+v", createuserResp)
	}

}

// ─────────────────────────── main ───────────────────────────

func main() {
	var (
		mode     = flag.String("mode", "grpc", "client mode: http or grpc")
		grpcAddr = flag.String("grpc.addr", "localhost:8081", "gRPC server address (mode=grpc)")
		httpAddr = flag.String("http.addr", "http://localhost:8080", "HTTP server address (mode=http)")
	)
	flag.Parse()

	logger := log.New(log.Writer(), "[demo] ", log.LstdFlags)

	switch *mode {
	case "grpc":
		logger.Printf("=== UserService gRPC Client Demo  addr=%s ===", *grpcAddr)
		client, err := NewUserServiceGRPCClient(*grpcAddr)
		if err != nil {
			logger.Fatalf("FATAL: dial grpc: %v", err)
		}
		defer client.Close()
		runDemo(client, logger)
	case "http":
		logger.Printf("=== UserService HTTP Client Demo  addr=%s ===", *httpAddr)
		runDemo(NewUserServiceHTTPClient(*httpAddr), logger)
	default:
		logger.Fatalf("unknown mode %q, use -mode=grpc or -mode=http", *mode)
	}

	logger.Println("=== Demo completed ===")
}
