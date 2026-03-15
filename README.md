# go-kit 微服务框架

> 一个功能完整、开箱即用的 Go 微服务开发框架，提供端点抽象、中间件机制、多传输协议、熔断限流、服务发现与代码生成等核心能力。

[![Go Version](https://img.shields.io/badge/go-1.21+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE.txt)

---

## 目录

- [特性](#特性)
- [快速开始](#快速开始)
- [项目结构](#项目结构)
- [核心概念](#核心概念)
  - [Endpoint — 端点](#1-endpoint--端点)
  - [Middleware — 中间件](#2-middleware--中间件)
  - [Transport — 传输层](#3-transport--传输层)
  - [CircuitBreaker — 熔断器](#4-circuitbreaker--熔断器)
  - [RateLimit — 限流](#5-ratelimit--限流)
  - [SD — 服务发现](#6-sd--服务发现)
  - [Log — 日志](#7-log--日志)
- [代码生成工具 microgen](#代码生成工具-microgen)
- [开发命令](#开发命令)
- [依赖](#依赖)
- [贡献指南](#贡献指南)
- [许可证](#许可证)

---

## 特性

| 功能 | 说明 |
|------|------|
| **端点抽象** | 统一的 `Endpoint` 类型，隔离业务逻辑与传输细节 |
| **中间件链** | 洋葱模型 `Chain`，按声明顺序从外到内包裹端点 |
| **HTTP 传输** | 完整的 Server/Client 封装，支持 Before/After/Finalizer 钩子 |
| **gRPC 传输** | Server/Client 封装，支持 Metadata 注入与 Interceptor |
| **熔断降级** | 内置 Gobreaker、Hystrix、HandyBreaker 三种实现 |
| **令牌桶限流** | 错误拒绝（ErroringLimiter）和延迟等待（DelayingLimiter）两种模式 |
| **服务发现** | Consul 集成，支持服务注册、发现与动态端点缓存 |
| **负载均衡** | 无锁原子 RoundRobin 轮询 |
| **重试执行器** | 支持最大次数、超时、指数退避和自定义 Callback |
| **结构化日志** | 基于 `go.uber.org/zap`，提供 Nop/Development 两种 Logger |
| **代码生成** | `microgen` 工具，一键生成 service/endpoint/transport/model 全套代码 |

---

## 快速开始

### 安装

```bash
go get github.com/dreamsxin/go-kit
```

### Hello World — HTTP 服务

```go
package main

import (
    "context"
    "encoding/json"
    "net/http"

    "github.com/dreamsxin/go-kit/endpoint"
    httpserver "github.com/dreamsxin/go-kit/transport/http/server"
)

func main() {
    // 1. 定义端点
    ep := endpoint.Endpoint(func(_ context.Context, req interface{}) (interface{}, error) {
        return map[string]string{"message": "Hello, " + req.(string)}, nil
    })

    // 2. 创建 HTTP Server
    handler := httpserver.NewServer(
        ep,
        func(_ context.Context, r *http.Request) (interface{}, error) {
            return r.URL.Query().Get("name"), nil
        },
        httpserver.EncodeJSONResponse,
    )

    http.ListenAndServe(":8080", handler)
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

ep = endpoint.Chain(
    ratelimit.NewErroringLimiter(rate.NewLimiter(rate.Every(time.Second), 100)),
    circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(gobreaker.Settings{})),
)(ep)
```

---

## 项目结构

```
go-kit/
├── cmd/
│   └── microgen/            # 代码生成工具
│       ├── main.go          # CLI 入口（11 个 flag）
│       ├── generator/       # 代码生成逻辑
│       ├── parser/          # IDL 解析器
│       └── templates/       # 11 个 Go 模板文件
│
├── endpoint/                # 端点核心模块
│   ├── endpoint.go          # Endpoint 类型、Nop、Failer
│   ├── middleware.go        # Middleware 类型、Chain
│   ├── factory.go           # Factory 类型
│   ├── endpoint_cache.go    # EndpointCache（热更新、失效控制）
│   ├── metrics.go           # MetricsMiddleware
│   ├── error_handler.go     # ErrorHandlingMiddleware、ErrorWrapper
│   ├── circuitbreaker/      # 熔断器实现
│   │   ├── gobreaker.go     # sony/gobreaker
│   │   ├── hystrix.go       # afex/hystrix-go
│   │   └── handy_breaker.go # streadway/handy
│   └── ratelimit/
│       └── token_bucket.go  # NewErroringLimiter、NewDelayingLimiter
│
├── transport/
│   ├── error_handler.go     # ErrorHandler 接口、LogErrorHandler、NopErrorHandler
│   └── http/
│   │   ├── context.go       # PopulateRequestContext（14 个 ContextKey）
│   │   ├── server/          # HTTP Server（NewServer + 5 个 ServerOption）
│   │   └── client/          # HTTP Client（NewClient + 5 个 ClientOption）
│   └── grpc/
│       ├── server/          # gRPC Server（NewServer + Interceptor）
│       └── client/          # gRPC Client（NewClient + SetRequestHeader）
│
├── sd/                      # 服务发现
│   ├── events/              # Event{Instances, Err}
│   ├── interfaces/          # Instancer / Balancer / Registrar 接口
│   ├── instance/            # Cache（线程安全广播）
│   ├── endpointer/
│   │   ├── endpointer.go    # NewEndpointer（Instancer+Factory → Endpointer）
│   │   ├── balancer/        # NewRoundRobin
│   │   └── executor/        # Retry / RetryAlways / RetryWithCallback
│   └── consul/              # Consul Instancer + Registrar + Client
│
├── log/                     # Logger = zap.Logger
├── utils/                   # Exponential（指数退避）
├── examples/                # 示例与集成测试
│   ├── basic/               # 中间件链示例
│   ├── usersvc/             # IDL 定义（CRUD 接口）
│   ├── profilesvc/          # 完整 HTTP 服务示例
│   └── transport/           # HTTP/gRPC Server+Client 集成测试
├── go.mod
└── Makefile
```

---

## 核心概念

### 1. Endpoint — 端点

端点是框架的最小调用单元，代表一次服务调用：

```go
type Endpoint func(ctx context.Context, request interface{}) (response interface{}, err error)

// 内置空操作端点，常用于测试
var Nop Endpoint = func(context.Context, interface{}) (interface{}, error) {
    return struct{}{}, nil
}
```

**Factory** — 根据服务实例地址创建端点（供服务发现使用）：

```go
type Factory func(instance string) (Endpoint, io.Closer, error)
```

**EndpointCache** — 缓存 `instance → Endpoint` 映射，监听服务发现事件动态更新：

```go
cache := endpoint.NewEndpointCache(factory, logger, endpoint.EndpointerOptions{
    InvalidateOnError: true,           // 发生错误时触发缓存失效
    InvalidateTimeout: 5 * time.Second, // 失效等待时间
})
// 接收服务发现事件
cache.Update(event)
// 获取当前所有可用端点
endpoints, err := cache.Endpoints()
```

`InvalidateOnError` 选项的语义：错误事件到来后，等待 `InvalidateTimeout` 到期时才清空旧端点缓存，在此期间仍可使用旧端点服务请求：

```go
endpoint.InvalidateOnError(5 * time.Second)  // 可作为 EndpointerOption 传入 NewEndpointer
```

**Failer** — 让 response 携带业务错误，用于区分传输错误和业务错误：

```go
type Failer interface {
    Failed() error
}
```

**MetricsMiddleware** — 无锁计数统计请求指标：

```go
metrics := &endpoint.Metrics{}
ep = endpoint.MetricsMiddleware(metrics)(ep)
// 调用后可读取：
// metrics.RequestCount / SuccessCount / ErrorCount / TotalDuration
```

**ErrorHandlingMiddleware** — 将端点错误包装为带 Operation 字段的 `*ErrorWrapper`：

```go
ep = endpoint.ErrorHandlingMiddleware("UserService.CreateUser")(ep)

// 错误解包：
var wrapped *endpoint.ErrorWrapper
if errors.As(err, &wrapped) {
    fmt.Println(wrapped.Operation, wrapped.Err)
}
```

---

### 2. Middleware — 中间件

```go
type Middleware func(Endpoint) Endpoint

// Chain 将多个中间件串联（洋葱模型）
// 执行顺序：outer pre → m1 pre → m2 pre → Endpoint → m2 post → m1 post → outer post
func Chain(outer Middleware, others ...Middleware) Middleware
```

**使用示例：**

```go
ep = endpoint.Chain(
    loggingMiddleware,      // 最外层：最先执行 pre，最后执行 post
    metricsMiddleware,
    authMiddleware,         // 最内层：最后执行 pre，最先执行 post
)(myEndpoint)
```

**自定义中间件：**

```go
import "go.uber.org/zap"

func LoggingMiddleware(logger *zap.Logger) endpoint.Middleware {
    return func(next endpoint.Endpoint) endpoint.Endpoint {
        return func(ctx context.Context, req interface{}) (interface{}, error) {
            logger.Sugar().Infof("request: %v", req)
            resp, err := next(ctx, req)
            logger.Sugar().Infof("response: %v, err: %v", resp, err)
            return resp, err
        }
    }
}
```

---

### 3. Transport — 传输层

#### 3.1 HTTP Server

```go
import (
    httpserver   "github.com/dreamsxin/go-kit/transport/http/server"
    httptransport "github.com/dreamsxin/go-kit/transport/http"   // PopulateRequestContext 在此包
    "github.com/dreamsxin/go-kit/transport"
)

handler := httpserver.NewServer(
    myEndpoint,                    // endpoint.Endpoint
    decodeMyRequest,               // DecodeRequestFunc
    httpserver.EncodeJSONResponse, // EncodeResponseFunc（内置）
    // Options（可选）：
    httpserver.ServerBefore(
        httptransport.PopulateRequestContext, // 将请求元数据注入 context
    ),
    httpserver.ServerAfter(func(ctx context.Context, r *http.Request, w *httpserver.InterceptingWriter) context.Context {
        return ctx
    }),
    httpserver.ServerErrorEncoder(myErrorEncoder),
    httpserver.ServerErrorHandler(transport.NewLogErrorHandler(logger)),
    httpserver.ServerFinalizer(func(ctx context.Context, r *http.Request, w *httpserver.InterceptingWriter) {
        // 请求结束后执行（无论成败），可读取 w.GetCode() / w.GetWritten()
    }),
)
http.ListenAndServe(":8080", handler)
```

内置 `EncodeResponseFunc`：

| 函数 | 说明 |
|------|------|
| `EncodeJSONResponse` | 序列化为 JSON，自动处理 `StatusCoder`/`Headerer` 接口 |
| `NopResponseEncoder` | 空操作，仅返回 200 |

内置 `DecodeRequestFunc`：

| 函数 | 说明 |
|------|------|
| `NopRequestDecoder` | 返回 nil request |

**Context Keys（由 `PopulateRequestContext` 注入）：**

```go
// import httptransport "github.com/dreamsxin/go-kit/transport/http"
ctx.Value(httptransport.ContextKeyRequestMethod)      // HTTP 方法
ctx.Value(httptransport.ContextKeyRequestPath)        // URL Path
ctx.Value(httptransport.ContextKeyRequestHost)        // Host
ctx.Value(httptransport.ContextKeyRequestRemoteAddr)  // 远端地址
ctx.Value(httptransport.ContextKeyRequestXRequestID)  // X-Request-Id Header
// ... 共 14 个 key
```

**增强响应接口（response 实现以下接口时自动生效）：**

```go
// 自定义 HTTP 状态码
type StatusCoder interface { StatusCode() int }

// 追加响应 Header
type Headerer interface { Headers() http.Header }
```

#### 3.2 HTTP Client

```go
import httpclient "github.com/dreamsxin/go-kit/transport/http/client"

tgt, _ := url.Parse("http://localhost:8080/users")
client := httpclient.NewClient(
    http.MethodPost, tgt,
    httpclient.EncodeJSONRequest, // EncodeRequestFunc（内置）
    decodeMyResponse,             // DecodeResponseFunc
    // Options（可选）：
    httpclient.ClientBefore(func(ctx context.Context, r *http.Request) context.Context {
        r.Header.Set("Authorization", "Bearer token")
        return ctx
    }),
    httpclient.ClientAfter(func(ctx context.Context, r *http.Response, _ error) context.Context {
        return ctx
    }),
    httpclient.ClientFinalizer(func(ctx context.Context, err error) {
        // 请求完成后执行（无论成败）
    }),
    httpclient.SetClient(myHTTPClient),    // 替换底层 http.Client
    httpclient.BufferedStream(false),      // 是否流式读取响应
)

ep := client.Endpoint()
resp, err := ep(ctx, myRequest)
```

#### 3.3 gRPC Server

```go
import grpcserver "github.com/dreamsxin/go-kit/transport/grpc/server"

kitServer := grpcserver.NewServer(
    myEndpoint,
    decodeGRPCRequest,   // DecodeRequestFunc: func(context.Context, interface{}) (interface{}, error)
    encodeGRPCResponse,  // EncodeResponseFunc: func(context.Context, interface{}) (interface{}, error)
    // Options（可选）：
    grpcserver.ServerBefore(func(ctx context.Context, md metadata.MD) context.Context {
        // 从 metadata 提取信息注入 context（如 correlation-id）
        return ctx
    }),
    grpcserver.ServerAfter(func(ctx context.Context, header *metadata.MD, trailer *metadata.MD) context.Context {
        // 设置响应 header/trailer metadata
        return ctx
    }),
    grpcserver.ServerErrorLogger(logger),
    grpcserver.ServerFinalizer(func(ctx context.Context, err error) {}),
)

// 注册到 gRPC server
grpcServer := grpc.NewServer()
pb.RegisterMyServiceServer(grpcServer, &myGRPCBinding{kitServer})

// 或使用内置 Interceptor（将 gRPC 请求路由到对应 kitServer）
grpcServer = grpc.NewServer(
    grpc.UnaryInterceptor(grpcserver.Interceptor),
)
```

#### 3.4 gRPC Client

```go
import grpcclient "github.com/dreamsxin/go-kit/transport/grpc/client"

conn, _ := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))

client := grpcclient.NewClient(
    conn,
    "MyService",        // serviceName
    "CreateUser",       // method
    encodeGRPCRequest,  // EncodeRequestFunc
    decodeGRPCResponse, // DecodeResponseFunc
    &pb.CreateUserResponse{}, // grpcReply：接收响应的 proto 结构体指针
    // Options（可选）：
    grpcclient.ClientBefore(
        grpcclient.SetRequestHeader("x-correlation-id", correlationID),
    ),
    grpcclient.ClientAfter(func(ctx context.Context, header metadata.MD, trailer metadata.MD) context.Context {
        return ctx
    }),
    grpcclient.ClientFinalizer(func(ctx context.Context, err error) {}),
)

ep := client.Endpoint()
resp, err := ep(ctx, &pb.CreateUserRequest{Username: "alice"})
```

---

### 4. CircuitBreaker — 熔断器

提供三种熔断实现，均返回 `endpoint.Middleware`，可直接用于 `Chain`。

#### Gobreaker（推荐）

```go
import (
    "github.com/dreamsxin/go-kit/endpoint/circuitbreaker"
    "github.com/sony/gobreaker"
)

cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
    Name:        "my-service",
    MaxRequests: 1,                  // 半开状态最大探测请求数
    Interval:    10 * time.Second,   // 统计窗口
    Timeout:     30 * time.Second,   // 熔断后等待时间
    ReadyToTrip: func(counts gobreaker.Counts) bool {
        return counts.ConsecutiveFailures > 5
    },
})

ep = circuitbreaker.Gobreaker(cb)(ep)
```

#### Hystrix

```go
import (
    "github.com/dreamsxin/go-kit/endpoint/circuitbreaker"
    hystrixgo "github.com/afex/hystrix-go/hystrix"
)

hystrixgo.ConfigureCommand("my-command", hystrixgo.CommandConfig{
    Timeout:                1000,
    MaxConcurrentRequests:  100,
    ErrorPercentThreshold:  25,
})

ep = circuitbreaker.Hystrix("my-command")(ep)
```

#### HandyBreaker

```go
import (
    "github.com/dreamsxin/go-kit/endpoint/circuitbreaker"
    "github.com/streadway/handy/breaker"
)

ep = circuitbreaker.HandyBreaker(breaker.NewBreaker(0.05))(ep)
```

---

### 5. RateLimit — 限流

基于 `golang.org/x/time/rate` 令牌桶算法，`rate.Limiter` 同时实现了 `Allower` 和 `Waiter` 接口，可直接传入。

```go
import (
    "github.com/dreamsxin/go-kit/endpoint/ratelimit"
    "golang.org/x/time/rate"
)

limiter := rate.NewLimiter(
    rate.Every(time.Second), // 每秒补充令牌速率
    100,                     // burst（令牌桶容量）
)

// 模式一：超限立即返回 ratelimit.ErrLimited 错误
ep = ratelimit.NewErroringLimiter(limiter)(ep)

// 模式二：超限则等待直到令牌可用（受 ctx deadline 控制）
ep = ratelimit.NewDelayingLimiter(limiter)(ep)
```

也可通过函数适配器实现自定义限流逻辑：

```go
ep = ratelimit.NewErroringLimiter(
    ratelimit.AllowerFunc(func() bool { return myCustomAllow() }),
)(ep)
```

---

### 6. SD — 服务发现

服务发现模块由四个层次组成：

```
Instancer → EndpointCache → Endpointer → Balancer → Retry
```

#### 核心接口

```go
// Instancer：监听服务实例变更
type Instancer interface {
    Register(chan<- events.Event)
    Deregister(chan<- events.Event)
    Stop()
}

// Balancer：从多个端点中选择一个
type Balancer interface {
    Endpoint() (endpoint.Endpoint, error)
}

// Registrar：服务注册/注销
type Registrar interface {
    Register()
    Deregister()
}
```

#### Consul 集成

**服务注册：**

```go
import "github.com/dreamsxin/go-kit/sd/consul"

consulClient := consul.NewClient(consulAPI)

registrar := consul.NewRegistrar(
    consulClient, logger,
    "my-service",      // 服务名
    "192.168.1.10",    // 地址
    8080,              // 端口
    consul.IDRegistrarOptions("my-service-1"),
    consul.TagsRegistrarOptions([]string{"v1", "primary"}),
    consul.CheckRegistrarOptions(&stdconsul.AgentServiceCheck{
        HTTP:     "http://192.168.1.10:8080/health",
        Interval: "10s",
    }),
)
registrar.Register()
defer registrar.Deregister()
```

**服务发现与负载均衡：**

```go
import (
    "github.com/dreamsxin/go-kit/sd/consul"
    "github.com/dreamsxin/go-kit/sd/endpointer"
    "github.com/dreamsxin/go-kit/sd/endpointer/balancer"
    "github.com/dreamsxin/go-kit/sd/endpointer/executor"
    "github.com/dreamsxin/go-kit/endpoint"
)

// 1. 创建 Consul Instancer（自动监听健康实例变更）
instancer := consul.NewInstancer(consulClient, logger, "my-service", true)
defer instancer.Stop()

// 2. 工厂函数：instance 地址 → Endpoint
factory := func(instance string) (endpoint.Endpoint, io.Closer, error) {
    conn, err := grpc.Dial(instance, grpc.WithTransportCredentials(insecure.NewCredentials()))
    // ... 创建 gRPC client endpoint
    return clientEp, conn, err
}

// 3. 创建 Endpointer（自动维护活跃端点列表）
ep := endpointer.NewEndpointer(
    instancer, factory, logger,
    endpoint.InvalidateOnError(5*time.Second),
)

// 4. 轮询负载均衡
lb := balancer.NewRoundRobin(ep)

// 5. 带重试的请求
retryEp := executor.Retry(3, 500*time.Millisecond, lb)
resp, err := retryEp(ctx, request)
```

#### 重试策略

```go
// 最多重试 max 次，整体超时 timeout
executor.Retry(max int, timeout time.Duration, b Balancer) endpoint.Endpoint

// 在超时内无限重试
executor.RetryAlways(timeout time.Duration, b Balancer) endpoint.Endpoint

// 自定义回调控制是否继续重试
executor.RetryWithCallback(timeout time.Duration, b Balancer,
    func(n int, received error) (keepTrying bool, replacement error) {
        if errors.Is(received, ErrNotFound) {
            return false, received  // 不可重试的错误，立即停止
        }
        return true, nil  // 继续重试
    },
) endpoint.Endpoint
```

#### 不依赖 Consul 的单元测试

```go
// 用 instance/Cache 替代真实 Consul，完全内存驱动
import "github.com/dreamsxin/go-kit/sd/instance"

cache := instance.NewCache()
cache.Update(events.Event{Instances: []string{"127.0.0.1:8080", "127.0.0.1:8081"}})

ep := endpointer.NewEndpointer(cache, factory, logger)
```

---

### 7. Log — 日志

日志模块是 `go.uber.org/zap` 的轻量封装：

```go
import "github.com/dreamsxin/go-kit/log"

// 开发模式（彩色输出，含调用位置）
logger, err := log.NewDevelopment()

// 静默丢弃所有日志（用于测试或不需要日志的场景）
logger = log.NewNopLogger()
```

---

## 代码生成工具 microgen

`microgen` 根据 IDL 文件自动生成完整的微服务脚手架代码。

### 安装

```bash
make install-microgen
# 或手动编译
go build -o microgen.exe ./cmd/microgen
```

### IDL 定义

只需在 `.go` 文件中定义 Service 接口，microgen 会自动解析：

```go
// examples/usersvc/idl.go
package usersvc

type User struct {
    ID       uint   `json:"id"`
    Username string `json:"username"`
    Email    string `json:"email"`
    Age      int    `json:"age"`
}

type CreateUserRequest  struct { Username, Email string; Age int }
type CreateUserResponse struct { User *User; Error string }

type UserService interface {
    CreateUser(ctx context.Context, req CreateUserRequest) (CreateUserResponse, error)
    GetUser(ctx context.Context, req GetUserRequest)       (GetUserResponse,    error)
    UpdateUser(ctx context.Context, req UpdateUserRequest) (UpdateUserResponse, error)
    DeleteUser(ctx context.Context, req DeleteUserRequest) (DeleteUserResponse, error)
    ListUsers(ctx context.Context, req ListUsersRequest)   (ListUsersResponse,  error)
}
```

### 生成命令

```bash
# 生成 HTTP 服务（最小化）
./microgen.exe \
    -idl    ./examples/usersvc/idl.go \
    -out    ./generated-usersvc \
    -import github.com/myorg/usersvc \
    -protocols http

# 生成 HTTP + gRPC 双协议服务（含 model + Swagger）
./microgen.exe \
    -idl       ./examples/usersvc/idl.go \
    -out       ./generated-usersvc-grpc \
    -import    github.com/myorg/usersvc \
    -protocols http,grpc \
    -model     \
    -swag

# 生成全量代码（含测试文件）
./microgen.exe \
    -idl       ./examples/usersvc/idl.go \
    -out       ./generated-full \
    -import    github.com/myorg/usersvc \
    -protocols http,grpc \
    -model     \
    -db        \
    -db.driver mysql \
    -tests     \
    -swag
```

### 所有 Flag

| Flag | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `-idl` | string | — | IDL 文件路径（**必填**） |
| `-out` | string | `.` | 生成代码输出目录 |
| `-import` | string | — | 生成项目的 Go module 路径 |
| `-protocols` | string | `http` | 传输协议，逗号分隔：`http` / `grpc` / `http,grpc` |
| `-service` | string | — | 覆盖服务名（默认取 IDL 中第一个接口名） |
| `-model` | bool | `false` | 生成 GORM model + repository 层 |
| `-db` | bool | `false` | main.go 包含数据库初始化逻辑 |
| `-db.driver` | string | `sqlite` | GORM 驱动：`sqlite`/`mysql`/`postgres`/`sqlserver`/`clickhouse` |
| `-swag` | bool | `false` | 生成 swaggo 注释 + `/swagger/` 路由 |
| `-tests` | bool | `false` | 生成 service_test.go 单元测试文件 |
| `-config` | bool | `true` | 生成 config/config.yaml |
| `-docs` | bool | `true` | 生成 README.md |

### 生成的目录结构

```
generated-usersvc/
├── cmd/
│   └── main.go              # 启动入口（-http.addr / -grpc.addr 参数）
├── service/usersvc/
│   └── service.go           # Service 接口实现（业务逻辑层）
├── endpoint/usersvc/
│   └── endpoints.go         # 端点定义与 Make*Endpoint 工厂
├── transport/usersvc/
│   ├── transport_http.go    # HTTP 路由与编解码
│   └── transport_grpc.go    # gRPC 编解码（-protocols grpc 时生成）
├── pb/usersvc/
│   └── usersvc.proto        # Protobuf 定义（-protocols grpc 时生成）
├── model/
│   └── model.go             # GORM 模型（-model 时生成）
├── repository/
│   └── repository.go        # 数据库访问层（-model 时生成）
├── config/
│   └── config.yaml          # 服务配置文件
├── go.mod
└── README.md
```

### 运行生成的服务

```bash
cd generated-usersvc
go mod tidy
go run ./cmd/main.go -http.addr :8080

# 测试
curl -X POST http://localhost:8080/userservice/createuser \
     -H "Content-Type: application/json" \
     -d '{"username":"alice","email":"alice@example.com","age":25}'
```

---

## 开发命令

```bash
# 安装工具链（golangci-lint / swag / protoc-gen-go 等）
make tools

# 安装依赖
make deps

# 运行全量测试（含竞态检测）
make test

# 生成测试覆盖率报告（输出 coverage.html）
make coverage

# 代码静态检查
make lint

# 构建所有包
make build

# 安装 microgen 到 $GOPATH/bin
make install-microgen

# 代码生成快捷命令
make gen          # HTTP + SQLite + model + swag
make gen-http     # 仅 HTTP（最小化）
make gen-grpc     # HTTP + gRPC + model + swag
make gen-full     # HTTP + gRPC + model + swag + tests

# 从 .proto 文件重新生成 pb.go（需 protoc）
make proto

# 启动生成的示例服务
make run-demo

# 查看所有目标
make help
```

---

## 依赖

| 包 | 版本 | 用途 |
|----|------|------|
| `go.uber.org/zap` | v1.26.0 | 结构化日志 |
| `google.golang.org/grpc` | v1.61.0 | gRPC 传输层 |
| `google.golang.org/protobuf` | v1.31.0 | Protobuf 序列化 |
| `github.com/gorilla/mux` | v1.8.1 | HTTP 路由 |
| `github.com/sony/gobreaker` | v1.0.0 | 熔断器（Gobreaker） |
| `github.com/afex/hystrix-go` | v0.0.0-20180502 | 熔断器（Hystrix） |
| `github.com/streadway/handy` | v0.0.0-20200128 | 熔断器（HandyBreaker） |
| `golang.org/x/time` | v0.5.0 | 令牌桶限流 |
| `github.com/hashicorp/consul/api` | v1.27.0 | Consul 服务发现 |
| `github.com/google/go-cmp` | v0.6.0 | 测试深比较 |

---

## 贡献指南

1. Fork 项目到你的 GitHub
2. 创建功能分支：`git checkout -b feat/my-feature`
3. 提交变更：`git commit -m "feat: add my feature"`
4. 推送到分支：`git push origin feat/my-feature`
5. 创建 Pull Request

**代码规范：**

- 运行 `make lint` 确保无静态检查错误
- 运行 `make test` 确保全量测试通过
- 新功能须附带测试用例

---

## 许可证

本项目采用 [MIT 许可证](LICENSE.txt)。

---

## Donation

- [捐赠（Donation）](https://github.com/dreamsxin/cphalcon7/blob/master/DONATE.md)
