# Examples
English | [简体中文](README_zh.md)

A guided tour of the go-kit framework, from simplest to most complete.

`examples/` is an independent module joined to the repository workspace. Run
every command below from the `v2/` module directory, or from `v2/examples/`
after dropping the `./examples/` prefix.

## Learning Path

| Directory | What it shows | Run |
|-----------|--------------|-----|
| `quickstart/` | Recommended kit API: `kit.New` + `kit.HandleJSONTyped` + `svc.Run` | `go run ./examples/quickstart` |
| `basic/` | Middleware chain execution order | `go test ./examples/basic/...` |
| `manual_composition/` | Explicit endpoint Builder + HTTP transport composition | `go run ./examples/manual_composition` |
| `best_practice/` | Production patterns: metrics, circuit breaker, rate limit, graceful shutdown | `go run ./examples/best_practice` |
| `middleware/` | Endpoint middleware: Chain, Builder, Failer, Timeout, Gobreaker, ErroringLimiter, DelayingLimiter | `go run ./examples/middleware` |
| `httpclient/` | HTTP client: NewJSONClient, ClientBefore/After/Finalizer, SetClient | `go run ./examples/httpclient` |
| `auth/` | Application-owned authentication and authorization middleware: Bearer keys, 401/403 via apperror, public health routes | `go run ./examples/auth` |
| `todosvc/` | Database CRUD service: SQLite repository, Service -> Endpoint -> HTTP, graceful shutdown | `go run ./examples/todosvc` |
| `interaction_policy/` | AI interaction runtime: MCP-style tool calls with authorization and audit hooks | `go run ./examples/interaction_policy` |
| `mcp_basic/` | Minimal MCP server: single tool, `NewRuntime()`, `mcp.ListenAndServe` | `go run ./examples/mcp_basic` |
| `mcp_full/` | Full MCP server: tools, resources, prompts, notifications, completions, SSE streaming | `go run ./examples/mcp_full` |
| `sd/` | Service discovery: instance.Cache, Endpointer, RoundRobin, Retry, sd/client.NewEndpoint, InvalidateOnError | `go run ./examples/sd` |
| `multisvc/` | IDL definition for two services in one package | (library) |
| `profilesvc/` | Full CRUD service: Service → Endpoint → HTTP transport + Consul client | `go run ./examples/profilesvc/cmd/profilesvc` |
| `transport/` | Deep-dive tests for HTTP server, HTTP client, and gRPC | `go test ./examples/transport/...` |
| `usersvc/` | IDL with GORM model — input for `microgen` code generation | (library) |

## Quick Start

```bash
# Recommended kit service
go run ./examples/quickstart
curl -X POST http://localhost:8080/greet \
     -H "Content-Type: application/json" \
     -d '{"name":"world"}'

# Lower-level component composition
go run ./examples/manual_composition
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

### 3. Type-safe HTTP handler

```go
// Automatic JSON decode/encode with concrete request and response types.
typed := endpoint.Unwrap[helloRequest, helloResponse](ep)
mux.Handle("/hello", server.NewTypedJSONServer(typed))
```

### 4. Service discovery in one line

```go
// Consul → Endpointer → RoundRobin → Retry, all wired automatically
ep, closer, err := sdclient.NewEndpoint(instancer, factory, logger,
    sdclient.WithMaxAttempts(3),
    sdclient.WithTimeout(500*time.Millisecond),
)
if err != nil { return err }
defer closer.Close()
```

## Run All Example Tests

```bash
go test ./examples/...                # compile + unit tests
go test ./tools/... -run TestAll      # integration smoke tests
make verify                           # full validation (runtime + microgen + integration)
```
