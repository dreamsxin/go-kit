# go-kit — Go Microservice Framework

[![Go Version](https://img.shields.io/badge/go-1.21+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE.txt)

Production-ready microservice framework for Go.  Independent components — import only what you need.

---

## 30-Second Start

```go
package main

import (
    "context"
    "github.com/dreamsxin/go-kit/kit"
)

type HelloReq  struct { Name string `json:"name"` }
type HelloResp struct { Message string `json:"message"` }

func main() {
    svc := kit.New(":8080")
    svc.Handle("/hello", kit.JSON[HelloReq](func(ctx context.Context, req HelloReq) (any, error) {
        return HelloResp{Message: "Hello, " + req.Name + "!"}, nil
    }))
    svc.Run()
}
```

```bash
go run ./main.go
curl -X POST http://localhost:8080/hello -d '{"name":"world"}'
# {"message":"Hello, world!"}
curl http://localhost:8080/health
# {"status":"ok"}
```

---

## 5-Minute Guide

### Add middleware

```go
var metrics endpoint.Metrics

svc := kit.New(":8080",
    kit.WithRateLimit(100),              // 100 req/s, reject over limit
    kit.WithCircuitBreaker(5),           // open after 5 consecutive failures
    kit.WithTimeout(5*time.Second),      // per-request deadline
    kit.WithMetrics(&metrics),           // built-in counters
    kit.WithRequestID(),                 // inject X-Request-ID
)
```

### Type-safe endpoints (no runtime panics)

```go
// TypedEndpoint[Req, Resp] — compile-time type checking
var createUser endpoint.TypedEndpoint[CreateUserReq, CreateUserResp] =
    func(ctx context.Context, req CreateUserReq) (CreateUserResp, error) {
        // req is already the right type — no .(CreateUserReq) needed
        return userService.Create(ctx, req)
    }

// Apply middleware, recover type safety
ep := endpoint.Unwrap[CreateUserReq, CreateUserResp](
    endpoint.NewTypedBuilder(createUser).
        WithMetrics(&metrics).
        WithErrorHandling("CreateUser").
        WithTimeout(5 * time.Second).
        WithBackpressure(200).           // max 200 concurrent requests
        WithTracing().                   // inject trace/request IDs
        Build(),
)
resp, err := ep(ctx, CreateUserReq{Name: "alice"})
```

### Service discovery

```go
// One line — Consul → RoundRobin → Retry, all wired
ep := sd.NewEndpointWithDefaults(instancer, factory, logger)

// Or with custom settings
ep = sd.NewEndpoint(instancer, factory, logger,
    sd.WithMaxRetries(3),
    sd.WithTimeout(500*time.Millisecond),
    sd.WithInvalidateOnError(5*time.Second),
)
```

### Generate a full service from a database

```bash
# From any existing database — generates HTTP service + GORM models + Swagger
./microgen.exe -from-db -driver sqlite -dsn app.db \
    -out ./gen -import github.com/myorg/myapp -service MyApp

cd gen && go mod tidy && go run ./cmd/main.go
```

---

## Architecture

```
Request
  │
  ▼
Transport (HTTP / gRPC)
  │  decode request
  ▼
Endpoint  ◄── Middleware chain (logging, metrics, rate limit, circuit breaker, tracing)
  │
  ▼
Business Logic  (pure functions, no framework dependency)
  │
  ▼
Transport
  │  encode response
  ▼
Response
```

**Design principle:** each layer is independent.  Business logic never imports transport or middleware packages.

---

## Component Map

| Component | Package | Purpose |
|-----------|---------|---------|
| `Endpoint` | `endpoint` | Core callable unit |
| `TypedEndpoint[Req,Resp]` | `endpoint` | Type-safe endpoint (no interface{}) |
| `Builder` | `endpoint` | Fluent middleware assembly |
| `MetricsMiddleware` | `endpoint` | Request counters |
| `LoggingMiddleware` | `endpoint` | Structured request logging |
| `TracingMiddleware` | `endpoint` | Trace/request ID propagation |
| `BackpressureMiddleware` | `endpoint` | Concurrency limiter |
| `TimeoutMiddleware` | `endpoint` | Per-request deadline |
| `ErrorHandlingMiddleware` | `endpoint` | Wrap errors with operation name |
| `Gobreaker` | `endpoint/circuitbreaker` | sony/gobreaker circuit breaker |
| `HandyBreaker` | `endpoint/circuitbreaker` | streadway/handy circuit breaker |
| `Hystrix` | `endpoint/circuitbreaker` | afex/hystrix-go circuit breaker |
| `NewErroringLimiter` | `endpoint/ratelimit` | Reject over limit |
| `NewDelayingLimiter` | `endpoint/ratelimit` | Wait for token |
| `NewJSONServer[T]` | `transport/http/server` | Zero-boilerplate HTTP server |
| `NewJSONServerWithMiddleware[T]` | `transport/http/server` | HTTP server + middleware |
| `JSONErrorEncoder` | `transport/http/server` | Default JSON error responses |
| `NewJSONClient[T]` | `transport/http/client` | Zero-boilerplate HTTP client |
| `NewJSONClientWithRetry[T]` | `transport/http/client` | HTTP client + timeout |
| `NewEndpoint` | `sd` | SD + balancer + retry in one call |
| `NewEndpointWithDefaults` | `sd` | SD with production defaults |
| `instance.Cache` | `sd/instance` | In-memory SD (no Consul needed) |
| `kit.New` | `kit` | High-level service builder |
| `kit.JSON[T]` | `kit` | One-line JSON handler |

---

## Production Patterns

### Full middleware stack

```go
var metrics endpoint.Metrics

ep := endpoint.NewBuilder(baseEndpoint).
    WithTracing().                                          // trace/request ID
    WithLogging(logger, "CreateUser").                      // structured logs
    WithMetrics(&metrics).                                  // counters
    WithErrorHandling("CreateUser").                        // wrap errors
    WithTimeout(5 * time.Second).                           // deadline
    WithBackpressure(200).                                  // concurrency limit
    Use(circuitbreaker.Gobreaker(cb)).                      // circuit breaker
    Use(ratelimit.NewErroringLimiter(limiter)).              // rate limit
    Build()
```

### HTTP server with all features

```go
handler := server.NewJSONServerWithMiddleware[CreateUserReq](
    func(ctx context.Context, req CreateUserReq) (any, error) {
        return userService.Create(ctx, req)
    },
    func(b *endpoint.Builder) *endpoint.Builder {
        return b.
            WithTracing().
            WithMetrics(&metrics).
            WithTimeout(5 * time.Second).
            WithBackpressure(100).
            Use(circuitbreaker.Gobreaker(cb))
    },
    // JSONErrorEncoder is the default — no need to pass it explicitly
)
```

### Graceful shutdown

```go
srv := &http.Server{Addr: ":8080", Handler: mux}

go func() { srv.ListenAndServe() }()

quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
<-quit

ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
srv.Shutdown(ctx)
```

### Distributed tracing (without external system)

```go
// Inject trace ID from incoming HTTP header
before := server.ServerBefore(func(ctx context.Context, r *http.Request) context.Context {
    if id := r.Header.Get("X-Trace-ID"); id != "" {
        ctx = endpoint.WithTraceID(ctx, endpoint.TraceID(id))
    }
    return ctx
})

// Read in business logic or middleware
traceID := endpoint.TraceIDFromContext(ctx)
reqID   := endpoint.RequestIDFromContext(ctx)
```

### Backpressure for large-scale systems

```go
// Prevent cascading failures when downstream slows down
var inflight int64
ep = endpoint.InFlightMiddleware(500, &inflight)(ep)

// Expose as metric
mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
    fmt.Fprintf(w, `{"inflight":%d,"requests":%d}`,
        atomic.LoadInt64(&inflight), metrics.RequestCount)
})
```

### Retry with custom strategy

```go
// Stop immediately on non-retryable errors, retry up to 5 times otherwise
retryEp := executor.RetryWithCallback(2*time.Second, lb,
    func(n int, err error) (keepTrying bool, replacement error) {
        if errors.Is(err, ErrInvalidArgument) || errors.Is(err, ErrNotFound) {
            return false, err  // don't retry client errors
        }
        if n >= 5 {
            return false, fmt.Errorf("gave up after %d attempts: %w", n, err)
        }
        return true, nil
    },
)
```

### Consul service registration + discovery

```go
// Registration (on startup)
registrar := consul.NewRegistrar(client, logger, "user-service", "10.0.0.1", 8080,
    consul.IDRegistrarOptions("user-service-1"),
    consul.CheckRegistrarOptions(&stdconsul.AgentServiceCheck{
        HTTP: "http://10.0.0.1:8080/health", Interval: "10s",
    }),
)
registrar.Register()
defer registrar.Deregister()

// Discovery (in client)
instancer := consul.NewInstancer(client, logger, "user-service", true)
defer instancer.Stop()

ep := sd.NewEndpointWithDefaults(instancer, grpcFactory, logger)
```

---

## Code Generation

Generate a complete microservice from an IDL file or existing database in seconds.

### From IDL

```go
// idl.go — define your service interface
type UserService interface {
    CreateUser(ctx context.Context, req CreateUserRequest) (CreateUserResponse, error)
    GetUser(ctx context.Context, req GetUserRequest)       (GetUserResponse,    error)
    ListUsers(ctx context.Context, req ListUsersRequest)   (ListUsersResponse,  error)
}
```

```bash
./microgen.exe -idl ./idl.go -out ./gen -import github.com/myorg/usersvc \
    -protocols http,grpc -model -driver sqlite -swag
cd gen && go mod tidy && go run ./cmd/main.go
```

### From database

```bash
# SQLite
./microgen.exe -from-db -driver sqlite -dsn app.db \
    -out ./gen -import github.com/myorg/app -service MyApp

# MySQL
./microgen.exe -from-db -driver mysql \
    -dsn "root:pass@tcp(127.0.0.1:3306)/mydb?charset=utf8mb4&parseTime=True" \
    -dbname mydb -out ./gen -import github.com/myorg/app -service MyApp

# Add new tables later (non-destructive)
./microgen.exe -from-db -driver mysql -dsn "..." -dbname mydb \
    -add-tables "orders,products" -out ./gen -import github.com/myorg/app
```

### Generated structure

```
gen/
├── cmd/main.go              # Entry point with graceful shutdown
├── service/{svc}/           # Business logic stub
├── endpoint/{svc}/          # Endpoint + middleware wiring
├── transport/{svc}/         # HTTP + gRPC handlers
├── model/model.go           # GORM models
├── repository/repository.go # Data access layer
├── config/                  # YAML config + loader
├── docs/                    # Swagger stub
└── idl.go                   # Service interface
```

---

## Examples

| Example | What it shows |
|---------|--------------|
| `go run ./examples/quickstart` | Minimal service — 30 lines |
| `go run ./examples/best_practice` | Production patterns |
| `go run ./examples/middleware` | Every middleware demonstrated |
| `go run ./examples/httpclient` | HTTP client patterns |
| `go run ./examples/sd` | Service discovery end-to-end |
| `go test ./examples/transport/...` | HTTP + gRPC transport tests |

```bash
# Run all example tests
python tools/test_examples.py
```

---

## Project Structure

```
go-kit/
├── kit/                     # High-level API (rapid prototyping)
├── endpoint/                # Core: Endpoint, TypedEndpoint, Builder, Middleware
│   ├── circuitbreaker/      # Gobreaker, Hystrix, HandyBreaker
│   └── ratelimit/           # ErroringLimiter, DelayingLimiter
├── transport/
│   ├── http/server/         # NewJSONServer, NewJSONServerWithMiddleware
│   ├── http/client/         # NewJSONClient, NewJSONClientWithRetry
│   └── grpc/                # gRPC server + client
├── sd/                      # Service discovery
│   ├── consul/              # Consul instancer + registrar
│   ├── endpointer/          # Endpointer, RoundRobin, Retry
│   └── instance/            # In-memory cache (testing)
├── log/                     # zap wrapper
├── utils/                   # Exponential backoff
├── examples/                # Runnable examples
└── cmd/microgen/            # Code generator
```

---

## Dependencies

| Package | Purpose |
|---------|---------|
| `go.uber.org/zap` | Structured logging |
| `google.golang.org/grpc` | gRPC transport |
| `github.com/gorilla/mux` | HTTP routing |
| `github.com/sony/gobreaker` | Circuit breaker |
| `golang.org/x/time` | Token bucket rate limiting |
| `github.com/hashicorp/consul/api` | Service discovery |

---

## Contributing

```bash
make test    # run all tests with race detection
make lint    # static analysis
make build   # build all packages
```

New features require tests.  See `CONTRIBUTING.md` for details.

---

## License

[MIT](LICENSE.txt)
