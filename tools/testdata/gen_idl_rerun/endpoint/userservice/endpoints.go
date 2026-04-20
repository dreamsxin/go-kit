package userservice

import (
	"context"
	"time"

	"github.com/dreamsxin/go-kit/endpoint"
	kitlog "github.com/dreamsxin/go-kit/log"
	idl "example.com/gen_idl_rerun"
	svc "example.com/gen_idl_rerun/service/userservice"
)

// UserServiceEndpoints groups the generated endpoints.
type UserServiceEndpoints struct {
	CreateUserEndpoint endpoint.Endpoint
	GetUserEndpoint endpoint.Endpoint
	ListUsersEndpoint endpoint.Endpoint
	DeleteUserEndpoint endpoint.Endpoint
	UpdateUserEndpoint endpoint.Endpoint
	FindByEmailEndpoint endpoint.Endpoint
	SearchUsersEndpoint endpoint.Endpoint
	QueryStatsEndpoint endpoint.Endpoint
	RemoveExpiredEndpoint endpoint.Endpoint
	EditProfileEndpoint endpoint.Endpoint
	ModifyEmailEndpoint endpoint.Endpoint
	PatchStatusEndpoint endpoint.Endpoint

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
		CreateUserEndpoint: build(MakeCreateUserEndpoint(s), "CreateUser"),
		GetUserEndpoint: build(MakeGetUserEndpoint(s), "GetUser"),
		ListUsersEndpoint: build(MakeListUsersEndpoint(s), "ListUsers"),
		DeleteUserEndpoint: build(MakeDeleteUserEndpoint(s), "DeleteUser"),
		UpdateUserEndpoint: build(MakeUpdateUserEndpoint(s), "UpdateUser"),
		FindByEmailEndpoint: build(MakeFindByEmailEndpoint(s), "FindByEmail"),
		SearchUsersEndpoint: build(MakeSearchUsersEndpoint(s), "SearchUsers"),
		QueryStatsEndpoint: build(MakeQueryStatsEndpoint(s), "QueryStats"),
		RemoveExpiredEndpoint: build(MakeRemoveExpiredEndpoint(s), "RemoveExpired"),
		EditProfileEndpoint: build(MakeEditProfileEndpoint(s), "EditProfile"),
		ModifyEmailEndpoint: build(MakeModifyEmailEndpoint(s), "ModifyEmail"),
		PatchStatusEndpoint: build(MakePatchStatusEndpoint(s), "PatchStatus"),
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

func MakeListUsersEndpoint(s svc.UserService) endpoint.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(idl.ListUsersRequest)
		resp, err := s.ListUsers(ctx, req)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}

func MakeDeleteUserEndpoint(s svc.UserService) endpoint.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(idl.DeleteUserRequest)
		resp, err := s.DeleteUser(ctx, req)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}

func MakeUpdateUserEndpoint(s svc.UserService) endpoint.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(idl.UpdateUserRequest)
		resp, err := s.UpdateUser(ctx, req)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}

func MakeFindByEmailEndpoint(s svc.UserService) endpoint.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(idl.GetUserRequest)
		resp, err := s.FindByEmail(ctx, req)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}

func MakeSearchUsersEndpoint(s svc.UserService) endpoint.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(idl.ListUsersRequest)
		resp, err := s.SearchUsers(ctx, req)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}

func MakeQueryStatsEndpoint(s svc.UserService) endpoint.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(idl.GetUserRequest)
		resp, err := s.QueryStats(ctx, req)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}

func MakeRemoveExpiredEndpoint(s svc.UserService) endpoint.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(idl.DeleteUserRequest)
		resp, err := s.RemoveExpired(ctx, req)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}

func MakeEditProfileEndpoint(s svc.UserService) endpoint.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(idl.UpdateUserRequest)
		resp, err := s.EditProfile(ctx, req)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}

func MakeModifyEmailEndpoint(s svc.UserService) endpoint.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(idl.UpdateUserRequest)
		resp, err := s.ModifyEmail(ctx, req)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}

func MakePatchStatusEndpoint(s svc.UserService) endpoint.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(idl.UpdateUserRequest)
		resp, err := s.PatchStatus(ctx, req)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}



func (e UserServiceEndpoints) CreateUser(ctx context.Context, req idl.CreateUserRequest) (idl.CreateUserResponse, error) {
	resp, err := e.CreateUserEndpoint(ctx, req)
	if err != nil {
		return idl.CreateUserResponse{}, err
	}
	return resp.(idl.CreateUserResponse), nil
}

func (e UserServiceEndpoints) GetUser(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error) {
	resp, err := e.GetUserEndpoint(ctx, req)
	if err != nil {
		return idl.GetUserResponse{}, err
	}
	return resp.(idl.GetUserResponse), nil
}

func (e UserServiceEndpoints) ListUsers(ctx context.Context, req idl.ListUsersRequest) (idl.ListUsersResponse, error) {
	resp, err := e.ListUsersEndpoint(ctx, req)
	if err != nil {
		return idl.ListUsersResponse{}, err
	}
	return resp.(idl.ListUsersResponse), nil
}

func (e UserServiceEndpoints) DeleteUser(ctx context.Context, req idl.DeleteUserRequest) (idl.DeleteUserResponse, error) {
	resp, err := e.DeleteUserEndpoint(ctx, req)
	if err != nil {
		return idl.DeleteUserResponse{}, err
	}
	return resp.(idl.DeleteUserResponse), nil
}

func (e UserServiceEndpoints) UpdateUser(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error) {
	resp, err := e.UpdateUserEndpoint(ctx, req)
	if err != nil {
		return idl.UpdateUserResponse{}, err
	}
	return resp.(idl.UpdateUserResponse), nil
}

func (e UserServiceEndpoints) FindByEmail(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error) {
	resp, err := e.FindByEmailEndpoint(ctx, req)
	if err != nil {
		return idl.GetUserResponse{}, err
	}
	return resp.(idl.GetUserResponse), nil
}

func (e UserServiceEndpoints) SearchUsers(ctx context.Context, req idl.ListUsersRequest) (idl.ListUsersResponse, error) {
	resp, err := e.SearchUsersEndpoint(ctx, req)
	if err != nil {
		return idl.ListUsersResponse{}, err
	}
	return resp.(idl.ListUsersResponse), nil
}

func (e UserServiceEndpoints) QueryStats(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error) {
	resp, err := e.QueryStatsEndpoint(ctx, req)
	if err != nil {
		return idl.GetUserResponse{}, err
	}
	return resp.(idl.GetUserResponse), nil
}

func (e UserServiceEndpoints) RemoveExpired(ctx context.Context, req idl.DeleteUserRequest) (idl.DeleteUserResponse, error) {
	resp, err := e.RemoveExpiredEndpoint(ctx, req)
	if err != nil {
		return idl.DeleteUserResponse{}, err
	}
	return resp.(idl.DeleteUserResponse), nil
}

func (e UserServiceEndpoints) EditProfile(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error) {
	resp, err := e.EditProfileEndpoint(ctx, req)
	if err != nil {
		return idl.UpdateUserResponse{}, err
	}
	return resp.(idl.UpdateUserResponse), nil
}

func (e UserServiceEndpoints) ModifyEmail(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error) {
	resp, err := e.ModifyEmailEndpoint(ctx, req)
	if err != nil {
		return idl.UpdateUserResponse{}, err
	}
	return resp.(idl.UpdateUserResponse), nil
}

func (e UserServiceEndpoints) PatchStatus(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error) {
	resp, err := e.PatchStatusEndpoint(ctx, req)
	if err != nil {
		return idl.UpdateUserResponse{}, err
	}
	return resp.(idl.UpdateUserResponse), nil
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
