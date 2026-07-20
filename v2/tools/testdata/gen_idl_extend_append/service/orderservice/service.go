package orderservice

import (
	"context"
	"errors"
	"time"

	idl "example.com/gen_idl_extend_append"
	kitlog "github.com/dreamsxin/go-kit/v2/log"
)

// OrderService defines the business contract.
type OrderService interface {

	// PlaceOrder
	PlaceOrder(ctx context.Context, req idl.PlaceOrderRequest) (idl.PlaceOrderResponse, error)
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
func NewService(cfg *ServiceConfig) OrderService {
	if cfg == nil {
		cfg = defaultConfig
	}
	return newServiceImpl(cfg)
}

func newServiceImpl(cfg *ServiceConfig) OrderService {
	logger := cfg.Logger
	if logger == nil {
		logger = kitlog.NewNopLogger()
	}
	var svc OrderService = &serviceImpl{
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

func (s *serviceImpl) PlaceOrder(ctx context.Context, req idl.PlaceOrderRequest) (idl.PlaceOrderResponse, error) {
	_ = req
	return idl.PlaceOrderResponse{}, errors.New("PlaceOrder: not implemented")
}

type ServiceMiddleware func(OrderService) OrderService

func LoggingMiddleware(logger *kitlog.Logger) ServiceMiddleware {
	return func(next OrderService) OrderService {
		return &loggingMiddleware{next: next, logger: logger}
	}
}

type loggingMiddleware struct {
	next   OrderService
	logger *kitlog.Logger
}

func (m *loggingMiddleware) PlaceOrder(ctx context.Context, req idl.PlaceOrderRequest) (resp idl.PlaceOrderResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Sugar().Infof("[OrderService] PlaceOrder err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Sugar().Infof("[OrderService] PlaceOrder elapsed=%v", time.Since(start))
		}
	}()
	return m.next.PlaceOrder(ctx, req)
}
