package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sony/gobreaker"
	"golang.org/x/time/rate"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/endpoint/circuitbreaker"
	"github.com/dreamsxin/go-kit/endpoint/ratelimit"
	"github.com/dreamsxin/go-kit/transport/http/server"
)

// 请求和响应结构体
type helloRequest struct {
	Name string `json:"name"`
}

type helloResponse struct {
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

// 业务端点实现
func makeHelloEndpoint() endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(helloRequest)
		if req.Name == "" {
			return nil, errors.New("name is required")
		}
		return helloResponse{
			Message: fmt.Sprintf("Hello, %s!", req.Name),
		}, nil
	}
}

// 请求解码器
func decodeRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req helloRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	return req, nil
}

// 响应编码器
func encodeResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}

// 错误编码器
func errorEncoder(_ context.Context, err error, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	var statusCode int
	switch {
	case errors.Is(err, ratelimit.ErrLimited):
		statusCode = http.StatusTooManyRequests
	default:
		statusCode = http.StatusInternalServerError
	}

	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(helloResponse{Error: err.Error()})
}

// 日志中间件
func loggingMiddleware(logger *log.Logger) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request interface{}) (interface{}, error) {
			logger.Printf("Processing request: %+v", request)
			start := time.Now()
			defer func() {
				logger.Printf("Request processed in %v", time.Since(start))
			}()
			return next(ctx, request)
		}
	}
}

func main() {
	logger := log.New(os.Stdout, "go-kit-example: ", log.LstdFlags)

	// 创建基础端点
	baseEndpoint := makeHelloEndpoint()

	// 创建熔断器
	cbSettings := gobreaker.Settings{
		Name:        "hello-endpoint",
		MaxRequests: 5,
		Interval:    10 * time.Second,
		Timeout:     5 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures > 3
		},
	}
	circuitBreaker := gobreaker.NewCircuitBreaker(cbSettings)

	// 创建限流器 - 每秒最多处理2个请求，突发最多5个请求
	rateLimiter := rate.NewLimiter(rate.Every(time.Second), 5)

	// 构建中间件链
	helloEndpoint := endpoint.Chain(
		loggingMiddleware(logger),                 // 日志
		endpoint.ErrorHandlingMiddleware("hello"), // 错误处理
		circuitbreaker.Gobreaker(circuitBreaker),  // 熔断器
		ratelimit.NewErroringLimiter(rateLimiter), // 限流器（错误模式）
	)(baseEndpoint)

	// 创建HTTP处理器
	handler := server.NewServer(
		helloEndpoint,
		decodeRequest,
		encodeResponse,
		server.ServerErrorEncoder(errorEncoder),
	)

	// 设置HTTP路由
	http.Handle("/hello", handler)

	// 启动服务器
	port := "8080"
	logger.Printf("Starting server on :%s", port)

	errs := make(chan error, 2)
	go func() {
		logger.Printf("Transport: HTTP Listening on :%s", port)
		errs <- http.ListenAndServe(":"+port, nil)
	}()

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		errs <- fmt.Errorf("%s", <-c)
	}()

	logger.Printf("Terminated: %s", <-errs)
}
