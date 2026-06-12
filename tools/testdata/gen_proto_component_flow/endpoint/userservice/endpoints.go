package userservice

import (
	"context"
	"time"

	"github.com/dreamsxin/go-kit/endpoint"
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
	build := func(ep endpoint.Endpoint, name string) endpoint.Endpoint {
		ep = applyGeneratedMiddleware(ep, logger, cfg, name)
		return applyCustomMiddleware(ep, logger, cfg, name)
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
