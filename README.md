# go-kit — Go Microservice Framework

[![Go Version](https://img.shields.io/badge/go-1.21+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE.txt)
[![Go Report Card](https://goreportcard.com/badge/github.com/dreamsxin/go-kit)](https://goreportcard.com/report/github.com/dreamsxin/go-kit)

Production-ready Go microservice framework.  Independent components — import only what you need.

---

## Table of Contents

- [Install](#install)
- [30-Second Start](#30-second-start)
- [5-Minute Guide](#5-minute-guide)
- [Architecture](#architecture)
- [Component Map](#component-map)
- [Production Patterns](#production-patterns)
- [Code Generation (microgen)](#code-generation-microgen)
- [Examples](#examples)
- [Testing](#testing)
- [Project Structure](#project-structure)
- [Dependencies](#dependencies)
- [Contributing](#contributing)

---

## Install

```bash
go get github.com/dreamsxin/go-kit
```

Requires Go 1.21+.

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
curl -X POST http://localhost:8080/hello \
     -H "Content-Type: application/json" \
     -d '{"name":"world"}'
# {"message":"Hello, world!"}

curl http://localhost:8080/health
# {"status":"ok"}
```

---

## 5-Minute Guide

The four building blocks of a go-kit service, shown as one cohesive example.

### Step 1 — Pure business logic

```go
// No framework imports — easy to unit test
type CreateUserReq  struct { Name  string `json:"name"` }
type CreateUserResp struct { ID    uint   `json:"id"` }

func createUser(_ context.Context, req CreateUserReq) (CreateUserResp, error) {
    if req.Name == "" {
        return CreateUserResp{}, errors.New("name is required")
    }
    id := db.Insert(req.Name) // your real logic here
    return CreateUserResp{ID: id}, nil
}
```

### Step 2 — Wrap with middleware (type-safe)

```go
// TypedEndpoint[Req, Resp] — compile-time type checking, no interface{} assertions
base := endpoint.TypedEndpoint[CreateUserReq, CreateUserResp](createUser)

var metrics endpoint.Metrics

// Builder assembles the middleware chain in declaration order
ep := endpoint.Unwrap[CreateUserReq, CreateUserResp](
    endpoint.NewTypedBuilder(base).
        WithTracing().                                    // inject trace/request IDs
        WithLogging(logger, "CreateUser").                // structured zap logs
        WithMetrics(&metrics).                            // request counters
        WithErrorHandling("CreateUser").                  // wrap errors with op name
        WithTimeout(5 * time.Second).                     // per-request deadline
        WithBackpressure(200).                            // max 200 concurrent calls
        Use(circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(
            gobreaker.Settings{Name: "CreateUser"},
        ))).
        Use(ratelimit.NewErroringLimiter(
            rate.NewLimiter(rate.Every(time.Second), 100),
        )).
        Build(),
)

// Call site is fully type-safe — no .(CreateUserResp) assertion needed
resp, err := ep(ctx, CreateUserReq{Name: "alice"})
fmt.Println(resp.ID)
```

### Step 3 — Expose over HTTP

```go
// NewJSONServerWithMiddleware wires handler + middleware + HTTP in one call.
// JSONErrorEncoder is the default — errors become {"error":"..."} automatically.
handler := server.NewJSONServerWithMiddleware[CreateUserReq](
    func(ctx context.Context, req CreateUserReq) (any, error) {
        return ep(ctx, req)
    },
    func(b *endpoint.Builder) *endpoint.Builder {
        return b.WithTimeout(5*time.Second).WithBackpressure(100)
    },
)

mux := http.NewServeMux()
mux.Handle("/users", handler)
mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
    fmt.Fprintf(w, `{"status":"ok","requests":%d}`, metrics.RequestCount)
})

srv := &http.Server{Addr: ":8080", Handler: mux}
go srv.ListenAndServe()
```

### Step 4 — Service registration + discovery

```go
// ── Server side: register with Consul on startup ──────────────────────────
registrar := consul.NewRegistrar(consulClient, logger,
    "user-service", "10.0.0.1", 8080,
    consul.IDRegistrarOptions("user-service-1"),
    consul.CheckRegistrarOptions(&stdconsul.AgentServiceCheck{
        HTTP: "http://10.0.0.1:8080/health", Interval: "10s",
    }),
)
registrar.Register()
defer registrar.Deregister()

// ── Client side: discover + load-balance + retry in one line ─────────────
instancer := consul.NewInstancer(consulClient, logger, "user-service", true)
defer instancer.Stop()

// factory creates an endpoint for each discovered instance address
factory := endpoint.Factory(func(addr string) (endpoint.Endpoint, io.Closer, error) {
    conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
    if err != nil {
        return nil, nil, err
    }
    return makeGRPCEndpoint(conn), conn, nil
})

// NewEndpointWithDefaults: Instancer → Endpointer → RoundRobin → Retry (3×, 500ms)
clientEp := sd.NewEndpointWithDefaults(instancer, factory, logger)

// Call exactly like a local endpoint — retries and failover are transparent
resp, err := clientEp(ctx, CreateUserReq{Name: "alice"})
```

### Putting it all together (no Consul needed for local dev)

```go
// Use instance.Cache as a drop-in Instancer — no external service required
cache := instance.NewCache()
cache.Update(events.Event{Instances: []string{"localhost:8080", "localhost:8081"}})

ep := sd.NewEndpoint(cache, factory, logger,
    sd.WithMaxRetries(3),
    sd.WithTimeout(500*time.Millisecond),
)
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
Endpoint  ◄── Middleware chain
  │             logging · metrics · tracing · backpressure
  │             timeout · circuit breaker · rate limit
  ▼
Business Logic  (pure functions, zero framework imports)
  │
  ▼
Transport
  │  encode response
  ▼
Response
```

**Design principle:** each layer is independent.  Business logic never imports transport or middleware packages — it stays pure and easy to test.

---

## Component Map

### endpoint/

| Symbol | Description |
|--------|-------------|
| `Endpoint` | `func(ctx, any) (any, error)` — core callable unit |
| `TypedEndpoint[Req,Resp]` | Compile-time type-safe endpoint |
| `Unwrap[Req,Resp](ep)` | Recover type safety after middleware |
| `NewBuilder(ep)` | Fluent middleware assembly |
| `Builder.WithMetrics(&m)` | Attach request counters |
| `Builder.WithErrorHandling("op")` | Wrap errors with operation name |
| `Builder.WithTimeout(d)` | Per-request deadline |
| `Builder.WithTracing()` | Inject trace/request IDs |
| `Builder.WithBackpressure(n)` | Concurrency limiter |
| `Builder.WithLogging(logger, "op")` | Structured zap logging |
| `Builder.Use(mw)` | Append any custom middleware |
| `LoggingMiddleware(logger, "op")` | Standalone logging middleware |
| `MetricsMiddleware(&m)` | Standalone metrics middleware |
| `TracingMiddleware()` | Standalone tracing middleware |
| `BackpressureMiddleware(n)` | Standalone concurrency limiter |
| `InFlightMiddleware(n, &counter)` | Concurrency limiter + live counter |
| `TimeoutMiddleware(d)` | Standalone timeout middleware |
| `ErrorHandlingMiddleware("op")` | Standalone error wrapper |
| `Failer` interface | Carry business errors in response value |

### endpoint/circuitbreaker/

| Symbol | Description |
|--------|-------------|
| `Gobreaker(cb)` | sony/gobreaker circuit breaker |
| `HandyBreaker(cb)` | streadway/handy circuit breaker |
| `Hystrix("cmd")` | afex/hystrix-go circuit breaker |

### endpoint/ratelimit/

| Symbol | Description |
|--------|-------------|
| `NewErroringLimiter(lim)` | Reject immediately when over limit |
| `NewDelayingLimiter(lim)` | Wait for token (respects ctx deadline) |

### transport/http/server/

| Symbol | Description |
|--------|-------------|
| `NewJSONServer[T](handler, opts...)` | Zero-boilerplate JSON server (default: JSONErrorEncoder) |
| `NewJSONServerWithMiddleware[T](handler, mwFn, opts...)` | JSON server + inline middleware |
| `NewServer(ep, dec, enc, opts...)` | Full-control server |
| `JSONErrorEncoder` | Default JSON error encoder `{"error":"..."}` |
| `EncodeJSONResponse` | JSON response encoder |
| `DecodeJSONRequest[T]()` | Typed JSON request decoder |
| `ServerBefore(...)` | Pre-decode context hooks |
| `ServerAfter(...)` | Post-endpoint hooks |
| `ServerFinalizer(...)` | Always-run hooks (latency logging) |

### transport/http/client/

| Symbol | Description |
|--------|-------------|
| `NewJSONClient[Resp](method, url, opts...)` | Zero-boilerplate typed HTTP client |
| `NewJSONClientWithRetry[Resp](method, url, timeout, opts...)` | HTTP client + timeout |
| `NewClient(method, url, enc, dec, opts...)` | Full-control client |
| `ClientBefore(...)` | Pre-send hooks (inject headers) |
| `ClientAfter(...)` | Post-receive hooks (read response headers) |
| `ClientFinalizer(...)` | Always-run hooks |

### sd/

| Symbol | Description |
|--------|-------------|
| `NewEndpoint(src, factory, logger, opts...)` | Wire SD → Endpointer → RoundRobin → Retry |
| `NewEndpointWithDefaults(src, factory, logger)` | Same with production defaults (3 retries, 500ms, 5s invalidate) |
| `WithMaxRetries(n)` | Max retry attempts (0 = unlimited) |
| `WithTimeout(d)` | Total budget including retries |
| `WithInvalidateOnError(d)` | Clear cache after SD error grace period |
| `instance.Cache` | In-memory Instancer (no Consul needed) |
| `consul.NewInstancer(...)` | Consul-backed Instancer |
| `consul.NewRegistrar(...)` | Consul service registration |

### kit/ (rapid prototyping)

| Symbol | Description |
|--------|-------------|
| `kit.New(addr, opts...)` | Create a ready-to-run HTTP service |
| `kit.JSON[T](handler)` | Package-level typed JSON handler |
| `svc.JSON(pattern, handler)` | Register a JSON route with service middleware |
| `svc.Handle(pattern, handler)` | Register any http.Handler |
| `svc.Run()` | Start + block until SIGINT/SIGTERM |
| `svc.Start()` | Start in background (non-blocking, for tests) |
| `svc.Shutdown(ctx)` | Graceful shutdown |
| `kit.WithRateLimit(rps)` | Token-bucket rate limiter |
| `kit.WithCircuitBreaker(n)` | Gobreaker circuit breaker |
| `kit.WithTimeout(d)` | Per-request deadline |
| `kit.WithMetrics(&m)` | Request counters |
| `kit.WithLogging(logger)` | Structured logging |
| `kit.WithRequestID()` | Inject X-Request-ID |

---

## Production Patterns

### Full middleware stack

```go
var metrics endpoint.Metrics

ep := endpoint.NewBuilder(baseEndpoint).
    WithTracing().                                    // trace/request ID
    WithLogging(logger, "CreateUser").                // structured logs
    WithMetrics(&metrics).                            // counters
    WithErrorHandling("CreateUser").                  // wrap errors
    WithTimeout(5 * time.Second).                     // deadline
    WithBackpressure(200).                            // concurrency limit
    Use(circuitbreaker.Gobreaker(cb)).                // circuit breaker
    Use(ratelimit.NewErroringLimiter(limiter)).        // rate limit
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

### Distributed tracing (no external system required)

```go
// Propagate trace ID from incoming header
before := server.ServerBefore(func(ctx context.Context, r *http.Request) context.Context {
    if id := r.Header.Get("X-Trace-ID"); id != "" {
        ctx = endpoint.WithTraceID(ctx, endpoint.TraceID(id))
    }
    return ctx
})

// Read anywhere in the call chain
traceID := endpoint.TraceIDFromContext(ctx)
reqID   := endpoint.RequestIDFromContext(ctx)
```

### Backpressure (large-scale systems)

```go
// Reject new requests when 500 are already in-flight
var inflight int64
ep = endpoint.InFlightMiddleware(500, &inflight)(ep)

mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
    fmt.Fprintf(w, `{"inflight":%d,"requests":%d}`,
        atomic.LoadInt64(&inflight), metrics.RequestCount)
})
```

### Retry with custom strategy

```go
retryEp := executor.RetryWithCallback(2*time.Second, lb,
    func(n int, err error) (keepTrying bool, replacement error) {
        if errors.Is(err, ErrInvalidArgument) {
            return false, err  // stop on client errors
        }
        return n < 5, nil
    },
)
```

### Consul registration + discovery

```go
// Register on startup
registrar := consul.NewRegistrar(client, logger, "user-service", "10.0.0.1", 8080,
    consul.IDRegistrarOptions("user-service-1"),
    consul.CheckRegistrarOptions(&stdconsul.AgentServiceCheck{
        HTTP: "http://10.0.0.1:8080/health", Interval: "10s",
    }),
)
registrar.Register()
defer registrar.Deregister()

// Discover in client
instancer := consul.NewInstancer(client, logger, "user-service", true)
defer instancer.Stop()
ep := sd.NewEndpointWithDefaults(instancer, grpcFactory, logger)
```

---

## Code Generation (microgen)

`microgen` generates a complete, production-ready microservice from either a Go interface file or an existing database schema.

### Install

```bash
go build -o microgen.exe ./cmd/microgen
# or
make install-microgen
```

### Mode 1 — From IDL file

Define your service interface:

```go
// idl.go
package usersvc

import "context"

type UserModel struct {
    ID       uint   `json:"id"       gorm:"primaryKey;autoIncrement"`
    Username string `json:"username" gorm:"column:username;not null;uniqueIndex"`
    Email    string `json:"email"    gorm:"column:email;not null"`
}

type UserService interface {
    CreateUser(ctx context.Context, req CreateUserRequest) (CreateUserResponse, error)
    GetUser(ctx context.Context, req GetUserRequest)       (GetUserResponse,    error)
    ListUsers(ctx context.Context, req ListUsersRequest)   (ListUsersResponse,  error)
    UpdateUser(ctx context.Context, req UpdateUserRequest) (UpdateUserResponse, error)
    DeleteUser(ctx context.Context, req DeleteUserRequest) (DeleteUserResponse, error)
}
```

Generate:

```bash
# HTTP only
./microgen.exe -idl ./idl.go -out ./gen -import github.com/myorg/usersvc

# HTTP + gRPC + Swagger + config
./microgen.exe -idl ./idl.go -out ./gen -import github.com/myorg/usersvc \
    -protocols http,grpc -model -driver sqlite -swag -config
```

### Mode 2 — From database

```bash
# SQLite
./microgen.exe -from-db -driver sqlite -dsn app.db \
    -out ./gen -import github.com/myorg/app -service MyApp -config

# MySQL
./microgen.exe -from-db -driver mysql \
    -dsn "root:pass@tcp(127.0.0.1:3306)/mydb?charset=utf8mb4&parseTime=True" \
    -dbname mydb -out ./gen -import github.com/myorg/app -service MyApp -config

# PostgreSQL
./microgen.exe -from-db -driver postgres \
    -dsn "host=127.0.0.1 user=postgres password=pass dbname=mydb sslmode=disable" \
    -out ./gen -import github.com/myorg/app -service MyApp

# Add new tables to an existing project (non-destructive)
./microgen.exe -from-db -driver mysql -dsn "..." -dbname mydb \
    -add-tables "orders,products" -out ./gen -import github.com/myorg/app
```

### All flags

| Flag | Default | Description |
|------|---------|-------------|
| `-idl` | — | IDL file path (IDL mode) |
| `-from-db` | `false` | Enable DB mode |
| `-driver` | `mysql` | `sqlite` · `mysql` · `postgres` · `sqlserver` |
| `-dsn` | — | Database DSN |
| `-dbname` | — | Database name (MySQL/SQLServer) |
| `-tables` | — | Comma-separated table filter (empty = all) |
| `-add-tables` | — | Append tables to existing project |
| `-out` | `.` | Output directory |
| `-import` | — | Go module import path |
| `-service` | — | Service name |
| `-protocols` | `http` | `http` · `grpc` · `http,grpc` |
| `-model` | `true` | Generate GORM model + repository |
| `-db` | `true` | Include DB init in main.go |
| `-driver` | `mysql` | DB driver for generated code |
| `-swag` | `false` | Generate Swagger annotations + UI |
| `-tests` | `false` | Generate service_test.go stubs |
| `-config` | `true` | Generate config/config.yaml |
| `-docs` | `true` | Generate README.md |
| `-prefix` | — | HTTP route prefix (e.g. `/api/v1`) |

### Generated project structure

```
gen/
├── cmd/main.go              # Entry point (zap logger, graceful shutdown)
├── service/{svc}/           # Business logic stub — implement here
├── endpoint/{svc}/          # Middleware: circuit breaker, rate limit, logging
│                            # MakeServerEndpointsWithConfig — config-driven
├── transport/{svc}/         # HTTP handlers
│                            # /debug/routes — lists all routes as JSON
├── sdk/{svc}sdk/            # ★ Distributable client SDK
│   └── client.go            #   Copy sdk/ + idl.go to share with third parties
├── pb/{svc}/                # Protobuf definition (-protocols grpc)
├── model/model.go           # GORM models (-model)
├── repository/repository.go # Data access layer (-model)
├── client/{svc}/            # Internal demo client (runnable example)
├── config/
│   ├── config.yaml          # All settings: server, db, middleware, debug
│   └── config.go            # Typed config loader with defaults
├── docs/                    # Swagger stub (-swag)
├── idl.go                   # Service interface + request/response types
└── go.mod
```

### Client SDK — share with third parties

Every generated service includes a ready-to-distribute SDK in `sdk/{svc}sdk/client.go`.
Third parties only need two files:

```
sdk/userservicesdk/client.go   ← copy this
idl.go                         ← copy this (types)
```

Then in their project:

```bash
go get github.com/dreamsxin/go-kit   # only dependency
```

```go
import (
    "github.com/myorg/myapp/sdk/userservicesdk"
    idl "github.com/myorg/myapp"
)

client := userservicesdk.New("http://api.example.com",
    userservicesdk.WithBearerToken(token),
    userservicesdk.WithTimeout(10*time.Second),
)

resp, err := client.CreateUser(ctx, idl.CreateUserRequest{Name: "alice"})
```

The SDK `Client` interface mirrors the server `Service` interface exactly —
the same method signatures, same request/response types.  Mock it in tests:

```go
type mockClient struct{}
func (m *mockClient) CreateUser(_ context.Context, req idl.CreateUserRequest) (idl.CreateUserResponse, error) {
    return idl.CreateUserResponse{ID: 1}, nil
}
```

### Built-in debug endpoints

Every generated service exposes:

```bash
GET /health          # {"status":"ok","service":"UserService"}
GET /debug/routes    # [{"method":"POST","path":"/userservice/createuser",...}]
GET /swagger/        # Swagger UI (with -swag)
```

Control via `config/config.yaml`:

```yaml
debug:
  routes_enabled: true   # set false in production
  print_routes: true     # print route table on startup
```

### Config-driven middleware

The generated `endpoint/{svc}/endpoints.go` wires real middleware from config:

```go
// main.go — automatically generated when -config is set
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

### HTTP route convention

| Method prefix | HTTP verb | Example |
|---------------|-----------|---------|
| `Get*`, `Find*`, `List*`, `Query*`, `Search*` | GET | `GET /userservice/listusers` |
| `Delete*`, `Remove*` | DELETE | `DELETE /userservice/deleteuser` |
| `Update*`, `Edit*`, `Patch*` | PUT | `PUT /userservice/updateuser` |
| others | POST | `POST /userservice/createuser` |

DB mode generates RESTful routes:

| Operation | Route |
|-----------|-------|
| Create | `POST /{svc}/{resource}` |
| Get by ID | `GET /{svc}/{resource}/{id}` |
| Update | `PUT /{svc}/{resource}/{id}` |
| Delete | `DELETE /{svc}/{resource}/{id}` |
| List | `GET /{svc}/{resource}s` |

---

## Examples

| Directory | What it shows | Run |
|-----------|--------------|-----|
| `examples/quickstart/` | Minimal service (30 lines) | `go run ./examples/quickstart` |
| `examples/best_practice/` | Production patterns: metrics, CB, rate limit | `go run ./examples/best_practice` |
| `examples/middleware/` | Every middleware: Chain, Builder, Failer, Gobreaker, … | `go run ./examples/middleware` |
| `examples/httpclient/` | HTTP client: NewJSONClient, hooks | `go run ./examples/httpclient` |
| `examples/sd/` | Service discovery: Cache, RoundRobin, Retry, … | `go run ./examples/sd` |
| `examples/profilesvc/` | Full CRUD service + Consul client | `go run ./examples/profilesvc/cmd/profilesvc` |
| `examples/transport/` | HTTP server/client + gRPC deep-dive | `go test ./examples/transport/...` |

---

## Testing

### Run framework tests

```bash
make test                          # all tests with race detection
go test ./...                      # all packages
go test -cover ./endpoint/...      # with coverage
```

### Run example tests

```bash
python tools/test_examples.py              # compile + go test + HTTP smoke
python tools/test_examples.py --no-runtime # compile + go test only (CI)
python tools/test_examples.py -k quickstart
```

### Run microgen integration tests

```bash
python tools/test_microgen.py              # 25 cases: IDL + DB + runtime
python tools/test_microgen.py --no-runtime # skip HTTP smoke tests
python tools/test_microgen.py -k db        # DB mode only
python tools/test_microgen.py -k runtime -v
```

### Run framework API validation

```bash
python tools/test_framework.py             # 14 cases: symbols + go test + HTTP
python tools/test_framework.py -k typed    # TypedEndpoint only
```

### Coverage summary

| Package | Coverage |
|---------|---------|
| `endpoint` | 88.5% |
| `endpoint/circuitbreaker` | 100% |
| `endpoint/ratelimit` | 100% |
| `transport/http/server` | 54.9% |
| `sd` | 100% |
| `sd/instance` | 97.6% |
| `utils` | 100% |

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
├── log/                     # zap wrapper (Logger = *zap.Logger)
├── utils/                   # Exponential backoff
├── examples/                # Runnable examples (see examples/README.md)
├── tools/                   # Test tools (see tools/README.md)
└── cmd/microgen/            # Code generator
```

---

## Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| `go.uber.org/zap` | v1.27 | Structured logging |
| `google.golang.org/grpc` | v1.80 | gRPC transport |
| `github.com/gorilla/mux` | v1.8 | HTTP routing |
| `github.com/sony/gobreaker` | v1.0 | Circuit breaker |
| `github.com/afex/hystrix-go` | latest | Circuit breaker (Hystrix) |
| `github.com/streadway/handy` | latest | Circuit breaker (HandyBreaker) |
| `golang.org/x/time` | v0.15 | Token bucket rate limiting |
| `github.com/hashicorp/consul/api` | v1.33 | Service discovery |
| `gorm.io/gorm` | v1.25 | ORM (generated projects) |

---

## Contributing

1. Fork the project
2. Create a feature branch: `git checkout -b feat/my-feature`
3. Write tests for new functionality
4. Run `make test` and `make lint`
5. Open a Pull Request

```bash
make tools   # install golangci-lint, swag, protoc plugins
make test    # run all tests with race detection
make lint    # static analysis
make build   # build all packages
make coverage  # generate coverage.html
```

---

## License

[MIT](LICENSE.txt)
