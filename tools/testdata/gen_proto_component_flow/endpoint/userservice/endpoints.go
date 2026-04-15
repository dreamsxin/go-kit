package userservice

import (
	"context"
	"time"

	"github.com/sony/gobreaker"
	"golang.org/x/time/rate"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/endpoint/circuitbreaker"
	"github.com/dreamsxin/go-kit/endpoint/ratelimit"
	kitlog "github.com/dreamsxin/go-kit/log"
	idl "example.com/gen_proto_component_flow/pb"
	svc "example.com/gen_proto_component_flow/service/userservice"
)

// UserServiceEndpoints groups the generated endpoints.
type UserServiceEndpoints struct {
	GetUserEndpoint endpoint.Endpoint
	CreateUserEndpoint endpoint.Endpoint

}

type MiddlewareConfig struct {
	CBEnabled          bool
	CBFailureThreshold uint32
	CBTimeout          time.Duration
	RLEnabled          bool
	RLRps              float64
	Timeout            time.Duration
}

var DefaultMiddlewareConfig = MiddlewareConfig{
	CBEnabled:          true,
	CBFailureThreshold: 5,
	CBTimeout:          60 * time.Second,
	RLEnabled:          true,
	RLRps:              100,
	Timeout:            30 * time.Second,
}

func MakeServerEndpoints(s svc.UserService, logger *kitlog.Logger) UserServiceEndpoints {
	return MakeServerEndpointsWithConfig(s, logger, DefaultMiddlewareConfig)
}

func MakeServerEndpointsWithConfig(
	s svc.UserService,
	logger *kitlog.Logger,
	cfg MiddlewareConfig,
) UserServiceEndpoints {
	var cbMiddleware endpoint.Middleware
	if cfg.CBEnabled {
		cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name: "UserService",
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

	return UserServiceEndpoints{
		GetUserEndpoint: build(MakeGetUserEndpoint(s), "GetUser"),
		CreateUserEndpoint: build(MakeCreateUserEndpoint(s), "CreateUser"),
	}
}


func MakeGetUserEndpoint(s svc.UserService) endpoint.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(idl.GetUserRequest)
		resp, err := s.GetUser(ctx, req)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}

func MakeCreateUserEndpoint(s svc.UserService) endpoint.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(idl.CreateUserRequest)
		resp, err := s.CreateUser(ctx, req)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}



func (e UserServiceEndpoints) GetUser(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error) {
	resp, err := e.GetUserEndpoint(ctx, req)
	if err != nil {
		return idl.GetUserResponse{}, err
	}
	return resp.(idl.GetUserResponse), nil
}

func (e UserServiceEndpoints) CreateUser(ctx context.Context, req idl.CreateUserRequest) (idl.CreateUserResponse, error) {
	resp, err := e.CreateUserEndpoint(ctx, req)
	if err != nil {
		return idl.CreateUserResponse{}, err
	}
	return resp.(idl.CreateUserResponse), nil
}


// RetryMiddleware is intended for client-side endpoint usage.
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
