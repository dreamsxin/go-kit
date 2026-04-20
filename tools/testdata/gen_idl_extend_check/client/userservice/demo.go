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

	idl "example.com/gen_idl_extend_check"
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


// CreateUser 通过 HTTP 调用 CreateUser
func (c *UserServiceHTTPClient) CreateUser(ctx context.Context, req idl.CreateUserRequest) (idl.CreateUserResponse, error) {
	var resp idl.CreateUserResponse
	return resp, c.do(ctx, "/createuser", req, &resp)
}

// GetUser 通过 HTTP 调用 GetUser
func (c *UserServiceHTTPClient) GetUser(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error) {
	var resp idl.GetUserResponse
	return resp, c.do(ctx, "/getuser", req, &resp)
}

// ListUsers 通过 HTTP 调用 ListUsers
func (c *UserServiceHTTPClient) ListUsers(ctx context.Context, req idl.ListUsersRequest) (idl.ListUsersResponse, error) {
	var resp idl.ListUsersResponse
	return resp, c.do(ctx, "/listusers", req, &resp)
}

// DeleteUser 通过 HTTP 调用 DeleteUser
func (c *UserServiceHTTPClient) DeleteUser(ctx context.Context, req idl.DeleteUserRequest) (idl.DeleteUserResponse, error) {
	var resp idl.DeleteUserResponse
	return resp, c.do(ctx, "/deleteuser", req, &resp)
}

// UpdateUser 通过 HTTP 调用 UpdateUser
func (c *UserServiceHTTPClient) UpdateUser(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error) {
	var resp idl.UpdateUserResponse
	return resp, c.do(ctx, "/updateuser", req, &resp)
}

// FindByEmail 通过 HTTP 调用 FindByEmail
func (c *UserServiceHTTPClient) FindByEmail(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error) {
	var resp idl.GetUserResponse
	return resp, c.do(ctx, "/findbyemail", req, &resp)
}

// SearchUsers 通过 HTTP 调用 SearchUsers
func (c *UserServiceHTTPClient) SearchUsers(ctx context.Context, req idl.ListUsersRequest) (idl.ListUsersResponse, error) {
	var resp idl.ListUsersResponse
	return resp, c.do(ctx, "/searchusers", req, &resp)
}

// QueryStats 通过 HTTP 调用 QueryStats
func (c *UserServiceHTTPClient) QueryStats(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error) {
	var resp idl.GetUserResponse
	return resp, c.do(ctx, "/querystats", req, &resp)
}

// RemoveExpired 通过 HTTP 调用 RemoveExpired
func (c *UserServiceHTTPClient) RemoveExpired(ctx context.Context, req idl.DeleteUserRequest) (idl.DeleteUserResponse, error) {
	var resp idl.DeleteUserResponse
	return resp, c.do(ctx, "/removeexpired", req, &resp)
}

// EditProfile 通过 HTTP 调用 EditProfile
func (c *UserServiceHTTPClient) EditProfile(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error) {
	var resp idl.UpdateUserResponse
	return resp, c.do(ctx, "/editprofile", req, &resp)
}

// ModifyEmail 通过 HTTP 调用 ModifyEmail
func (c *UserServiceHTTPClient) ModifyEmail(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error) {
	var resp idl.UpdateUserResponse
	return resp, c.do(ctx, "/modifyemail", req, &resp)
}

// PatchStatus 通过 HTTP 调用 PatchStatus
func (c *UserServiceHTTPClient) PatchStatus(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error) {
	var resp idl.UpdateUserResponse
	return resp, c.do(ctx, "/patchstatus", req, &resp)
}


// ─────────────────────────── 通用接口 ───────────────────────────

// UserServiceClient 统一客户端接口（HTTP 和 gRPC 均实现该接口）
type UserServiceClient interface {
	CreateUser(ctx context.Context, req idl.CreateUserRequest) (idl.CreateUserResponse, error)
	GetUser(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error)
	ListUsers(ctx context.Context, req idl.ListUsersRequest) (idl.ListUsersResponse, error)
	DeleteUser(ctx context.Context, req idl.DeleteUserRequest) (idl.DeleteUserResponse, error)
	UpdateUser(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error)
	FindByEmail(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error)
	SearchUsers(ctx context.Context, req idl.ListUsersRequest) (idl.ListUsersResponse, error)
	QueryStats(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error)
	RemoveExpired(ctx context.Context, req idl.DeleteUserRequest) (idl.DeleteUserResponse, error)
	EditProfile(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error)
	ModifyEmail(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error)
	PatchStatus(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error)

}

// ─────────────────────────── Demo logic ───────────────────────────

func runDemo(client UserServiceClient, logger *log.Logger) {
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

	logger.Println(">>> ListUsers")
	listusersResp, err := client.ListUsers(ctx, idl.ListUsersRequest{})
	if err != nil {
		logger.Printf("    FAIL: %v", err)
	} else {
		logger.Printf("    OK  : %+v", listusersResp)
	}

	logger.Println(">>> DeleteUser")
	deleteuserResp, err := client.DeleteUser(ctx, idl.DeleteUserRequest{})
	if err != nil {
		logger.Printf("    FAIL: %v", err)
	} else {
		logger.Printf("    OK  : %+v", deleteuserResp)
	}

	logger.Println(">>> UpdateUser")
	updateuserResp, err := client.UpdateUser(ctx, idl.UpdateUserRequest{})
	if err != nil {
		logger.Printf("    FAIL: %v", err)
	} else {
		logger.Printf("    OK  : %+v", updateuserResp)
	}

	logger.Println(">>> FindByEmail")
	findbyemailResp, err := client.FindByEmail(ctx, idl.GetUserRequest{})
	if err != nil {
		logger.Printf("    FAIL: %v", err)
	} else {
		logger.Printf("    OK  : %+v", findbyemailResp)
	}

	logger.Println(">>> SearchUsers")
	searchusersResp, err := client.SearchUsers(ctx, idl.ListUsersRequest{})
	if err != nil {
		logger.Printf("    FAIL: %v", err)
	} else {
		logger.Printf("    OK  : %+v", searchusersResp)
	}

	logger.Println(">>> QueryStats")
	querystatsResp, err := client.QueryStats(ctx, idl.GetUserRequest{})
	if err != nil {
		logger.Printf("    FAIL: %v", err)
	} else {
		logger.Printf("    OK  : %+v", querystatsResp)
	}

	logger.Println(">>> RemoveExpired")
	removeexpiredResp, err := client.RemoveExpired(ctx, idl.DeleteUserRequest{})
	if err != nil {
		logger.Printf("    FAIL: %v", err)
	} else {
		logger.Printf("    OK  : %+v", removeexpiredResp)
	}

	logger.Println(">>> EditProfile")
	editprofileResp, err := client.EditProfile(ctx, idl.UpdateUserRequest{})
	if err != nil {
		logger.Printf("    FAIL: %v", err)
	} else {
		logger.Printf("    OK  : %+v", editprofileResp)
	}

	logger.Println(">>> ModifyEmail")
	modifyemailResp, err := client.ModifyEmail(ctx, idl.UpdateUserRequest{})
	if err != nil {
		logger.Printf("    FAIL: %v", err)
	} else {
		logger.Printf("    OK  : %+v", modifyemailResp)
	}

	logger.Println(">>> PatchStatus")
	patchstatusResp, err := client.PatchStatus(ctx, idl.UpdateUserRequest{})
	if err != nil {
		logger.Printf("    FAIL: %v", err)
	} else {
		logger.Printf("    OK  : %+v", patchstatusResp)
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
		logger.Printf("=== UserService HTTP Client Demo  addr=%s ===", *httpAddr)
		runDemo(NewUserServiceHTTPClient(*httpAddr), logger)
	default:
		logger.Fatalf("unknown mode %q, use -mode=http", *mode)
	}

	logger.Println("=== Demo completed ===")
}
