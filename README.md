# go-kit 使用帮助文档

<https://github.com/dreamsxin/go-kit>

## 介绍

该标准库提供了微服务开发的核心组件，包括端点(Endpoint)抽象、中间件机制、熔断降级、限流和服务发现缓存等功能，帮助开发者快速构建可靠的分布式服务。

## 目录结构

```plaintext
go-kit/
├── cmd/                          # 命令行工具
│   └── microgen/                 # 代码生成器
│       ├── generator/            # 代码生成逻辑
│       ├── parser/               # IDL 解析器
│       └── templates/            # 代码模板
├── endpoint/                     # 端点核心模块
│   ├── circuitbreaker/           # 熔断降级实现
│   │   ├── gobreaker.go         # Sony gobreaker 实现
│   │   ├── hystrix.go           # Netflix Hystrix 实现
│   │   └── handy_breaker.go     # Handy 熔断器
│   ├── ratelimit/               # 限流组件
│   │   └── token_bucket.go      # 令牌桶限流算法
│   ├── endpoint.go              # 端点基础定义
│   ├── middleware.go            # 中间件机制
│   └── factory.go               # 端点工厂模式
├── transport/                    # 传输层实现
│   ├── http/                    # HTTP 传输层
│   │   ├── client/              # HTTP 客户端
│   │   └── server/              # HTTP 服务端
│   ├── grpc/                    # gRPC 传输层
│   │   ├── client/              # gRPC 客户端
│   │   └── server/              # gRPC 服务端
│   └── error_handler.go         # 错误处理
├── sd/                          # 服务发现组件
├── examples/                    # 示例代码
├── log/                         # 日志组件
├── utils/                       # 工具函数
├── go.mod                       # 模块定义
└── README.md                    # 项目文档
```

## 🛠️ 核心组件详解

### 1. 端点(Endpoint)系统

端点是服务的基本单元，定义了服务的输入输出格式。

```go
// 端点定义：映射到一个具体目标地址
type Endpoint func(ctx context.Context, request interface{}) (response interface{}, err error)
```

创建端点示例：

```go
// 定义服务接口
type Server interface {
    Hello(name string) (ret string, err error)
}

// 将服务方法转换为端点
func MakeTestHelloEndpoint(svc Server) endpoint.Endpoint {
    return func(ctx context.Context, request interface{}) (interface{}, error) {
        name := request.(string)
        ret, err := svc.Hello(name)
        return ret, err
    }
}
```

**中间件支持**：

- 日志记录
- 熔断降级
- 限流控制
- 监控指标
- 认证授权

### 2. 传输层(Transport)

支持多种传输协议，提供统一的抽象接口：

#### HTTP 传输层

- **服务端**：基于 Gorilla Mux 的路由处理
- **客户端**：标准 HTTP 客户端封装
- **编解码**：JSON/XML 等格式支持

#### gRPC 传输层

- **服务端**：完整的 gRPC 服务绑定
- **客户端**：gRPC 客户端连接管理
- **Proto 支持**：自动生成 protobuf 定义

### 3. 服务发现(SD)

集成 Consul 等服务发现机制，支持动态服务注册与发现：

```go
// 服务发现工厂
type Factory func(instance string) (endpoint.Endpoint, error)
```

### 4. 中间件(Middleware)

支持通过中间件对端点进行增强，如日志、监控、限流等。

```go
// 定义端点中间件类型
type Middleware func(Endpoint) Endpoint

// 链式调用中间件
func Chain(outer Middleware, others ...Middleware) Middleware {
    return func(next Endpoint) Endpoint {
        for i := len(others) - 1; i >= 0; i-- { // 反向遍历，保证执行顺序
            next = others[i](next)
        }
        return outer(next)
    }
}
```

使用示例：

```go
// 创建中间件链
var endpoint endpoint.Endpoint
endpoint = MakeTestHelloEndpoint(svc)
endpoint = Chain(
    loggingMiddleware,
    circuitbreakerMiddleware,
    ratelimitMiddleware,
)(endpoint)
```

### 5. 熔断降级

提供两种熔断实现：基于`sony/gobreaker`和`afex/hystrix`。

gobreaker 实现：

```go
func Gobreaker(cb *gobreaker.CircuitBreaker) endpoint.Middleware {
    return func(next endpoint.Endpoint) endpoint.Endpoint {
        return func(ctx context.Context, request interface{}) (interface{}, error) {
            return cb.Execute(func() (interface{}, error) { return next(ctx, request) })
        }
    }
}
```

hystrix 实现：

```go
func Hystrix(commandName string) endpoint.Middleware {
    return func(next endpoint.Endpoint) endpoint.Endpoint {
        return func(ctx context.Context, request interface{}) (response interface{}, err error) {
            var resp interface{}
            if err := hystrix.Do(commandName, func() (err error) {
                resp, err = next(ctx, request)
                return err
            }, nil); err != nil {
                return nil, err
            }
            return resp, nil
        }
    }
}
```

### 6. 限流

基于令牌桶算法实现请求限流，支持错误拒绝和延迟等待两种模式。

```go
// 错误拒绝模式
func NewErroringLimiter(limit Allower) endpoint.Middleware {
    return func(next endpoint.Endpoint) endpoint.Endpoint {
        return func(ctx context.Context, request interface{}) (interface{}, error) {
            if !limit.Allow() {
                return nil, ErrLimited
            }
            return next(ctx, request)
        }
    }
}

// 延迟等待模式
func NewDelayingLimiter(limit Waiter) endpoint.Middleware {
    return func(next endpoint.Endpoint) endpoint.Endpoint {
        return func(ctx context.Context, request interface{}) (interface{}, error) {
            if err := limit.Wait(ctx); err != nil {
                return nil, err
            }
            return next(ctx, request)
        }
    }
}
```

## 注意事项

- 中间件顺序：Chain 函数会反向执行传入的中间件，实际执行顺序为第一个参数最后执行
- 熔断策略：根据业务需求选择 gobreaker 或 hystrix 实现
- 限流配置：令牌桶参数需根据服务承载能力合理设置
- 端点缓存：结合服务发现组件使用时，需正确实现 Factory 接口

## 代码自动生成

```shell
# 使用 examples/usersvc 作为模板生成代码
.\microgen.exe \
    -idl ./examples/usersvc/idl.go \
    -out ./generated-usersvc \
    -import github.com/your-project/usersvc \
    -protocols http \
    -service UserService
```

### 运行生成的代码

```shell
# 进入生成的服务目录
cd generated-usersvc

# 安装依赖
go mod init github.com/your-project/usersvc
go mod tidy

# 运行服务
go run ./cmd/usersvc/main.go -http.addr :8080
```

## Donation

- [捐贈（Donation）](https://github.com/dreamsxin/cphalcon7/blob/master/DONATE.md)
