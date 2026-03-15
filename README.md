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
  - [Executor — 重试执行器](#6-executor--重试执行器)
    - [三种重试策略](#三种重试策略)
    - [指数退避算法](#指数退避算法)
    - [RetryError 错误诊断](#retryerror-错误诊断)
    - [并发执行模型](#并发执行模型)
  - [SD — 服务发现](#7-sd--服务发现)
    - [Consul 集成](#consul-集成)
    - [重试执行器](#重试执行器)
    - [不依赖 Consul 的单元测试](#不依赖-consul-的单元测试)
    - [服务端 + 客户端完整场景示例](#服务端--客户端完整场景示例)
  - [Log — 日志](#8-log--日志)
  - [中间件链路打印](#9-中间件链路打印--调用链--文件行号)
    - [打印调用者文件名与行号](#91-打印调用者文件名与行号)
    - [Request-ID 链路透传中间件](#92-request-id-链路透传中间件)
    - [完整链路中间件组合示例](#93-完整链路中间件组合示例)
  - [MetricsMiddleware — 内置指标收集](#10-metricsmiddleware--内置指标收集)
  - [ErrorHandlingMiddleware — 错误包装](#11-errorhandlingmiddleware--错误包装)
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

> **设计理念：各组件相互独立，按需引入。**
> 只需要 HTTP 服务？仅 import `transport/http/server`。
> 只需要熔断？仅 import `endpoint/circuitbreaker`。
> 只需要限流？仅 import `endpoint/ratelimit`。
> 各包之间无强依赖，`endpoint.Endpoint` 是唯一的"胶水"类型——任何中间件、任何传输层、任何服务发现实现，都只认这一个接口，可以自由组合、替换或丢弃。

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
    "time"
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
// 自定义 HTTP 状态码（默认 200）
type StatusCoder interface { StatusCode() int }

// 追加任意响应 Header
type Headerer interface { Headers() http.Header }
```

**错误编码（`ErrorEncoder`）：**

`transport.DefaultErrorEncoder` 是服务器的默认错误编码器，按以下规则处理：
1. error 实现了 `json.Marshaler` → 返回 JSON body，`Content-Type: application/json`
2. error 实现了 `StatusCoder` → 使用其 `StatusCode()`，否则默认 `500`
3. error 实现了 `Headerer` → 将其 `Headers()` 合并入响应 Header

```go
// 自定义错误类型，自动控制 HTTP 状态码和 Header
type APIError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}
func (e *APIError) Error() string       { return e.Message }
func (e *APIError) StatusCode() int     { return e.Code }
func (e *APIError) MarshalJSON() ([]byte, error) {
    return json.Marshal(struct{ Code int; Message string }{e.Code, e.Message})
}

// 替换为完全自定义的 ErrorEncoder
httpserver.ServerErrorEncoder(func(ctx context.Context, err error, w http.ResponseWriter) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusBadRequest)
    json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
})

// 完全屏蔽错误日志
httpserver.ServerErrorHandler(transport.NopErrorHandler)
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

**`BufferedStream` 模式（流式响应）：**

默认情况下 response body 在 decode 后自动关闭。启用 `BufferedStream(true)` 时，body 的关闭权移交给调用方，适合文件下载、SSE 等场景：

```go
client := httpclient.NewClient(
    http.MethodGet, tgt,
    httpclient.EncodeJSONRequest,
    func(ctx context.Context, r *http.Response) (interface{}, error) {
        // 不要在此处关闭 r.Body，由调用方负责
        return r.Body, nil
    },
    httpclient.BufferedStream(true),
)
ep := client.Endpoint()
resp, _ := ep(ctx, nil)
body := resp.(io.ReadCloser)
defer body.Close() // 调用方负责关闭，关闭时同时取消 context
```

**`NewExplicitClient` — 完全自定义 Request 构建：**

当需要完整控制 `*http.Request`（如动态签名、非标准路径）时，使用 `NewExplicitClient` 跳过内置的 URL+方法绑定：

```go
// EncodeRequestFunc 签名：func(ctx, *http.Request, request) (*http.Request, error)
// 入参 *http.Request 可能为 nil，需自行构建
customEnc := func(ctx context.Context, _ *http.Request, req interface{}) (*http.Request, error) {
    r, _ := http.NewRequestWithContext(ctx, http.MethodPost,
        "https://api.example.com/v2/users", nil)
    r.Header.Set("X-Signature", computeHMAC(req))
    json.NewEncoder(/* body */).Encode(req)
    return r, nil
}
client := httpclient.NewExplicitClient(customEnc, decodeMyResponse)
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
import (
    grpcclient "github.com/dreamsxin/go-kit/transport/grpc/client"
    "google.golang.org/grpc/metadata"
)

conn, _ := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))

client := grpcclient.NewClient(
    conn,
    "MyService",        // serviceName（不含前导 /）
    "CreateUser",       // method
    encodeGRPCRequest,  // EncodeRequestFunc: func(ctx, req) (interface{}, error)
    decodeGRPCResponse, // DecodeResponseFunc: func(ctx, reply) (interface{}, error)
    &pb.CreateUserResponse{}, // grpcReply：接收响应的 proto 结构体指针（每次调用复用）
    // Options（可选）：
    grpcclient.ClientBefore(
        // RequestFunc 签名：func(context.Context, *metadata.MD) context.Context
        // 在此向 outgoing metadata 写入自定义 key（如 correlation-id、auth token）
        func(ctx context.Context, md *metadata.MD) context.Context {
            md.Set("x-correlation-id", correlationIDFromCtx(ctx))
            md.Set("authorization", "Bearer "+tokenFromCtx(ctx))
            return ctx
        },
    ),
    grpcclient.ClientAfter(
        // ResponseFunc 签名：func(context.Context, metadata.MD, metadata.MD) context.Context
        // 可从 header/trailer 中读取服务端返回的 metadata
        func(ctx context.Context, header metadata.MD, trailer metadata.MD) context.Context {
            if traceID := header.Get("x-trace-id"); len(traceID) > 0 {
                ctx = context.WithValue(ctx, traceIDKey, traceID[0])
            }
            return ctx
        },
    ),
    grpcclient.ClientFinalizer(func(ctx context.Context, err error) {
        // 请求完成后执行（无论成败），适合记录耗时日志
    }),
)

ep := client.Endpoint()
resp, err := ep(ctx, &pb.CreateUserRequest{Username: "alice"})
```

**gRPC Client 钩子函数签名对照：**

| 钩子 | 函数签名 | 调用时机 |
|---|---|---|
| `ClientBefore` | `func(context.Context, *metadata.MD) context.Context` | 发送请求前，写入 outgoing metadata |
| `ClientAfter` | `func(context.Context, metadata.MD, metadata.MD) context.Context` | 收到响应后，读取 header/trailer |
| `ClientFinalizer` | `func(context.Context, error)` | 请求结束（无论成败） |

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

### 6. Executor — 重试执行器

`sd/endpointer/executor` 包提供**带重试能力的 Endpoint 包装器**，配合 Balancer 在每次重试时自动切换到不同的后端实例，实现故障转移。

```go
import (
    "github.com/dreamsxin/go-kit/sd/endpointer/executor"
    "github.com/dreamsxin/go-kit/sd/interfaces"
)
```

执行器的入参是任意实现了 `interfaces.Balancer` 的负载均衡器（如 `balancer.NewRoundRobin`），出参是一个标准 `endpoint.Endpoint`，可无缝嵌入中间件链。

#### 三种重试策略

```go
// --- 策略一：Retry — 固定最大次数 ---
// 最多调用 max 次（首次 + max-1 次重试），整体受 timeout 控制
// 超出次数或超时时，返回包含所有历史错误的 RetryError
retryEp := executor.Retry(3, 500*time.Millisecond, lb)

// --- 策略二：RetryAlways — 超时内无限重试 ---
// 只要未超时就一直重试，适合幂等的读操作
// ⚠ 注意：若后端持续失败，会在 timeout 到期前一直重试，CPU/连接开销较大
retryEp := executor.RetryAlways(2*time.Second, lb)

// --- 策略三：RetryWithCallback — 自定义回调精确控制 ---
// 每次失败后调用 cb(尝试次数 n, 本次错误)，由回调决定是否继续
retryEp := executor.RetryWithCallback(
    500*time.Millisecond, lb,
    func(n int, received error) (keepTrying bool, replacement error) {
        // 业务错误（参数非法、未授权）不应重试，立即返回
        if errors.Is(received, ErrInvalidArgument) || errors.Is(received, ErrUnauthorized) {
            return false, received
        }
        // 超过 5 次不再尝试，附加重试次数信息
        if n >= 5 {
            return false, fmt.Errorf("giving up after %d retries: %w", n, received)
        }
        // 其他错误（网络抖动、超时）继续重试
        return true, nil
    },
)
```

**回调签名：**

```go
// n：当前是第几次尝试（从 1 开始）
// received：本次调用返回的错误
// keepTrying：是否继续下一次重试
// replacement：替换最终返回的 error（nil 则使用 received）
type RetryCallback func(n int, received error) (keepTrying bool, replacement error)
```

#### 指数退避算法

每次重试失败后，执行器会等待一段时间再发起下一次请求。等待时间采用**带随机抖动的指数退避**策略：

```
初始等待：10ms
每次失败后：等待时间 × 2，再乘以 [0.5, 1.5) 随机系数（避免惊群）
上限：1 分钟
```

实际代码（`utils.Exponential`）：

```go
func Exponential(d time.Duration) time.Duration {
    d *= 2
    // 乘以 [0.5, 1.5) 的随机系数，引入抖动
    d = time.Duration(int64(float64(d.Nanoseconds()) * (rand.Float64() + 0.5)))
    if d > time.Minute {
        d = time.Minute
    }
    return d
}
```

典型退避序列（参考值）：

| 第 N 次失败 | 标称等待 | 实际范围（含抖动） |
|---|---|---|
| 1 | 20ms | 10–30ms |
| 2 | 40ms | 20–60ms |
| 3 | 80ms | 40–120ms |
| 4 | 160ms | 80–240ms |
| 5 | 320ms | 160–480ms |
| … | … | 上限 1 分钟 |

> **注意：** 退避等待发生在**失败后、下次重试前**。整体超时（`timeout`）从第一次调用开始计时，退避时间会消耗整体超时，因此建议 `timeout` 值要大于预期最大重试次数 × 单次平均耗时。

#### RetryError 错误诊断

当所有重试均失败时，返回 `executor.RetryError`，其中包含所有历史错误，便于排查：

```go
resp, err := retryEp(ctx, req)
if err != nil {
    var retryErr executor.RetryError
    if errors.As(err, &retryErr) {
        // RawErrors：每次尝试的原始错误列表（按顺序）
        for i, e := range retryErr.RawErrors {
            fmt.Printf("attempt %d: %v\n", i+1, e)
        }
        // Final：最终决定性错误（通常与最后一次 RawErrors 相同，
        // 或为 RetryCallback 通过 replacement 替换的错误）
        fmt.Printf("final error: %v\n", retryErr.Final)
    }
}
```

`RetryError.Error()` 字符串格式：

```
connection refused (previously: dial timeout; context deadline exceeded)
```

即：**最终错误 + 括号内所有历史错误**，一行输出完整重试轨迹，便于日志搜索。

#### 并发执行模型

每次重试均在**新 goroutine** 中并发执行，通过 channel 等待结果：

```go
// 内部伪代码
for i := 1; ; i++ {
    go func() {
        ep, _ := balancer.Endpoint() // 每次从 Balancer 取一个（不同）节点
        resp, err := ep(ctx, req)
        // 结果写入 responses / errs channel
    }()
    select {
    case <-ctx.Done():   return nil, ctx.Err()     // 整体超时
    case resp := <-responses: return resp, nil     // 成功
    case err := <-errs:  // 失败 → 检查 callback → 退避 → 继续循环
    }
}
```

**关键行为：**
- 每次重试都调用 `b.Endpoint()` 重新选节点，配合 RoundRobin 自动切换到下一个实例
- `context.WithTimeout` 在整个重试过程共享，所有 goroutine 均受同一超时控制
- goroutine 泄露保护：新 goroutine 中的 `ep(ctx, req)` 会因 `ctx` 被 cancel 而提前返回

---

### 7. SD — 服务发现

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
    "io"
    "time"
    "context"

    "github.com/dreamsxin/go-kit/sd/consul"
    "github.com/dreamsxin/go-kit/sd/endpointer"
    "github.com/dreamsxin/go-kit/sd/endpointer/balancer"
    "github.com/dreamsxin/go-kit/sd/endpointer/executor"
    "github.com/dreamsxin/go-kit/endpoint"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
)

// 1. 创建 Consul Instancer（自动监听健康实例变更，passingOnly=true 只返回健康节点）
instancer := consul.NewInstancer(consulClient, logger, "my-service", true)
defer instancer.Stop()

// 2. 工厂函数：instance 地址（"host:port"）→ Endpoint + io.Closer
//    每当 Consul 发现新实例时自动调用；实例下线时自动调用 Closer.Close()
factory := endpoint.Factory(func(instance string) (endpoint.Endpoint, io.Closer, error) {
    conn, err := grpc.NewClient(instance, grpc.WithTransportCredentials(insecure.NewCredentials()))
    if err != nil {
        return nil, nil, err
    }
    ep := makeGRPCEndpoint(conn) // 用 conn 构建具体的 gRPC client endpoint
    return ep, conn, nil
})

// 3. 创建 Endpointer（自动维护活跃端点列表，出错 5s 后失效缓存触发重建）
ep := endpointer.NewEndpointer(
    instancer, factory, logger,
    endpoint.InvalidateOnError(5*time.Second), // 可选：错误发现后多久清空端点缓存
)

// 4. 轮询负载均衡（无锁原子操作，高并发友好）
lb := balancer.NewRoundRobin(ep)

// 5. 带重试的请求 — 最多 3 次，整体超时 500ms，每次自动切换到不同节点
retryEp := executor.Retry(3, 500*time.Millisecond, lb)
resp, err := retryEp(ctx, request)
```

#### 重试执行器

SD 与 Executor 的配合很简单：用 `balancer.NewRoundRobin(ep)` 得到 Balancer，再传给 `executor.Retry` / `executor.RetryAlways` / `executor.RetryWithCallback` 即可。详细的重试策略、指数退避算法、`RetryError` 诊断和并发执行模型请参阅 [第 6 节 Executor — 重试执行器](#6-executor--重试执行器)。

```go
lb := balancer.NewRoundRobin(ep)

// 最多 3 次，整体超时 500ms，每次自动切换到不同节点
retryEp := executor.Retry(3, 500*time.Millisecond, lb)
resp, err := retryEp(ctx, request)
```

#### 不依赖 Consul 的单元测试

```go
// 用 instance/Cache 直接注入地址，完全内存驱动，无需启动 Consul
import (
    "github.com/dreamsxin/go-kit/sd/instance"
    "github.com/dreamsxin/go-kit/sd/events"
)

cache := instance.NewCache()
// 模拟两个实例上线
cache.Update(events.Event{Instances: []string{"127.0.0.1:8080", "127.0.0.1:8081"}})

ep := endpointer.NewEndpointer(cache, factory, logger)
lb := balancer.NewRoundRobin(ep)
retryEp := executor.Retry(2, 200*time.Millisecond, lb)

// 模拟一个实例下线
cache.Update(events.Event{Instances: []string{"127.0.0.1:8080"}})
```

#### 服务端 + 客户端完整场景示例

下面的示例将**服务注册（Server 端）**与**服务发现 + 负载均衡 + 重试（Client 端）**串联在一起，展示真实微服务场景下的完整用法。

**Server 端 — 启动时注册，退出时注销：**

```go
package main

import (
    "net/http"
    "os"
    "os/signal"
    "syscall"

    "github.com/dreamsxin/go-kit/log"
    "github.com/dreamsxin/go-kit/sd/consul"
    stdconsul "github.com/hashicorp/consul/api"
)

func main() {
    logger, _ := log.NewDevelopment()
    defer logger.Sync()

    // 1. 连接 Consul（默认 127.0.0.1:8500）
    consulAPI, _ := stdconsul.NewClient(stdconsul.DefaultConfig())
    consulClient := consul.NewClient(consulAPI)

    // 2. 注册服务，带 HTTP 健康检查
    registrar := consul.NewRegistrar(
        consulClient, logger,
        "user-service",    // 服务名
        "192.168.1.10",    // 本机地址（Consul 可访问的 IP）
        8080,              // 端口
        consul.IDRegistrarOptions("user-service-1"),
        consul.TagsRegistrarOptions([]string{"v1", "grpc"}),
        consul.CheckRegistrarOptions(&stdconsul.AgentServiceCheck{
            HTTP:                           "http://192.168.1.10:8080/health",
            Interval:                       "10s",
            DeregisterCriticalServiceAfter: "30s", // 健康检查失败 30s 后自动注销
        }),
    )
    registrar.Register()
    defer registrar.Deregister() // 进程退出时自动注销，避免僵尸节点

    // 3. 启动 HTTP 服务（省略业务代码）
    srv := &http.Server{Addr: ":8080", Handler: buildRouter()}
    go srv.ListenAndServe()

    // 4. 等待退出信号
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
}
```

**Client 端 — 自动发现、轮询、重试：**

```go
package main

import (
    "context"
    "io"
    "time"

    "github.com/dreamsxin/go-kit/log"
    "github.com/dreamsxin/go-kit/endpoint"
    "github.com/dreamsxin/go-kit/sd/consul"
    "github.com/dreamsxin/go-kit/sd/endpointer"
    "github.com/dreamsxin/go-kit/sd/endpointer/balancer"
    "github.com/dreamsxin/go-kit/sd/endpointer/executor"
    stdconsul "github.com/hashicorp/consul/api"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
)

func main() {
    logger, _ := log.NewDevelopment()
    defer logger.Sync()

    // 1. 连接 Consul
    consulAPI, _ := stdconsul.NewClient(stdconsul.DefaultConfig())
    consulClient := consul.NewClient(consulAPI)

    // 2. 订阅 user-service 的健康实例列表（长轮询，自动推送变更）
    instancer := consul.NewInstancer(
        consulClient, logger,
        "user-service",
        true, // passingOnly：只返回通过健康检查的实例
        consul.TagsInstancerOptions([]string{"v1"}), // 只发现带 v1 标签的实例
    )
    defer instancer.Stop()

    // 3. 工厂函数：每个实例地址建立一个 gRPC 连接
    factory := endpoint.Factory(func(instance string) (endpoint.Endpoint, io.Closer, error) {
        conn, err := grpc.NewClient(instance,
            grpc.WithTransportCredentials(insecure.NewCredentials()),
        )
        if err != nil {
            return nil, nil, err
        }
        ep := makeUserServiceEndpoint(conn) // 构建具体 gRPC call
        return ep, conn, nil
    })

    // 4. Endpointer 自动维护活跃连接池
    //    InvalidateOnError: Consul 报错后 10s 清空缓存，防止打到已下线实例
    ep := endpointer.NewEndpointer(
        instancer, factory, logger,
        endpoint.InvalidateOnError(10*time.Second),
    )

    // 5. 负载均衡：无锁原子轮询
    lb := balancer.NewRoundRobin(ep)

    // 6. 重试执行器：最多 3 次，整体超时 1s，每次自动切换到下一个节点
    retryEp := executor.Retry(3, time.Second, lb)

    // 7. 发起请求（与普通 endpoint.Endpoint 用法完全一致）
    ctx := context.Background()
    resp, err := retryEp(ctx, &GetUserRequest{ID: 42})
    _ = resp
    _ = err
}
```

> **关键点：**
> - Consul 实例变更（上下线）通过**事件驱动**实时推送，Client 端无需轮询
> - `endpoint.Factory` 负责连接生命周期管理，实例下线时自动 `Close()` 连接
> - 重试时每次调用 `lb.Endpoint()` 会选出不同实例，天然实现故障转移
> - 整个链路的**唯一公共接口**是 `endpoint.Endpoint`，SD/Retry/Middleware 均可自由组合

---

### 8. Log — 日志

日志模块是 `go.uber.org/zap` 的轻量封装，`log.Logger` 即 `*zap.Logger`，可直接使用全部 zap API。

```go
import "github.com/dreamsxin/go-kit/log"

// 开发模式：彩色输出，自动附带调用位置（DPanic 级别在开发时 panic）
logger, err := log.NewDevelopment()
if err != nil {
    panic(err)
}
defer logger.Sync()

// 生产模式：JSON 输出，高性能，建议正式部署使用
import "go.uber.org/zap"
logger, err = zap.NewProduction()

// 静默丢弃所有日志（用于测试或完全不需要日志的场景）
// 定义在 log/nop_logger.go：func NewNopLogger() *Logger { return zap.NewNop() }
logger = log.NewNopLogger()
```

**结构化字段写法：**

```go
logger.Info("user created",
    zap.String("username", "alice"),
    zap.Uint("user_id", 42),
    zap.Duration("took", time.Since(start)),
    zap.Error(err),
)
// 输出（生产模式 JSON）：
// {"level":"info","ts":1710000000.123,"msg":"user created","username":"alice","user_id":42,"took":"1.2ms","error":null}
```

**在 Transport 层接入错误日志：**

```go
import "github.com/dreamsxin/go-kit/transport"

// NewLogErrorHandler 将传输层错误写入 logger，不会影响业务流程
handler := httpserver.NewServer(
    ep,
    decodeRequest,
    httpserver.EncodeJSONResponse,
    httpserver.ServerErrorHandler(transport.NewLogErrorHandler(logger)),
)
```

---

### 9. 中间件链路打印 — 调用链 + 文件行号

#### 9.1 打印调用者文件名与行号

使用 `runtime.Caller` 在中间件内记录**调用者**的精确位置，便于快速定位问题。

```go
package middleware

import (
    "context"
    "fmt"
    "runtime"
    "time"

    "github.com/dreamsxin/go-kit/endpoint"
    "go.uber.org/zap"
)

// CallerInfo 返回调用栈第 skip 层的 "file:line" 字符串。
// skip=0 是 CallerInfo 本身，skip=1 是调用 CallerInfo 的函数，依此类推。
func CallerInfo(skip int) string {
    _, file, line, ok := runtime.Caller(skip)
    if !ok {
        return "unknown"
    }
    // 只保留最后两段路径，避免绝对路径过长
    short := file
    cnt := 0
    for i := len(file) - 1; i > 0; i-- {
        if file[i] == '/' {
            cnt++
            if cnt == 2 {
                short = file[i+1:]
                break
            }
        }
    }
    return fmt.Sprintf("%s:%d", short, line)
}

// TracingMiddleware 在每次调用时打印：
//   - 请求进入时的调用位置（文件:行号）
//   - 调用耗时
//   - 错误信息（如有）
func TracingMiddleware(logger *zap.Logger) endpoint.Middleware {
    return func(next endpoint.Endpoint) endpoint.Endpoint {
        return func(ctx context.Context, req interface{}) (resp interface{}, err error) {
            // skip=1：跳过 CallerInfo 本身，定位到匿名 Endpoint 函数调用处
            // 如需定位到更上层（调用方业务代码），可调大 skip 值
            caller := CallerInfo(1)
            start := time.Now()
            logger.Info("endpoint called",
                zap.String("caller", caller),
                zap.Any("request", req),
            )
            defer func() {
                logger.Info("endpoint returned",
                    zap.String("caller", caller),
                    zap.Duration("took", time.Since(start)),
                    zap.Error(err),
                )
            }()
            return next(ctx, req)
        }
    }
}
```

**输出示例：**

```
2026-03-15T10:00:00.000+0800  INFO  endpoint called   {"caller": "usersvc/handler.go:42", "request": {...}}
2026-03-15T10:00:00.003+0800  INFO  endpoint returned {"caller": "usersvc/handler.go:42", "took": "3ms", "error": null}
```

---

#### 9.2 Request-ID 链路透传中间件

在分布式场景中，通过 `context` 透传 `request-id`，让同一请求经过的每一层中间件都打印相同 ID，方便串联完整调用链。

```go
package middleware

import (
    "context"
    "time"

    "github.com/dreamsxin/go-kit/endpoint"
    "github.com/google/uuid"
    "go.uber.org/zap"
)

type contextKey string

const RequestIDKey contextKey = "request_id"

// RequestIDMiddleware 从 context 中读取 request-id（不存在则自动生成），
// 注入到 context 后传递给下游，同时在日志中透传。
func RequestIDMiddleware(logger *zap.Logger) endpoint.Middleware {
    return func(next endpoint.Endpoint) endpoint.Endpoint {
        return func(ctx context.Context, req interface{}) (interface{}, error) {
            rid, _ := ctx.Value(RequestIDKey).(string)
            if rid == "" {
                rid = uuid.New().String()
                ctx = context.WithValue(ctx, RequestIDKey, rid)
            }
            caller := CallerInfo(1)
            start := time.Now()
            logger.Info(">> request",
                zap.String("request_id", rid),
                zap.String("caller", caller),
                zap.Any("req", req),
            )
            resp, err := next(ctx, req)
            logger.Info("<< response",
                zap.String("request_id", rid),
                zap.String("caller", caller),
                zap.Duration("took", time.Since(start)),
                zap.Error(err),
            )
            return resp, err
        }
    }
}
```

**HTTP Server 端注入 Request-ID（从 Header 读取）：**

```go
import (
    "net/http"
    "context"
    httptransport "github.com/dreamsxin/go-kit/transport/http"
)

// 在 ServerBefore 中将 HTTP Header 中的 X-Request-ID 写入 context
func injectRequestID(ctx context.Context, r *http.Request) context.Context {
    rid := r.Header.Get("X-Request-ID")
    if rid == "" {
        rid = uuid.New().String()
    }
    return context.WithValue(ctx, middleware.RequestIDKey, rid)
}

handler := httpserver.NewServer(
    ep,
    decodeRequest,
    httpserver.EncodeJSONResponse,
    httpserver.ServerBefore(injectRequestID),
)
```

---

#### 9.3 完整链路中间件组合示例

```go
import (
    "github.com/dreamsxin/go-kit/endpoint"
    "github.com/dreamsxin/go-kit/endpoint/circuitbreaker"
    "github.com/dreamsxin/go-kit/endpoint/ratelimit"
    "github.com/sony/gobreaker"
    "golang.org/x/time/rate"
)

ep = endpoint.Chain(
    RequestIDMiddleware(logger),   // 1. 最外层：注入/透传 request-id，打印链路入口
    TracingMiddleware(logger),     // 2. 打印调用文件:行号 + 耗时
    ratelimit.NewErroringLimiter(  // 3. 限流
        rate.NewLimiter(rate.Every(time.Second), 100),
    ),
    circuitbreaker.Gobreaker(      // 4. 熔断
        gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "my-svc"}),
    ),
)(myEndpoint)
```

**执行顺序（洋葱模型）：**

```
HTTP Request
  └─ RequestIDMiddleware  pre   → 生成/读取 request-id，打印 ">> request"
       └─ TracingMiddleware pre  → 记录 caller file:line，计时开始
            └─ RateLimit        → 检查限流
                 └─ CircuitBreaker → 检查熔断
                      └─ myEndpoint（业务逻辑）
                 └─ CircuitBreaker post
            └─ RateLimit post
       └─ TracingMiddleware post → 打印耗时、error
  └─ RequestIDMiddleware  post  → 打印 "<< response"
```

**日志输出示例（同一请求的完整链路）：**

```
# RequestIDMiddleware 打印（最外层 pre）
INFO  >> request   {"request_id": "a1b2-c3d4", "caller": "transport_http.go:55", "req": {"name":"alice"}}

# TracingMiddleware 打印（第二层 pre）
INFO  endpoint called  {"caller": "transport_http.go:55", "request": {"name":"alice"}}

# ... 业务逻辑执行 ...

# TracingMiddleware 打印（第二层 post）
INFO  endpoint returned {"caller": "transport_http.go:55", "took": "2ms", "error": null}

# RequestIDMiddleware 打印（最外层 post）
INFO  << response  {"request_id": "a1b2-c3d4", "caller": "transport_http.go:55", "took": "2ms", "error": null}
```

> **提示：** 相同的 `request_id` 贯穿所有层，`grep request_id=a1b2-c3d4` 即可过滤出一次请求的完整调用链。

---

### 10. MetricsMiddleware — 内置指标收集

`endpoint.MetricsMiddleware` 是一个轻量的进程内指标中间件，无需引入外部 Prometheus/StatsD 依赖，适合快速埋点或单机场景。

```go
import "github.com/dreamsxin/go-kit/endpoint"

// 1. 创建指标收集器（普通结构体，零值可用）
metrics := &endpoint.Metrics{}

// 2. 包裹端点
ep = endpoint.MetricsMiddleware(metrics)(ep)

// 3. 任意时刻读取指标
fmt.Printf("total:   %d\n", metrics.RequestCount)
fmt.Printf("success: %d\n", metrics.SuccessCount)
fmt.Printf("errors:  %d\n", metrics.ErrorCount)
fmt.Printf("avg_ms:  %.2f\n",
    float64(metrics.TotalDuration.Milliseconds())/float64(metrics.RequestCount))
fmt.Printf("last_at: %s\n", metrics.LastRequestTime.Format(time.RFC3339))
```

**`Metrics` 字段说明：**

| 字段 | 类型 | 说明 |
|---|---|---|
| `RequestCount` | `int64` | 总调用次数 |
| `SuccessCount` | `int64` | 成功次数（err == nil） |
| `ErrorCount` | `int64` | 失败次数（err != nil） |
| `TotalDuration` | `time.Duration` | 所有调用累计耗时 |
| `LastRequestTime` | `time.Time` | 最近一次请求时间 |

> **注意：** `Metrics` 结构体字段使用非原子的 `int64` 累加，适合单 goroutine 或低并发场景。高并发生产环境建议配合 `sync/atomic` 封装，或使用 Prometheus `Counter`/`Histogram`。

**与 Prometheus 集成示意（扩展用法）：**

```go
import (
    "github.com/dreamsxin/go-kit/endpoint"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    reqTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "endpoint_requests_total",
        Help: "Total endpoint requests",
    }, []string{"method", "status"})

    reqDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "endpoint_duration_seconds",
        Buckets: prometheus.DefBuckets,
    }, []string{"method"})
)

// 自定义 Prometheus 中间件（替代或叠加 MetricsMiddleware）
func PrometheusMiddleware(method string) endpoint.Middleware {
    return func(next endpoint.Endpoint) endpoint.Endpoint {
        return func(ctx context.Context, req interface{}) (interface{}, error) {
            start := time.Now()
            resp, err := next(ctx, req)
            status := "ok"
            if err != nil {
                status = "error"
            }
            reqTotal.WithLabelValues(method, status).Inc()
            reqDuration.WithLabelValues(method).Observe(time.Since(start).Seconds())
            return resp, err
        }
    }
}
```

---

### 11. ErrorHandlingMiddleware — 错误包装

`endpoint.ErrorHandlingMiddleware` 将端点返回的错误包装为带操作名的 `ErrorWrapper`，便于在调用链中追溯错误来源，同时保留 `errors.As` / `errors.Is` 链式解包能力。

```go
import "github.com/dreamsxin/go-kit/endpoint"

// 包裹端点，所有错误自动带上 "CreateUser" 操作标签
ep = endpoint.ErrorHandlingMiddleware("CreateUser")(ep)

// 调用端点
_, err := ep(ctx, req)
if err != nil {
    // err.Error() → "CreateUser: connection refused"
    fmt.Println(err)

    // 解包原始错误（支持 errors.As / errors.Is）
    var wrapper *endpoint.ErrorWrapper
    if errors.As(err, &wrapper) {
        fmt.Println("operation:", wrapper.Operation) // "CreateUser"
        fmt.Println("cause:",     wrapper.Err)       // 原始 error
    }
}
```

**`ErrorWrapper` 结构：**

```go
type ErrorWrapper struct {
    Operation string // 操作名，如 "CreateUser"、"GetProfile"
    Err       error  // 原始错误（Unwrap() 返回此值）
}
```

**组合使用场景——多层服务调用时的错误溯源：**

```go
// 每层包裹自己的操作名，形成清晰的调用链
userEp  = endpoint.ErrorHandlingMiddleware("UserService.Get")(userEp)
orderEp = endpoint.ErrorHandlingMiddleware("OrderService.Create")(orderEp)

// 错误输出：
// "OrderService.Create: UserService.Get: user not found"
```

> **与 Transport 层配合：** `transport.DefaultErrorEncoder` 会检查 error 是否实现了 `StatusCoder` / `Headerer` 接口，可在自定义 Error 类型中同时实现这两个接口，精确控制 HTTP 响应状态码和 Header：
>
> ```go
> type AppError struct {
>     Code    int
>     Message string
> }
> func (e *AppError) Error() string       { return e.Message }
> func (e *AppError) StatusCode() int     { return e.Code }           // 控制 HTTP 状态码
> func (e *AppError) Headers() http.Header {                          // 追加响应 Header
>     h := http.Header{}
>     h.Set("X-Error-Code", strconv.Itoa(e.Code))
>     return h
> }
> ```

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
make gen          # HTTP + SQLite + model + swag（输出到 OUT=./generated）
make gen-http     # 仅 HTTP（最小化）
make gen-grpc     # HTTP + gRPC + model + swag
make gen-full     # HTTP + gRPC + model + swag + tests

# 自定义参数
make gen IDL=./myapp/idl.go OUT=./myapp IMPORT=github.com/myorg/myapp
make gen DB_DRIVER=mysql

# 启动生成的示例服务（需先 make gen）
make run-demo                    # 默认使用 OUT=./generated，端口 :8080
make run-demo OUT=./myapp HTTP_PORT=:9090

# 重新生成 Swagger 文档
make swag-demo OUT=./myapp

# 从 .proto 文件重新生成 pb.go（需安装 protoc，Linux/macOS）
make proto OUT=./myapp
# Windows 下直接使用 microgen 生成时打印的 protoc 命令手动执行

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
