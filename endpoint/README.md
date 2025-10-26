# Endpoint 端点模块

端点(Endpoint)是 go-kit 框架的核心抽象，定义了微服务的基本调用单元。本模块提供了端点的定义、中间件机制、熔断降级和限流等功能。

## 核心组件

### 基础端点

- **endpoint.go** - 端点接口定义与基础实现
- **endpoint_cache.go** - 端点缓存管理
- **factory.go** - 端点创建工厂
- **middleware.go** - 端点中间件框架

### 熔断降级

- **circuitbreaker/** - 熔断器实现
  - `gobreaker.go` - Sony gobreaker 实现
  - `hystrix.go` - Netflix Hystrix 实现
  - `handy_breaker.go` - Handy 熔断器

### 限流控制

- **ratelimit/** - 限流实现
  - `token_bucket.go` - 令牌桶算法限流

## 快速使用

### 创建基础端点

```go
import "github.com/dreamsxin/go-kit/endpoint"

// 定义端点函数
var myEndpoint endpoint.Endpoint = func(ctx context.Context, request interface{}) (interface{}, error) {
    return "Hello, World!", nil
}
```

### 使用中间件

```go
import (
    "github.com/dreamsxin/go-kit/endpoint"
    "github.com/dreamsxin/go-kit/endpoint/circuitbreaker"
    "github.com/dreamsxin/go-kit/endpoint/ratelimit"
    "github.com/sony/gobreaker"
    "golang.org/x/time/rate"
)

// 创建熔断器
cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{})
// 创建限流器
limiter := rate.NewLimiter(rate.Every(time.Second), 10)

// 构建中间件链
ep := endpoint.Chain(
    loggingMiddleware,
    endpoint.ErrorHandlingMiddleware("service"),
    circuitbreaker.Gobreaker(cb),
    ratelimit.NewErroringLimiter(limiter),
)(myEndpoint)
```

## API 参考

### Endpoint 接口

```go
type Endpoint func(ctx context.Context, request interface{}) (response interface{}, err error)
```

### Middleware 类型

```go
type Middleware func(Endpoint) Endpoint
```

### 常用中间件

- `ErrorHandlingMiddleware` - 错误处理中间件
- `Gobreaker` - gobreaker 熔断器中间件
- `NewErroringLimiter` - 错误模式限流中间件
- `NewDelayingLimiter` - 延迟模式限流中间件

## 最佳实践

1. **中间件顺序**：日志 → 错误处理 → 熔断 → 限流
2. **错误处理**：使用 ErrorHandlingMiddleware 包装业务端点
3. **配置调优**：根据业务需求调整熔断和限流参数
4. **监控指标**：结合 metrics.go 实现监控指标收集
