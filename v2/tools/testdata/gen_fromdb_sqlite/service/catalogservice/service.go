package catalogservice

import (
	"context"
	"errors"
	"time"

	idl "example.com/gen_fromdb_sqlite"
	kitlog "github.com/dreamsxin/go-kit/v2/log"
)

// CatalogService defines the business contract.
type CatalogService interface {

	// CreateUser - Create User
	CreateUser(ctx context.Context, req idl.CreateUserRequest) (idl.CreateUserResponse, error)

	// GetUser - Get User details
	GetUser(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error)

	// UpdateUser - Update User
	UpdateUser(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error)

	// DeleteUser - Delete User
	DeleteUser(ctx context.Context, req idl.DeleteUserRequest) (idl.DeleteUserResponse, error)

	// ListUsers - List Users
	ListUsers(ctx context.Context, req idl.ListUsersRequest) (idl.ListUsersResponse, error)
}

// ServiceConfig controls generated service behavior.
type ServiceConfig struct {
	EnableLogging bool           `json:"enable_logging"`
	Logger        *kitlog.Logger `json:"-"`
}

var defaultConfig = &ServiceConfig{
	EnableLogging: true,
}

// NewService creates a service without repository dependencies.
func NewService(cfg *ServiceConfig) CatalogService {
	return NewServiceWithRepo(cfg, GeneratedRepos{})
}

// NewServiceWithRepo creates a service with repository dependencies.
func NewServiceWithRepo(cfg *ServiceConfig, repos GeneratedRepos) CatalogService {
	if cfg == nil {
		cfg = defaultConfig
	}
	return newServiceImpl(cfg, repos)
}

func newServiceImpl(cfg *ServiceConfig, repos GeneratedRepos) CatalogService {
	logger := cfg.Logger
	if logger == nil {
		logger = kitlog.NewNopLogger()
	}
	var svc CatalogService = &serviceImpl{
		config: cfg,
		logger: logger,
		repos:  repos,
	}

	if cfg.EnableLogging {
		svc = LoggingMiddleware(logger)(svc)
	}
	return svc
}

type serviceImpl struct {
	config *ServiceConfig
	logger *kitlog.Logger
	repos  GeneratedRepos
}

func (s *serviceImpl) CreateUser(ctx context.Context, req idl.CreateUserRequest) (idl.CreateUserResponse, error) {
	_ = req
	return idl.CreateUserResponse{}, errors.New("CreateUser: not implemented")
}

func (s *serviceImpl) GetUser(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error) {
	_ = req
	return idl.GetUserResponse{}, errors.New("GetUser: not implemented")
}

func (s *serviceImpl) UpdateUser(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error) {
	_ = req
	return idl.UpdateUserResponse{}, errors.New("UpdateUser: not implemented")
}

func (s *serviceImpl) DeleteUser(ctx context.Context, req idl.DeleteUserRequest) (idl.DeleteUserResponse, error) {
	_ = req
	return idl.DeleteUserResponse{}, errors.New("DeleteUser: not implemented")
}

func (s *serviceImpl) ListUsers(ctx context.Context, req idl.ListUsersRequest) (idl.ListUsersResponse, error) {
	_ = req
	return idl.ListUsersResponse{}, errors.New("ListUsers: not implemented")
}

type ServiceMiddleware func(CatalogService) CatalogService

func LoggingMiddleware(logger *kitlog.Logger) ServiceMiddleware {
	return func(next CatalogService) CatalogService {
		return &loggingMiddleware{next: next, logger: logger}
	}
}

type loggingMiddleware struct {
	next   CatalogService
	logger *kitlog.Logger
}

func (m *loggingMiddleware) CreateUser(ctx context.Context, req idl.CreateUserRequest) (resp idl.CreateUserResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Sugar().Infof("[CatalogService] CreateUser err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Sugar().Infof("[CatalogService] CreateUser elapsed=%v", time.Since(start))
		}
	}()
	return m.next.CreateUser(ctx, req)
}

func (m *loggingMiddleware) GetUser(ctx context.Context, req idl.GetUserRequest) (resp idl.GetUserResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Sugar().Infof("[CatalogService] GetUser err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Sugar().Infof("[CatalogService] GetUser elapsed=%v", time.Since(start))
		}
	}()
	return m.next.GetUser(ctx, req)
}

func (m *loggingMiddleware) UpdateUser(ctx context.Context, req idl.UpdateUserRequest) (resp idl.UpdateUserResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Sugar().Infof("[CatalogService] UpdateUser err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Sugar().Infof("[CatalogService] UpdateUser elapsed=%v", time.Since(start))
		}
	}()
	return m.next.UpdateUser(ctx, req)
}

func (m *loggingMiddleware) DeleteUser(ctx context.Context, req idl.DeleteUserRequest) (resp idl.DeleteUserResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Sugar().Infof("[CatalogService] DeleteUser err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Sugar().Infof("[CatalogService] DeleteUser elapsed=%v", time.Since(start))
		}
	}()
	return m.next.DeleteUser(ctx, req)
}

func (m *loggingMiddleware) ListUsers(ctx context.Context, req idl.ListUsersRequest) (resp idl.ListUsersResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Sugar().Infof("[CatalogService] ListUsers err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Sugar().Infof("[CatalogService] ListUsers elapsed=%v", time.Since(start))
		}
	}()
	return m.next.ListUsers(ctx, req)
}
