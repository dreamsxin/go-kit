package catalogservice

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	idl "example.com/gen_fromdb_sqlite"
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
	LogLevel      string        `json:"log_level"`
	Timeout       time.Duration `json:"timeout"`
	EnableLogging bool          `json:"enable_logging"`
	EnableMetrics bool          `json:"enable_metrics"`
}

var defaultConfig = &ServiceConfig{
	LogLevel:      "info",
	Timeout:       30 * time.Second,
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
	var svc CatalogService = &serviceImpl{
		config: cfg,
		logger: log.Default(),
		repos:  repos,
	}

	if cfg.EnableLogging {
		svc = LoggingMiddleware(log.Default())(svc)
	}
	if cfg.EnableMetrics {
		svc = MetricsMiddleware()(svc)
	}

	return svc
}


type serviceImpl struct {
	config *ServiceConfig
	logger *log.Logger
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


func errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

type ServiceMiddleware func(CatalogService) CatalogService

func LoggingMiddleware(logger *log.Logger) ServiceMiddleware {
	return func(next CatalogService) CatalogService {
		return &loggingMiddleware{next: next, logger: logger}
	}
}

type loggingMiddleware struct {
	next   CatalogService
	logger *log.Logger
}


func (m *loggingMiddleware) CreateUser(ctx context.Context, req idl.CreateUserRequest) (resp idl.CreateUserResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Printf("[CatalogService] CreateUser err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Printf("[CatalogService] CreateUser elapsed=%v", time.Since(start))
		}
	}()
	return m.next.CreateUser(ctx, req)
}

func (m *loggingMiddleware) GetUser(ctx context.Context, req idl.GetUserRequest) (resp idl.GetUserResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Printf("[CatalogService] GetUser err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Printf("[CatalogService] GetUser elapsed=%v", time.Since(start))
		}
	}()
	return m.next.GetUser(ctx, req)
}

func (m *loggingMiddleware) UpdateUser(ctx context.Context, req idl.UpdateUserRequest) (resp idl.UpdateUserResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Printf("[CatalogService] UpdateUser err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Printf("[CatalogService] UpdateUser elapsed=%v", time.Since(start))
		}
	}()
	return m.next.UpdateUser(ctx, req)
}

func (m *loggingMiddleware) DeleteUser(ctx context.Context, req idl.DeleteUserRequest) (resp idl.DeleteUserResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Printf("[CatalogService] DeleteUser err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Printf("[CatalogService] DeleteUser elapsed=%v", time.Since(start))
		}
	}()
	return m.next.DeleteUser(ctx, req)
}

func (m *loggingMiddleware) ListUsers(ctx context.Context, req idl.ListUsersRequest) (resp idl.ListUsersResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Printf("[CatalogService] ListUsers err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Printf("[CatalogService] ListUsers elapsed=%v", time.Since(start))
		}
	}()
	return m.next.ListUsers(ctx, req)
}


func MetricsMiddleware() ServiceMiddleware {
	return func(next CatalogService) CatalogService {
		return &metricsMiddleware{next: next}
	}
}

type metricsMiddleware struct {
	next CatalogService
}


func (m *metricsMiddleware) CreateUser(ctx context.Context, req idl.CreateUserRequest) (idl.CreateUserResponse, error) {
	return m.next.CreateUser(ctx, req)
}

func (m *metricsMiddleware) GetUser(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error) {
	return m.next.GetUser(ctx, req)
}

func (m *metricsMiddleware) UpdateUser(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error) {
	return m.next.UpdateUser(ctx, req)
}

func (m *metricsMiddleware) DeleteUser(ctx context.Context, req idl.DeleteUserRequest) (idl.DeleteUserResponse, error) {
	return m.next.DeleteUser(ctx, req)
}

func (m *metricsMiddleware) ListUsers(ctx context.Context, req idl.ListUsersRequest) (idl.ListUsersResponse, error) {
	return m.next.ListUsers(ctx, req)
}

