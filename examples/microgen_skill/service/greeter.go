package service

import (
	"context"
	"fmt"
	"log"
	"time"

	idl "github.com/dreamsxin/go-kit/examples/microgen_skill/pb"
)

// ─────────────────────────── 接口定义 ───────────────────────────

// Greeter 定义业务逻辑接口
type Greeter interface {

	// SayHello — SayHello greets a user.
	SayHello(ctx context.Context, req idl.HelloRequest) (idl.HelloResponse, error)

	// GetStatus — GetStatus returns the current service status.
	GetStatus(ctx context.Context, req idl.Empty) (idl.StatusResponse, error)

}

// ─────────────────────────── 配置 ───────────────────────────

// ServiceConfig 服务配置
type ServiceConfig struct {
	LogLevel      string        `json:"log_level"`
	Timeout       time.Duration `json:"timeout"`
	EnableLogging bool          `json:"enable_logging"`
	EnableMetrics bool          `json:"enable_metrics"`
}

// defaultConfig 默认配置
var defaultConfig = &ServiceConfig{
	LogLevel:      "info",
	Timeout:       30 * time.Second,
	EnableLogging: true,
}



// ─────────────────────────── 构造 ───────────────────────────


// NewService 创建服务实例
//
// cfg 为空时使用默认配置。
func NewService(cfg *ServiceConfig) Greeter {
	if cfg == nil {
		cfg = defaultConfig
	}
	return newServiceImpl(cfg)
}

// newServiceImpl 创建底层实现并包装中间件
func newServiceImpl(cfg *ServiceConfig) Greeter {
	var svc Greeter = &serviceImpl{
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


// ─────────────────────────── 实现 ───────────────────────────

// serviceImpl 服务具体实现
type serviceImpl struct {
	config *ServiceConfig
	logger *log.Logger
}


// SayHello 实现 Greeter.SayHello
func (s *serviceImpl) SayHello(ctx context.Context, req idl.HelloRequest) (idl.HelloResponse, error) {
	return idl.HelloResponse{
		Message: fmt.Sprintf("Hello, %s!", req.Name),
	}, nil
}

// GetStatus 实现 Greeter.GetStatus
func (s *serviceImpl) GetStatus(ctx context.Context, req idl.Empty) (idl.StatusResponse, error) {
	return idl.StatusResponse{
		Alive:   true,
		Version: "1.0.0",
	}, nil
}


// ─────────────────────────── 工具函数 ───────────────────────────

// errorf 格式化业务错误
func errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

// ─────────────────────────── 中间件 ───────────────────────────

// ServiceMiddleware 服务中间件类型
type ServiceMiddleware func(Greeter) Greeter

// ── Logging ──

// LoggingMiddleware 日志中间件：记录每次方法调用的耗时与错误
func LoggingMiddleware(logger *log.Logger) ServiceMiddleware {
	return func(next Greeter) Greeter {
		return &loggingMiddleware{next: next, logger: logger}
	}
}

type loggingMiddleware struct {
	next   Greeter
	logger *log.Logger
}


func (m *loggingMiddleware) SayHello(ctx context.Context, req idl.HelloRequest) (resp idl.HelloResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Printf("[Greeter] SayHello err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Printf("[Greeter] SayHello elapsed=%v", time.Since(start))
		}
	}()
	return m.next.SayHello(ctx, req)
}

func (m *loggingMiddleware) GetStatus(ctx context.Context, req idl.Empty) (resp idl.StatusResponse, err error) {
	start := time.Now()
	defer func() {
		if err != nil {
			m.logger.Printf("[Greeter] GetStatus err=%v elapsed=%v", err, time.Since(start))
		} else {
			m.logger.Printf("[Greeter] GetStatus elapsed=%v", time.Since(start))
		}
	}()
	return m.next.GetStatus(ctx, req)
}


// ── Metrics ──

// MetricsMiddleware 指标中间件（占位，可替换为 Prometheus 等实现）
func MetricsMiddleware() ServiceMiddleware {
	return func(next Greeter) Greeter {
		return &metricsMiddleware{next: next}
	}
}

type metricsMiddleware struct {
	next Greeter
}


func (m *metricsMiddleware) SayHello(ctx context.Context, req idl.HelloRequest) (idl.HelloResponse, error) {
	// TODO: 在此添加指标埋点（如 Prometheus counter/histogram）
	return m.next.SayHello(ctx, req)
}

func (m *metricsMiddleware) GetStatus(ctx context.Context, req idl.Empty) (idl.StatusResponse, error) {
	// TODO: 在此添加指标埋点（如 Prometheus counter/histogram）
	return m.next.GetStatus(ctx, req)
}

