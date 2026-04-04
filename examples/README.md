# Examples

A guided tour of the go-kit framework, from simplest to most complete.

## Learning Path

| Directory | What it shows | Run |
|-----------|--------------|-----|
| `basic/` | Middleware chain execution order | `go test ./examples/basic/...` |
| `quickstart/` | Minimal HTTP service with Builder + NewJSONServer | `go run ./examples/quickstart` |
| `best_practice/` | Production patterns: metrics, circuit breaker, rate limit, graceful shutdown | `go run ./examples/best_practice` |
| `middleware/` | Every endpoint middleware: Chain, Builder, Failer, Timeout, Gobreaker, HandyBreaker, ErroringLimiter, DelayingLimiter | `go run ./examples/middleware` |
| `httpclient/` | HTTP client: NewJSONClient, ClientBefore/After/Finalizer, SetClient | `go run ./examples/httpclient` |
| `sd/` | Service discovery: instance.Cache, Endpointer, RoundRobin, Retry, RetryWithCallback, sd.NewEndpoint, InvalidateOnError | `go run ./examples/sd` |
| `common/` | Shared helpers: Greeter interface, Headerer response | (library) |
| `multisvc/` | IDL definition for two services in one package | (library) |
| `profilesvc/` | Full CRUD service: Service → Endpoint → HTTP transport + Consul client | `go run ./examples/profilesvc/cmd/profilesvc` |
| `transport/` | Deep-dive tests for HTTP server, HTTP client, and gRPC | `go test ./examples/transport/...` |
| `usersvc/` | IDL with GORM model — input for `microgen` code generation | (library) |

## Quick Start

```bash
# Minimal HTTP service
go run ./examples/quickstart
curl -X POST http://localhost:8080/hello \
     -H "Content-Type: application/json" \
     -d '{"name":"world"}'

# Best-practice service (metrics + circuit breaker + rate limit)
go run ./examples/best_practice
curl -X POST http://localhost:8080/hello -H "Content-Type: application/json" -d '{"name":"Alice"}'
curl http://localhost:8080/metrics

# Full profile service
go run ./examples/profilesvc/cmd/profilesvc
curl -X POST http://localhost:8080/profiles/ \
     -H "Content-Type: application/json" \
     -d '{"id":"1","name":"Alice"}'
curl http://localhost:8080/profiles/1
```

## Key Patterns

### 1. Business logic stays pure

```go
// No framework imports — easy to test
func helloLogic(_ context.Context, req helloRequest) (helloResponse, error) {
    if req.Name == "" {
        return helloResponse{}, errors.New("name is required")
    }
    return helloResponse{Message: "Hello, " + req.Name + "!"}, nil
}
```

### 2. Fluent middleware assembly

```go
var metrics endpoint.Metrics
ep := endpoint.NewBuilder(base).
    WithMetrics(&metrics).
    WithErrorHandling("hello").
    Use(endpoint.TimeoutMiddleware(5 * time.Second)).
    Use(circuitbreaker.Gobreaker(cb)).
    Use(ratelimit.NewErroringLimiter(limiter)).
    Build()
```

### 3. Zero-boilerplate HTTP handler

```go
// Automatic JSON decode/encode — no DecodeRequestFunc needed
mux.Handle("/hello", server.NewJSONServer[helloRequest](
    func(ctx context.Context, req helloRequest) (any, error) {
        return ep(ctx, req)
    },
))
```

### 4. Service discovery in one line

```go
// Consul → Endpointer → RoundRobin → Retry, all wired automatically
ep := sd.NewEndpoint(instancer, factory, logger,
    sd.WithMaxRetries(3),
    sd.WithTimeout(500*time.Millisecond),
)
```

## Run All Example Tests

```bash
python tools/test_examples.py          # compile + go test + HTTP smoke tests
python tools/test_examples.py --no-runtime  # compile + go test only (CI)
python tools/test_examples.py -k transport  # filter by name
```
