package userservice

import (
	"context"
	"errors"
	"time"

	idl "example.com/gen_proto_grpc_runtime/pb"
	kitlog "github.com/dreamsxin/go-kit/v2/log"
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
	EnableLogging bool           `json:"enable_logging"`
	Logger        *kitlog.Logger `json:"-"`
}

var defaultConfig = &ServiceConfig{
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
	logger := cfg.Logger
	if logger == nil {
		logger = kitlog.NewNopLogger()
	}
	var svc UserService = &serviceImpl{
		config: cfg,
		logger: logger,
	}

	if cfg.EnableLogging {
		svc = LoggingMiddleware(logger)(svc)
	}
	return svc
}

type serviceImpl struct {
	config *ServiceConfig
	logger *kitlog.Logger
}

func (s *serviceImpl) GetUser(ctx context.Context, req idl.GetUserRequest) (idl.GetUserResponse, error) {
	_ = req
	return idl.GetUserResponse{}, errors.New("GetUser: not implemented")
}

func (s *serviceImpl) CreateUser(ctx context.Context, req idl.CreateUserRequest) (idl.CreateUserResponse, error) {
	_ = req
	return idl.CreateUserResponse{}, errors.New("CreateUser: not implemented")
}

type ServiceMiddleware func(UserService) UserService

func LoggingMiddleware(logger *kitlog.Logger) ServiceMiddleware {
	return func(next UserService) UserService {
		return &loggingMiddleware{next: next, logger: logger}
	}
}

type loggingMiddleware struct {
	next   UserService
	logger *kitlog.Logger
}

func (m *loggingMiddleware) GetUser(ctx context.Context, req idl.GetUserRequest) (resp idl.GetUserResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Sugar().Infof("[UserService] GetUser err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Sugar().Infof("[UserService] GetUser elapsed=%v", time.Since(start))
		}
	}()
	return m.next.GetUser(ctx, req)
}

func (m *loggingMiddleware) CreateUser(ctx context.Context, req idl.CreateUserRequest) (resp idl.CreateUserResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Sugar().Infof("[UserService] CreateUser err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Sugar().Infof("[UserService] CreateUser elapsed=%v", time.Since(start))
		}
	}()
	return m.next.CreateUser(ctx, req)
}
