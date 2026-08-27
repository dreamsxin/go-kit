# 示例

[English](README.md) | 简体中文

一份 go-kit 框架的导览，从最简单到最完整。

`examples/` 是一个加入仓库 workspace 的独立模块。请从 `v2/` 模块目录运行下面的每条命令；如果位于 `v2/examples/` 目录下，则去掉 `./examples/` 前缀后再运行。

## 学习路径

| 目录 | 展示内容 | 运行 |
|-----------|--------------|-----|
| `quickstart/` | 推荐的 kit API：`kit.New` + `kit.HandleJSONTyped` + `svc.Run` | `go run ./examples/quickstart` |
| `basic/` | 中间件链的执行顺序 | `go test ./examples/basic/...` |
| `manual_composition/` | 显式的 endpoint Builder + HTTP 传输层组合 | `go run ./examples/manual_composition` |
| `best_practice/` | 生产级模式：指标、熔断器、限流、优雅关闭 | `go run ./examples/best_practice` |
| `middleware/` | endpoint 中间件：Chain、Builder、Failer、Timeout、CircuitBreaker、RateLimit、DelayRateLimit | `go run ./examples/middleware` |
| `httpclient/` | HTTP 客户端：NewJSONClient、ClientBefore/After/Finalizer、SetClient | `go run ./examples/httpclient` |
| `auth/` | 应用自有的认证与授权中间件：Bearer 密钥、通过 apperror 返回 401/403、公开的健康检查路由 | `go run ./examples/auth` |
| `envelope/` | 传输层响应组装：通过 `kit.WithJSONServerOptions` 一次定义信封与错误格式 | `go run ./examples/envelope` |
| `customcodec/` | HTTP 上的自定义 body 格式：RawBodyCodec、同格式错误编码器 | `go run ./examples/customcodec` |
| `todosvc/` | 数据库 CRUD 服务：SQLite 仓储、Service -> Endpoint -> HTTP、优雅关闭 | `go run ./examples/todosvc` |
| `interaction_policy/` | AI 交互运行时：带授权与审计钩子的 MCP 风格工具调用 | `go run ./examples/interaction_policy` |
| `mcp_basic/` | 最小 MCP 服务器：单个工具、`NewRuntime()`、`mcp.ListenAndServe` | `go run ./examples/mcp_basic` |
| `mcp_full/` | 完整 MCP 服务器：工具、资源、提示词、通知、补全、SSE 流式传输 | `go run ./examples/mcp_full` |
| `sd/` | 服务发现：instance.Cache、Endpointer、RoundRobin、Retry、sd/client.NewEndpoint、InvalidateOnError | `go run ./examples/sd` |
| `multisvc/` | 在一个包中为两个服务定义 IDL | （库） |
| `profilesvc/` | 完整的 CRUD 服务：Service → Endpoint → HTTP 传输层 + Consul 客户端 | `go run ./examples/profilesvc/cmd/profilesvc` |
| `transport/` | 针对 HTTP 服务器、HTTP 客户端和 gRPC 的深入测试 | `go test ./examples/transport/...` |
| `usersvc/` | 带 GORM 模型的 IDL — `microgen` 代码生成的输入 | （库） |

## 快速开始

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

## 关键模式

### 1. 业务逻辑保持纯净

```go
// No framework imports — easy to test
func helloLogic(_ context.Context, req helloRequest) (helloResponse, error) {
    if req.Name == "" {
        return helloResponse{}, errors.New("name is required")
    }
    return helloResponse{Message: "Hello, " + req.Name + "!"}, nil
}
```

### 2. 链式中间件装配

```go
var metrics endpoint.Metrics
ep := endpoint.NewBuilder(base).
    WithMetrics(&metrics).
    WithErrorHandling("hello").
    Use(endpoint.TimeoutMiddleware(5 * time.Second)).
    Use(endpoint.NewCircuitBreaker().Middleware()).
    Use(endpoint.RateLimitMiddleware(limiter)).
    Build()
```

### 3. 类型安全的 HTTP handler

```go
// Automatic JSON decode/encode with concrete request and response types.
typed := endpoint.Unwrap[helloRequest, helloResponse](ep)
mux.Handle("/hello", server.NewTypedJSONServer(typed))
```

### 4. 一行完成服务发现

```go
// Consul → Endpointer → RoundRobin → Retry, all wired automatically
ep, closer, err := sdclient.NewEndpoint(instancer, factory, logger,
    sdclient.WithMaxAttempts(3),
    sdclient.WithTimeout(500*time.Millisecond),
)
if err != nil { return err }
defer closer.Close()
```

## 运行全部示例测试

```bash
go test ./examples/...                # compile + unit tests
go test ./tools/... -run TestAll      # integration smoke tests
make verify                           # full validation (runtime + microgen + integration)
```
