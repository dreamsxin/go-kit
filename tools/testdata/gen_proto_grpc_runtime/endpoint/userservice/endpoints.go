package userservice

import (
	"context"
	"errors"
	"time"

	"github.com/dreamsxin/go-kit/endpoint"
	kitlog "github.com/dreamsxin/go-kit/log"
	idl "example.com/gen_proto_grpc_runtime/pb"
	svc "example.com/gen_proto_grpc_runtime/service/userservice"
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
	RetryEnabled       bool
	RetryMaxAttempts   int
	RetryBackoff       time.Duration
	Timeout            time.Duration
}

var DefaultMiddlewareConfig = MiddlewareConfig{
	CBEnabled:          false,
	CBFailureThreshold: 5,
	CBTimeout:          60 * time.Second,
	RLEnabled:          true,
	RLRps:              100,
	RetryEnabled:       false,
	RetryMaxAttempts:   3,
	RetryBackoff:       2 * time.Second,
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


// RetryMiddleware retries only errors that explicitly implement
// interface{ Retryable() bool } and return true. It is safe for server-side
// endpoint chains because ordinary business errors are not retried.
func RetryMiddleware(maxAttempts int, backoff time.Duration) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (response any, err error) {
			if maxAttempts <= 1 {
				return next(ctx, request)
			}
			for i := 0; ; i++ {
				response, err = next(ctx, request)
				if err == nil || !retryableEndpointError(err) || i+1 >= maxAttempts {
					return response, err
				}
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(backoff * time.Duration(i+1)):
				}
			}
		}
	}
}

func retryableEndpointError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var retryable interface{ Retryable() bool }
	if errors.As(err, &retryable) {
		return retryable.Retryable()
	}
	return false
}
