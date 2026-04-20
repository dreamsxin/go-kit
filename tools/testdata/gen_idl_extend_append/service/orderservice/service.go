package orderservice

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	idl "example.com/gen_idl_extend_append"
)

// OrderService defines the business contract.
type OrderService interface {

	// PlaceOrder
	PlaceOrder(ctx context.Context, req idl.PlaceOrderRequest) (idl.PlaceOrderResponse, error)

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
func NewService(cfg *ServiceConfig) OrderService {
	if cfg == nil {
		cfg = defaultConfig
	}
	return newServiceImpl(cfg)
}

func newServiceImpl(cfg *ServiceConfig) OrderService {
	var svc OrderService = &serviceImpl{
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


func (s *serviceImpl) PlaceOrder(ctx context.Context, req idl.PlaceOrderRequest) (idl.PlaceOrderResponse, error) {
	_ = req
	return idl.PlaceOrderResponse{}, errors.New("PlaceOrder: not implemented")
}


func errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

type ServiceMiddleware func(OrderService) OrderService

func LoggingMiddleware(logger *log.Logger) ServiceMiddleware {
	return func(next OrderService) OrderService {
		return &loggingMiddleware{next: next, logger: logger}
	}
}

type loggingMiddleware struct {
	next   OrderService
	logger *log.Logger
}


func (m *loggingMiddleware) PlaceOrder(ctx context.Context, req idl.PlaceOrderRequest) (resp idl.PlaceOrderResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Printf("[OrderService] PlaceOrder err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Printf("[OrderService] PlaceOrder elapsed=%v", time.Since(start))
		}
	}()
	return m.next.PlaceOrder(ctx, req)
}


func MetricsMiddleware() ServiceMiddleware {
	return func(next OrderService) OrderService {
		return &metricsMiddleware{next: next}
	}
}

type metricsMiddleware struct {
	next OrderService
}


func (m *metricsMiddleware) PlaceOrder(ctx context.Context, req idl.PlaceOrderRequest) (idl.PlaceOrderResponse, error) {
	return m.next.PlaceOrder(ctx, req)
}

