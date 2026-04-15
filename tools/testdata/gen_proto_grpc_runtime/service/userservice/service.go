package userservice

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	idl "example.com/gen_proto_grpc_runtime/pb"
)

// UserService defines the business contract.
type UserService interface {

	// GetUser - GetUser retrieves a user by ID.
	GetUser(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error)

	// CreateUser - CreateUser creates a new user.
	CreateUser(ctx context.Context, req idl.CreateUserRequest) (idl.CreateUserResponse, error)

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




// NewService creates a service instance.
func NewService(cfg *ServiceConfig) UserService {
	if cfg == nil {
		cfg = defaultConfig
	}
	return newServiceImpl(cfg)
}

func newServiceImpl(cfg *ServiceConfig) UserService {
	var svc UserService = &serviceImpl{
		config: cfg,
		logger: log.Default(),
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
}


func (s *serviceImpl) GetUser(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error) {
	_ = req
	return idl.GetUserResponse{}, errors.New("GetUser: not implemented")
}

func (s *serviceImpl) CreateUser(ctx context.Context, req idl.CreateUserRequest) (idl.CreateUserResponse, error) {
	_ = req
	return idl.CreateUserResponse{}, errors.New("CreateUser: not implemented")
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


func MetricsMiddleware() ServiceMiddleware {
	return func(next UserService) UserService {
		return &metricsMiddleware{next: next}
	}
}

type metricsMiddleware struct {
	next UserService
}


func (m *metricsMiddleware) GetUser(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error) {
	return m.next.GetUser(ctx, req)
}

func (m *metricsMiddleware) CreateUser(ctx context.Context, req idl.CreateUserRequest) (idl.CreateUserResponse, error) {
	return m.next.CreateUser(ctx, req)
}

