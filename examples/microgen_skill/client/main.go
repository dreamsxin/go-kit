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

	idl "github.com/dreamsxin/go-kit/examples/microgen_skill/pb"
	genTransport "github.com/dreamsxin/go-kit/examples/microgen_skill/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ─────────────────────────── HTTP Client ───────────────────────────

// GreeterHTTPClient Greeter HTTP 客户端
type GreeterHTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewGreeterHTTPClient 创建 HTTP 客户端，baseURL 如 "http://localhost:8080"
func NewGreeterHTTPClient(baseURL string) *GreeterHTTPClient {
	return &GreeterHTTPClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *GreeterHTTPClient) do(ctx context.Context, path string, req, resp interface{}) error {
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


// SayHello 通过 HTTP 调用 SayHello
func (c *GreeterHTTPClient) SayHello(ctx context.Context, req idl.HelloRequest) (idl.HelloResponse, error) {
	var resp idl.HelloResponse
	return resp, c.do(ctx, "<no value>/sayhello", req, &resp)
}

// GetStatus 通过 HTTP 调用 GetStatus
func (c *GreeterHTTPClient) GetStatus(ctx context.Context, req idl.Empty) (idl.StatusResponse, error) {
	var resp idl.StatusResponse
	return resp, c.do(ctx, "<no value>/getstatus", req, &resp)
}

// ─────────────────────────── gRPC Client ───────────────────────────

// GreeterGRPCClient gRPC 客户端
type GreeterGRPCClient struct {
	conn *grpc.ClientConn
	sayhello func(ctx context.Context, request interface{}) (interface{}, error)
	getstatus func(ctx context.Context, request interface{}) (interface{}, error)

}

// NewGreeterGRPCClient 创建 gRPC 客户端，addr 格式如 "localhost:8081"
func NewGreeterGRPCClient(addr string, opts ...grpc.DialOption) (*GreeterGRPCClient, error) {
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
	return &GreeterGRPCClient{
		conn: conn,
		sayhello: genTransport.NewGRPCSayHelloClient(conn),
		getstatus: genTransport.NewGRPCGetStatusClient(conn),

	}, nil
}

// Close 关闭 gRPC 连接
func (c *GreeterGRPCClient) Close() error {
	return c.conn.Close()
}


// SayHello 通过 gRPC 调用 SayHello
func (c *GreeterGRPCClient) SayHello(ctx context.Context, req idl.HelloRequest) (idl.HelloResponse, error) {
	resp, err := c.sayhello(ctx, req)
	if err != nil {
		return idl.HelloResponse{}, err
	}
	return resp.(idl.HelloResponse), nil
}

// GetStatus 通过 gRPC 调用 GetStatus
func (c *GreeterGRPCClient) GetStatus(ctx context.Context, req idl.Empty) (idl.StatusResponse, error) {
	resp, err := c.getstatus(ctx, req)
	if err != nil {
		return idl.StatusResponse{}, err
	}
	return resp.(idl.StatusResponse), nil
}


// ─────────────────────────── 通用接口 ───────────────────────────

// GreeterClient 统一客户端接口（HTTP 和 gRPC 均实现该接口）
type GreeterClient interface {
	SayHello(ctx context.Context, req idl.HelloRequest) (idl.HelloResponse, error)
	GetStatus(ctx context.Context, req idl.Empty) (idl.StatusResponse, error)

}

// ─────────────────────────── Demo logic ───────────────────────────

func runDemo(client GreeterClient, logger *log.Logger) {
	ctx := context.Background()

	logger.Println(">>> SayHello")
	sayhelloResp, err := client.SayHello(ctx, idl.HelloRequest{})
	if err != nil {
		logger.Printf("    FAIL: %v", err)
	} else {
		logger.Printf("    OK  : %+v", sayhelloResp)
	}

	logger.Println(">>> GetStatus")
	getstatusResp, err := client.GetStatus(ctx, idl.Empty{})
	if err != nil {
		logger.Printf("    FAIL: %v", err)
	} else {
		logger.Printf("    OK  : %+v", getstatusResp)
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
		logger.Printf("=== Greeter gRPC Client Demo  addr=%s ===", *grpcAddr)
		client, err := NewGreeterGRPCClient(*grpcAddr)
		if err != nil {
			logger.Fatalf("FATAL: dial grpc: %v", err)
		}
		defer client.Close()
		runDemo(client, logger)
	case "http":
		logger.Printf("=== Greeter HTTP Client Demo  addr=%s ===", *httpAddr)
		runDemo(NewGreeterHTTPClient(*httpAddr), logger)
	default:
		logger.Fatalf("unknown mode %q, use -mode=grpc or -mode=http", *mode)
	}

	logger.Println("=== Demo completed ===")
}
