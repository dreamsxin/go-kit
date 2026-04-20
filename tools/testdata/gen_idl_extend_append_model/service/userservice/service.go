package userservice

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	idl "example.com/gen_idl_extend_append_model"
)

// UserService defines the business contract.
type UserService interface {

	// CreateUser - CreateUser creates a new user.
	CreateUser(ctx context.Context, req idl.CreateUserRequest) (idl.CreateUserResponse, error)

	// GetUser - GetUser retrieves a user by ID.
	GetUser(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error)

	// ListUsers - ListUsers lists all users.
	ListUsers(ctx context.Context, req idl.ListUsersRequest) (idl.ListUsersResponse, error)

	// DeleteUser - DeleteUser removes a user.
	DeleteUser(ctx context.Context, req idl.DeleteUserRequest) (idl.DeleteUserResponse, error)

	// UpdateUser - UpdateUser modifies a user.
	UpdateUser(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error)

	// FindByEmail - FindByEmail finds users by email prefix.
	FindByEmail(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error)

	// SearchUsers - SearchUsers searches users.
	SearchUsers(ctx context.Context, req idl.ListUsersRequest) (idl.ListUsersResponse, error)

	// QueryStats - QueryStats returns statistics.
	QueryStats(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error)

	// RemoveExpired - RemoveExpired removes expired users.
	RemoveExpired(ctx context.Context, req idl.DeleteUserRequest) (idl.DeleteUserResponse, error)

	// EditProfile - EditProfile edits profile.
	EditProfile(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error)

	// ModifyEmail - ModifyEmail modifies email.
	ModifyEmail(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error)

	// PatchStatus - PatchStatus patches status.
	PatchStatus(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error)

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
func NewService(cfg *ServiceConfig) UserService {
	return NewServiceWithRepo(cfg, GeneratedRepos{})
}

// NewServiceWithRepo creates a service with repository dependencies.
func NewServiceWithRepo(cfg *ServiceConfig, repos GeneratedRepos) UserService {
	if cfg == nil {
		cfg = defaultConfig
	}
	return newServiceImpl(cfg, repos)
}

func newServiceImpl(cfg *ServiceConfig, repos GeneratedRepos) UserService {
	var svc UserService = &serviceImpl{
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

func (s *serviceImpl) ListUsers(ctx context.Context, req idl.ListUsersRequest) (idl.ListUsersResponse, error) {
	_ = req
	return idl.ListUsersResponse{}, errors.New("ListUsers: not implemented")
}

func (s *serviceImpl) DeleteUser(ctx context.Context, req idl.DeleteUserRequest) (idl.DeleteUserResponse, error) {
	_ = req
	return idl.DeleteUserResponse{}, errors.New("DeleteUser: not implemented")
}

func (s *serviceImpl) UpdateUser(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error) {
	_ = req
	return idl.UpdateUserResponse{}, errors.New("UpdateUser: not implemented")
}

func (s *serviceImpl) FindByEmail(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error) {
	_ = req
	return idl.GetUserResponse{}, errors.New("FindByEmail: not implemented")
}

func (s *serviceImpl) SearchUsers(ctx context.Context, req idl.ListUsersRequest) (idl.ListUsersResponse, error) {
	_ = req
	return idl.ListUsersResponse{}, errors.New("SearchUsers: not implemented")
}

func (s *serviceImpl) QueryStats(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error) {
	_ = req
	return idl.GetUserResponse{}, errors.New("QueryStats: not implemented")
}

func (s *serviceImpl) RemoveExpired(ctx context.Context, req idl.DeleteUserRequest) (idl.DeleteUserResponse, error) {
	_ = req
	return idl.DeleteUserResponse{}, errors.New("RemoveExpired: not implemented")
}

func (s *serviceImpl) EditProfile(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error) {
	_ = req
	return idl.UpdateUserResponse{}, errors.New("EditProfile: not implemented")
}

func (s *serviceImpl) ModifyEmail(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error) {
	_ = req
	return idl.UpdateUserResponse{}, errors.New("ModifyEmail: not implemented")
}

func (s *serviceImpl) PatchStatus(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error) {
	_ = req
	return idl.UpdateUserResponse{}, errors.New("PatchStatus: not implemented")
}


func errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

type ServiceMiddleware func(UserService) UserService

func LoggingMiddleware(logger *log.Logger) ServiceMiddleware {
	return func(next UserService) UserService {
		return &loggingMiddleware{next: next, logger: logger}
	}
}

type loggingMiddleware struct {
	next   UserService
	logger *log.Logger
}


func (m *loggingMiddleware) CreateUser(ctx context.Context, req idl.CreateUserRequest) (resp idl.CreateUserResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Printf("[UserService] CreateUser err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Printf("[UserService] CreateUser elapsed=%v", time.Since(start))
		}
	}()
	return m.next.CreateUser(ctx, req)
}

func (m *loggingMiddleware) GetUser(ctx context.Context, req idl.GetUserRequest) (resp idl.GetUserResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Printf("[UserService] GetUser err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Printf("[UserService] GetUser elapsed=%v", time.Since(start))
		}
	}()
	return m.next.GetUser(ctx, req)
}

func (m *loggingMiddleware) ListUsers(ctx context.Context, req idl.ListUsersRequest) (resp idl.ListUsersResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Printf("[UserService] ListUsers err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Printf("[UserService] ListUsers elapsed=%v", time.Since(start))
		}
	}()
	return m.next.ListUsers(ctx, req)
}

func (m *loggingMiddleware) DeleteUser(ctx context.Context, req idl.DeleteUserRequest) (resp idl.DeleteUserResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Printf("[UserService] DeleteUser err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Printf("[UserService] DeleteUser elapsed=%v", time.Since(start))
		}
	}()
	return m.next.DeleteUser(ctx, req)
}

func (m *loggingMiddleware) UpdateUser(ctx context.Context, req idl.UpdateUserRequest) (resp idl.UpdateUserResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Printf("[UserService] UpdateUser err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Printf("[UserService] UpdateUser elapsed=%v", time.Since(start))
		}
	}()
	return m.next.UpdateUser(ctx, req)
}

func (m *loggingMiddleware) FindByEmail(ctx context.Context, req idl.GetUserRequest) (resp idl.GetUserResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Printf("[UserService] FindByEmail err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Printf("[UserService] FindByEmail elapsed=%v", time.Since(start))
		}
	}()
	return m.next.FindByEmail(ctx, req)
}

func (m *loggingMiddleware) SearchUsers(ctx context.Context, req idl.ListUsersRequest) (resp idl.ListUsersResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Printf("[UserService] SearchUsers err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Printf("[UserService] SearchUsers elapsed=%v", time.Since(start))
		}
	}()
	return m.next.SearchUsers(ctx, req)
}

func (m *loggingMiddleware) QueryStats(ctx context.Context, req idl.GetUserRequest) (resp idl.GetUserResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Printf("[UserService] QueryStats err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Printf("[UserService] QueryStats elapsed=%v", time.Since(start))
		}
	}()
	return m.next.QueryStats(ctx, req)
}

func (m *loggingMiddleware) RemoveExpired(ctx context.Context, req idl.DeleteUserRequest) (resp idl.DeleteUserResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Printf("[UserService] RemoveExpired err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Printf("[UserService] RemoveExpired elapsed=%v", time.Since(start))
		}
	}()
	return m.next.RemoveExpired(ctx, req)
}

func (m *loggingMiddleware) EditProfile(ctx context.Context, req idl.UpdateUserRequest) (resp idl.UpdateUserResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Printf("[UserService] EditProfile err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Printf("[UserService] EditProfile elapsed=%v", time.Since(start))
		}
	}()
	return m.next.EditProfile(ctx, req)
}

func (m *loggingMiddleware) ModifyEmail(ctx context.Context, req idl.UpdateUserRequest) (resp idl.UpdateUserResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Printf("[UserService] ModifyEmail err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Printf("[UserService] ModifyEmail elapsed=%v", time.Since(start))
		}
	}()
	return m.next.ModifyEmail(ctx, req)
}

func (m *loggingMiddleware) PatchStatus(ctx context.Context, req idl.UpdateUserRequest) (resp idl.UpdateUserResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Printf("[UserService] PatchStatus err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Printf("[UserService] PatchStatus elapsed=%v", time.Since(start))
		}
	}()
	return m.next.PatchStatus(ctx, req)
}


func MetricsMiddleware() ServiceMiddleware {
	return func(next UserService) UserService {
		return &metricsMiddleware{next: next}
	}
}

type metricsMiddleware struct {
	next UserService
}


func (m *metricsMiddleware) CreateUser(ctx context.Context, req idl.CreateUserRequest) (idl.CreateUserResponse, error) {
	return m.next.CreateUser(ctx, req)
}

func (m *metricsMiddleware) GetUser(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error) {
	return m.next.GetUser(ctx, req)
}

func (m *metricsMiddleware) ListUsers(ctx context.Context, req idl.ListUsersRequest) (idl.ListUsersResponse, error) {
	return m.next.ListUsers(ctx, req)
}

func (m *metricsMiddleware) DeleteUser(ctx context.Context, req idl.DeleteUserRequest) (idl.DeleteUserResponse, error) {
	return m.next.DeleteUser(ctx, req)
}

func (m *metricsMiddleware) UpdateUser(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error) {
	return m.next.UpdateUser(ctx, req)
}

func (m *metricsMiddleware) FindByEmail(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error) {
	return m.next.FindByEmail(ctx, req)
}

func (m *metricsMiddleware) SearchUsers(ctx context.Context, req idl.ListUsersRequest) (idl.ListUsersResponse, error) {
	return m.next.SearchUsers(ctx, req)
}

func (m *metricsMiddleware) QueryStats(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error) {
	return m.next.QueryStats(ctx, req)
}

func (m *metricsMiddleware) RemoveExpired(ctx context.Context, req idl.DeleteUserRequest) (idl.DeleteUserResponse, error) {
	return m.next.RemoveExpired(ctx, req)
}

func (m *metricsMiddleware) EditProfile(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error) {
	return m.next.EditProfile(ctx, req)
}

func (m *metricsMiddleware) ModifyEmail(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error) {
	return m.next.ModifyEmail(ctx, req)
}

func (m *metricsMiddleware) PatchStatus(ctx context.Context, req idl.UpdateUserRequest) (idl.UpdateUserResponse, error) {
	return m.next.PatchStatus(ctx, req)
}

