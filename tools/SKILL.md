---
description: "go-kit microservice framework — how to build Go microservices with this framework"
---

# go-kit Framework Skill

This skill teaches you how to build Go microservices using the go-kit framework in this repository.

## Repository Layout

```
github.com/dreamsxin/go-kit
├── kit/              ← High-level API (start here for new projects)
├── endpoint/         ← Core: Endpoint, TypedEndpoint, Builder, Middleware
│   ├── circuitbreaker/
│   └── ratelimit/
├── transport/
│   ├── http/server/  ← NewJSONServer, NewJSONServerWithMiddleware
│   ├── http/client/  ← NewJSONClient, NewJSONClientWithRetry
│   └── grpc/
├── sd/               ← Service discovery: NewEndpoint, NewEndpointWithDefaults
│   ├── consul/
│   ├── endpointer/
│   └── instance/     ← In-memory SD for tests
├── log/              ← *zap.Logger wrapper
├── examples/         ← Runnable examples (start reading here)
└── cmd/microgen/     ← Code generator
```

## 30-Second Service

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
    svc.JSON("/hello", func(ctx context.Context, req any) (any, error) {
        r := req.(HelloReq)
        return HelloResp{Message: "Hello, " + r.Name + "!"}, nil
    })
    svc.Run()
}
```

For typed (no type assertions):

```go
svc.Handle("/hello", kit.JSON[HelloReq](func(ctx context.Context, req HelloReq) (any, error) {
    return HelloResp{Message: "Hello, " + req.Name + "!"}, nil
}))
```

## Production Service Pattern

```go
package main

import (
    "context"
    "time"
    "github.com/sony/gobreaker"
    "golang.org/x/time/rate"
    "github.com/dreamsxin/go-kit/endpoint"
    "github.com/dreamsxin/go-kit/endpoint/circuitbreaker"
    "github.com/dreamsxin/go-kit/endpoint/ratelimit"
    kitlog "github.com/dreamsxin/go-kit/log"
    httpserver "github.com/dreamsxin/go-kit/transport/http/server"
)

// Step 1: pure business logic (no framework imports)
type CreateUserReq  struct { Name string `json:"name"` }
type CreateUserResp struct { ID uint `json:"id"` }

func createUser(_ context.Context, req CreateUserReq) (CreateUserResp, error) {
    // ... business logic
    return CreateUserResp{ID: 1}, nil
}

// Step 2: wire with middleware
func main() {
    logger, _ := kitlog.NewDevelopment()
    defer logger.Sync()

    var metrics endpoint.Metrics

    // TypedEndpoint — compile-time type safety
    base := endpoint.TypedEndpoint[CreateUserReq, CreateUserResp](createUser)

    // Build middleware chain
    ep := endpoint.Unwrap[CreateUserReq, CreateUserResp](
        endpoint.NewTypedBuilder(base).
            WithMetrics(&metrics).
            WithErrorHandling("CreateUser").
            WithTimeout(5 * time.Second).
            WithTracing().
            WithBackpressure(200).
            Use(circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(
                gobreaker.Settings{Name: "CreateUser"},
            ))).
            Use(ratelimit.NewErroringLimiter(
                rate.NewLimiter(rate.Every(time.Second), 100),
            )).
            Build(),
    )

    // Step 3: HTTP handler — automatic JSON, default error encoder
    handler := httpserver.NewJSONServer[CreateUserReq](
        func(ctx context.Context, req CreateUserReq) (any, error) {
            return ep(ctx, req)
        },
    )

    http.Handle("/users", handler)
    http.ListenAndServe(":8080", nil)
}
```

## Key APIs

### endpoint package

```go
// Untyped (legacy compatible)
var ep endpoint.Endpoint = func(ctx context.Context, req any) (any, error) { ... }

// Typed (recommended for new code)
var ep endpoint.TypedEndpoint[Req, Resp] = func(ctx context.Context, req Req) (Resp, error) { ... }

// Builder — fluent middleware assembly
ep := endpoint.NewBuilder(base).
    WithMetrics(&metrics).          // built-in counters
    WithErrorHandling("op").        // wrap errors with operation name
    WithTimeout(5*time.Second).     // per-request deadline
    WithTracing().                  // inject trace/request IDs
    WithBackpressure(200).          // max concurrent requests
    WithLogging(logger, "op").      // structured logging
    Use(myMiddleware).              // any custom middleware
    Build()

// Recover type safety after middleware
typed := endpoint.Unwrap[Req, Resp](ep)
resp, err := typed(ctx, req)  // no type assertions needed
```

### transport/http/server

```go
// Simplest — automatic JSON, default error encoder
handler := server.NewJSONServer[Req](func(ctx context.Context, req Req) (any, error) {
    return resp, nil
})

// With middleware
handler := server.NewJSONServerWithMiddleware[Req](
    myHandler,
    func(b *endpoint.Builder) *endpoint.Builder {
        return b.WithTimeout(5*time.Second).Use(cb)
    },
)

// Custom error encoder
server.ServerErrorEncoder(server.JSONErrorEncoder)  // {"error": "..."}

// Hooks
server.ServerBefore(func(ctx context.Context, r *http.Request) context.Context { ... })
server.ServerAfter(func(ctx context.Context, r *http.Request, w *server.InterceptingWriter) context.Context { ... })
server.ServerFinalizer(func(ctx context.Context, r *http.Request, w *server.InterceptingWriter) { ... })
```

### transport/http/client

```go
// Typed client — automatic JSON
ep, err := client.NewJSONClient[RespType](http.MethodPost, "http://host/path")
resp, err := ep(ctx, reqValue)
result := resp.(RespType)

// With timeout
ep, err := client.NewJSONClientWithRetry[RespType](http.MethodGet, url, 5*time.Second)

// Hooks
client.ClientBefore(func(ctx context.Context, r *http.Request) context.Context {
    r.Header.Set("Authorization", "Bearer "+token)
    return ctx
})
```

### sd package (service discovery)

```go
// One-liner with defaults (3 retries, 500ms timeout, 5s invalidate)
ep := sd.NewEndpointWithDefaults(instancer, factory, logger)

// Custom settings
ep := sd.NewEndpoint(instancer, factory, logger,
    sd.WithMaxRetries(3),
    sd.WithTimeout(500*time.Millisecond),
    sd.WithInvalidateOnError(5*time.Second),
)

// In-memory for tests (no Consul needed)
cache := instance.NewCache()
cache.Update(events.Event{Instances: []string{"host1:8080", "host2:8080"}})
ep := sd.NewEndpoint(cache, factory, logger)

// Consul
instancer := consul.NewInstancer(consulClient, logger, "service-name", true)
defer instancer.Stop()
```

### log package

```go
// Development (coloured, human-readable)
logger, _ := log.NewDevelopment()
defer logger.Sync()

// Production (JSON)
logger, _ = zap.NewProduction()

// Silent (tests)
logger = log.NewNopLogger()

// Usage
logger.Sugar().Infof("user created: %s", name)
logger.Info("request", zap.String("method", "POST"), zap.Duration("took", d))
```

## Code Generation

### From IDL file

```go
// 1. Define your service in idl.go
package myapp

import "context"

type UserModel struct {
    ID   uint   `json:"id"   gorm:"primaryKey;autoIncrement"`
    Name string `json:"name" gorm:"not null"`
}

type CreateUserRequest  struct { Name string `json:"name"` }
type CreateUserResponse struct { User *UserModel `json:"user"` }

type UserService interface {
    CreateUser(ctx context.Context, req CreateUserRequest) (CreateUserResponse, error)
}
```

```bash
# 2. Generate
./microgen.exe -idl ./idl.go -out ./gen -import github.com/myorg/myapp \
    -protocols http -model -driver sqlite -config -swag

# 3. Run
cd gen && go mod tidy && go run ./cmd/main.go
```

### From database

```bash
# SQLite
./microgen.exe -from-db -driver sqlite -dsn app.db \
    -out ./gen -import github.com/myorg/app -service MyApp -config

# MySQL
./microgen.exe -from-db -driver mysql \
    -dsn "root:pass@tcp(127.0.0.1:3306)/mydb?charset=utf8mb4&parseTime=True" \
    -dbname mydb -out ./gen -import github.com/myorg/app -service MyApp -config

# Add tables later (non-destructive)
./microgen.exe -from-db -driver sqlite -dsn app.db \
    -add-tables "orders,products" -out ./gen -import github.com/myorg/app
```

### Generated project structure

```
gen/
├── cmd/main.go              # Entry point (zap logger, graceful shutdown)
├── service/{svc}/           # Business logic stub — fill in your logic here
├── endpoint/{svc}/          # Middleware: circuit breaker, rate limit, logging
├── transport/{svc}/         # HTTP handlers + /debug/routes endpoint
├── model/model.go           # GORM models (with -model)
├── repository/repository.go # Data access layer
├── config/config.yaml       # All settings: server, db, middleware, debug
├── config/config.go         # Typed config loader
└── idl.go                   # Service interface
```

### Config-driven middleware (generated projects)

```go
// endpoints.go — MakeServerEndpointsWithConfig wires real middleware from config
cfg, _ := config.Load("config/config.yaml")
endpoints := userserviceEndpoint.MakeServerEndpointsWithConfig(svc, logger,
    userserviceEndpoint.MiddlewareConfig{
        CBEnabled:          cfg.Middleware.CircuitBreaker.Enabled,
        CBFailureThreshold: uint32(cfg.Middleware.CircuitBreaker.FailureThreshold),
        CBTimeout:          cfg.Middleware.CircuitBreaker.Timeout,
        RLEnabled:          cfg.Middleware.RateLimit.Enabled,
        RLRps:              cfg.Middleware.RateLimit.RequestsPerSecond,
        Timeout:            30 * time.Second,
    })
```

## Debug Endpoints (generated projects)

Every generated service exposes:

```bash
GET /health          # {"status":"ok","service":"UserService"}
GET /debug/routes    # [{"method":"POST","path":"/userservice/createuser","handler":"CreateUser"}, ...]
GET /swagger/        # Swagger UI (with -swag flag)
```

Control via `config/config.yaml`:
```yaml
debug:
  routes_enabled: true   # disable in production
  print_routes: true     # print route table on startup
```

## Testing Patterns

```go
// Unit test an endpoint directly
func TestCreateUser(t *testing.T) {
    ep := MakeCreateUserEndpoint(NewService(nil))
    resp, err := ep(context.Background(), CreateUserRequest{Name: "alice"})
    // ...
}

// Integration test with in-memory SD
func TestWithSD(t *testing.T) {
    cache := instance.NewCache()
    cache.Update(events.Event{Instances: []string{"127.0.0.1:8080"}})
    ep := sd.NewEndpoint(cache, myFactory, log.NewNopLogger())
    // ...
}

// HTTP handler test
func TestHandler(t *testing.T) {
    handler := server.NewJSONServer[HelloReq](myHandler)
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"test"}`)))
    // ...
}
```

## Common Mistakes

| Mistake | Fix |
|---------|-----|
| Using `RetryMiddleware` on server endpoints | Only use retry on client endpoints |
| Ignoring `logger.Sync()` | Always `defer logger.Sync()` |
| Type-asserting response without checking | Use `TypedEndpoint` + `Unwrap` |
| Hardcoding DSN in code | Use `config/config.yaml` + `-config` flag |
| Not handling `ErrBackpressure` | Check `errors.Is(err, endpoint.ErrBackpressure)` |
