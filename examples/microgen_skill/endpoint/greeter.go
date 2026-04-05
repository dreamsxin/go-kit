package endpoint

import (
	"context"
	"time"

	"github.com/sony/gobreaker"
	"golang.org/x/time/rate"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/endpoint/circuitbreaker"
	"github.com/dreamsxin/go-kit/endpoint/ratelimit"
	kitlog "github.com/dreamsxin/go-kit/log"
	idl "github.com/dreamsxin/go-kit/examples/microgen_skill/pb"
	"github.com/dreamsxin/go-kit/examples/microgen_skill/service"
)

// Endpoints 封装所有服务端点
type GreeterEndpoints struct {
	SayHelloEndpoint endpoint.Endpoint
	GetStatusEndpoint endpoint.Endpoint

}

// MiddlewareConfig 端点中间件配置，与 config/config.go 中的 MiddlewareConfig 对应
type MiddlewareConfig struct {
	CBEnabled          bool
	CBFailureThreshold uint32
	CBTimeout          time.Duration
	RLEnabled          bool
	RLRps              float64
	Timeout            time.Duration
}

// DefaultMiddlewareConfig 默认中间件配置
var DefaultMiddlewareConfig = MiddlewareConfig{
	CBEnabled:          true,
	CBFailureThreshold: 5,
	CBTimeout:          60 * time.Second,
	RLEnabled:          true,
	RLRps:              100,
	Timeout:            30 * time.Second,
}

// MakeServerEndpoints 使用默认中间件配置创建服务端端点
func MakeServerEndpoints(svc service.Greeter, logger *kitlog.Logger) GreeterEndpoints {
	return MakeServerEndpointsWithConfig(svc, logger, DefaultMiddlewareConfig)
}

// MakeServerEndpointsWithConfig 使用自定义中间件配置创建服务端端点
func MakeServerEndpointsWithConfig(
	svc service.Greeter,
	logger *kitlog.Logger,
	cfg MiddlewareConfig,
) GreeterEndpoints {
	var cbMiddleware endpoint.Middleware
	if cfg.CBEnabled {
		cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name: "Greeter",
			ReadyToTrip: func(c gobreaker.Counts) bool {
				return c.ConsecutiveFailures >= cfg.CBFailureThreshold
			},
			Timeout: cfg.CBTimeout,
		})
		cbMiddleware = circuitbreaker.Gobreaker(cb)
	}

	var rlMiddleware endpoint.Middleware
	if cfg.RLEnabled && cfg.RLRps > 0 {
		lim := rate.NewLimiter(rate.Limit(cfg.RLRps), int(cfg.RLRps))
		rlMiddleware = ratelimit.NewErroringLimiter(lim)
	}

	build := func(ep endpoint.Endpoint, name string) endpoint.Endpoint {
		b := endpoint.NewBuilder(ep).
			WithLogging(logger, name).
			WithTimeout(cfg.Timeout)
		if cbMiddleware != nil {
			b = b.Use(cbMiddleware)
		}
		if rlMiddleware != nil {
			b = b.Use(rlMiddleware)
		}
		return b.Build()
	}

	return GreeterEndpoints{
		SayHelloEndpoint: build(MakeSayHelloEndpoint(svc), "SayHello"),
		GetStatusEndpoint: build(MakeGetStatusEndpoint(svc), "GetStatus"),
	}
}


// MakeSayHelloEndpoint 创建单个端点
func MakeSayHelloEndpoint(svc service.Greeter) endpoint.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(idl.HelloRequest)
		resp, err := svc.SayHello(ctx, req)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}

// MakeGetStatusEndpoint 创建单个端点
func MakeGetStatusEndpoint(svc service.Greeter) endpoint.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(idl.Empty)
		resp, err := svc.GetStatus(ctx, req)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}


// 客户端方法

// SayHello 调用服务端端点
func (e GreeterEndpoints) SayHello(ctx context.Context, req idl.HelloRequest) (idl.HelloResponse, error) {
	resp, err := e.SayHelloEndpoint(ctx, req)
	if err != nil {
		return idl.HelloResponse{}, err
	}
	return resp.(idl.HelloResponse), nil
}

// GetStatus 调用服务端端点
func (e GreeterEndpoints) GetStatus(ctx context.Context, req idl.Empty) (idl.StatusResponse, error) {
	resp, err := e.GetStatusEndpoint(ctx, req)
	if err != nil {
		return idl.StatusResponse{}, err
	}
	return resp.(idl.StatusResponse), nil
}


// RetryMiddleware 重试中间件。
// ⚠️  仅适用于【客户端】endpoint（如 HTTP/gRPC client）。
func RetryMiddleware(maxRetries int, backoff time.Duration) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (response any, err error) {
			for i := 0; i < maxRetries; i++ {
				response, err = next(ctx, request)
				if err == nil {
					return response, nil
				}
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(backoff * time.Duration(i+1)):
				}
			}
			return nil, err
		}
	}
}
