# go-kit 使用帮助文档

<https://github.com/dreamsxin/go-kit>

## 介绍

该标准库提供了微服务开发的核心组件，包括端点(Endpoint)抽象、中间件机制、熔断降级、限流和服务发现缓存等功能，帮助开发者快速构建可靠的分布式服务。

## 目录结构

```plainText
go-kit/
├── endpoint/          # 端点管理与中间件
├── examples/          # 示例代码
├── sd/                # 服务发现组件
└── transport/         # 传输层实现
└── utils/             # 工具函数
```

## 核心组件

### 1. 端点(Endpoint)

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

### 2. 中间件(Middleware)

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

### 3. 熔断降级

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

### 4. 限流

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
    -import github.com/dreamsxin/go-kit/examples/usersvc \
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
