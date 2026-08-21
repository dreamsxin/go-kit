# go-kit v2 Manual

English | [简体中文](MANUAL_zh.md)

This manual is the complete user guide for go-kit v2, organized as Quick Start
-> Core Concepts -> Components -> Production. Each chapter stands alone, but
reading them in order builds the whole picture fastest.

## Part 1: Quick Start

**Goal: a running service in about fifteen minutes.**

1. **Install and generate** -- install microgen and generate a runnable HTTP
   service from an IDL
   -> [README: Generate A Service](README.md)
2. **Understand what was generated** -- which files you own and which the
   generator owns
   -> [MICROGEN: Generated Project Manifest](MICROGEN.md)
3. **Walk the request path** -- experience Service -> Endpoint -> Transport on
   runnable examples
   -> [examples tour](examples/README.md)
4. **Assemble by hand** -- build a small service from zero with `kit`
   -> [README: Build With kit](README.md)

## Part 2: Core Concepts

Understand the three invariants before writing the first line.

### 2.1 The Request Path

```text
Transport request
    -> decode
    -> endpoint middleware
    -> endpoint
    -> service method
    -> encode
    -> transport response
```

Each layer owns one kind of decision:

| Layer | Owns | Must not own |
| --- | --- | --- |
| Service | Business rules and domain orchestration | HTTP/gRPC types and status mapping |
| Endpoint | Transport-neutral request boundary and middleware | Socket/server lifecycle |
| Transport | Protocol decode, encode, headers, and status | Business rules and retry policy |
| Assembly | Dependency wiring and process lifecycle | Hidden global state |

See [ARCHITECTURE](ARCHITECTURE.md).

### 2.2 Error Handling

Business errors are classified in `apperror`; the transport layer maps them to
protocol status codes automatically:

```go
return Todo{}, apperror.New(apperror.KindNotFound, "todo.not_found", "todo not found")
```

- `KindInvalidArgument` -> 400
- `KindUnauthenticated` -> 401
- `KindPermissionDenied` -> 403
- `KindNotFound` -> 404
- `KindAlreadyExists`/`KindConflict` -> 409
- `KindResourceExhausted` -> 429
- unclassified -> 500 (opaque to clients)

### 2.3 Lifecycle

The process entry point owns signals and the root context; the service owns
listeners and graceful shutdown after startup succeeds:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
if err := svc.Run(ctx); err != nil {
    return err
}
```

Optional components (background jobs, gRPC servers) attach through
`kit.Lifecycle` and share the same bounded shutdown.

## Part 3: Components (By Request Path)

The component tour follows the order a request flows through them. Each
section gives a canonical usage example and a documentation link.

### 3.1 endpoint: Endpoints And Middleware

An endpoint is the transport-neutral request function; every cross-cutting
concern composes through middleware:

```go
ep := endpoint.NewBuilder(createUser).
    WithValidation().
    WithTimeout(5*time.Second).
    Use(endpoint.NewCircuitBreaker(...).Middleware()).
    Use(endpoint.RateLimitMiddleware(limiter)).
    WithMetrics(&metrics).
    Build()
```

- [endpoint guide](endpoint/README.md): Builder, Chain, TypedEndpoint, the
  four middleware flow-control patterns (short-circuit, branch, repeat,
  replace)
- Runnable example: [examples/middleware](examples/README.md)

### 3.2 transport: Protocol Adaptation

transport adapts endpoints to HTTP/gRPC: decoding/encoding, headers, status,
pagination, file upload, SSE, and trace-context propagation:

```go
// Server
server.NewTypedJSONServer(ep, server.ServerResponseEncoder(encodeAPIResponse))

// Client
client.NewJSONClient(http.MethodPost, url, enc, dec,
    client.Before(transporthttp.InjectTraceparent),
)
```

- [transport guide](transport/README.md): composition and nesting semantics,
  the pagination convention, combining multiple request parsers
- Runnable examples: [examples/envelope](examples/README.md),
  [examples/httpclient](examples/README.md)

### 3.3 kit: Service Assembly

kit assembles endpoints and the HTTP transport into a runnable service with
health checks, lifecycle, and graceful shutdown:

```go
svc, err := kit.New(":8080",
    kit.WithRequestID(),
    kit.WithTimeout(5*time.Second),
    kit.WithJSONServerOptions(
        server.ServerResponseEncoder(encodeAPIResponse),
        server.ServerErrorEncoder(encodeAPIError),
    ),
)
kit.HandleJSONTyped(svc, "/hello", hello)
```

- [Build With kit](README.md)
- Runnable examples: [examples/quickstart](examples/README.md),
  [examples/todosvc](examples/README.md), [examples/auth](examples/README.md)

### 3.4 sd: Service Discovery And Load Balancing

Service discovery resolves an instance set into callable endpoints with
balancing and controlled retry:

```go
call, closer, err := sdclient.NewEndpoint(instancer, factory, logger,
    sdclient.WithMaxAttempts(3),
    sdclient.WithTimeout(500*time.Millisecond),
)
```

- [sd guide](sd/README.md): Instancer contract, endpointer, round-robin,
  retry
- Runnable example: [examples/sd](examples/README.md)

### 3.5 interaction: AI Interaction And MCP

The interaction runtime exposes tools, resources, prompts, and policy hooks
over MCP Streamable HTTP:

```go
rt := interaction.NewRuntime().WithHooks(
    interaction.AuthorizationHook{Authorizer: allowTools("echo")},
)
mux.Handle("/mcp", interactionmcp.NewHandler(rt))
```

- [interaction guide](interaction/README.md)
- Runnable examples: [examples/mcp_basic](examples/README.md),
  [examples/interaction_policy](examples/README.md)

### 3.6 security: HTTP Security Middleware

security/http provides trusted-proxy resolution, CORS, CSRF, security
headers, and IP policy:

```go
handler := httpsecurity.Chain(
    httpsecurity.TrustedProxy(proxy),
    httpsecurity.CORS(corsPolicy),
    httpsecurity.CSRF(csrfConfig),
)(rootHandler)
```

- [security/http guide](security/http/README.md)
- Runnable example: [examples/auth](examples/README.md)

### 3.7 observability: Observability

The standard-library slog adapter and the OpenTelemetry adapter cover logging
and tracing/metrics respectively:

```go
ep = slogadapter.LoggingMiddleware(logger, "CreateUser")(ep)
ep = oteladapter.TracingMiddleware(tracer, "GetUser")(ep)
```

- [slog adapter](observability/slog/README.md),
  [otel adapter](observability/otel/README.md)
- Production guidance: [PRODUCTION: Logging/Metrics/Tracing](PRODUCTION.md)

### 3.8 Optional Integrations: Consul And gRPC

Consul service discovery and the gRPC transport are independent modules,
installed on demand:

```bash
go get github.com/dreamsxin/go-kit/v2/integrations/consul@v0.2.3
go get github.com/dreamsxin/go-kit/v2/integrations/grpc@v0.2.3
```

- [Consul integration](integrations/consul/README.md)
- [gRPC integration](integrations/grpc/README.md)

### 3.9 microgen: Code Generator

microgen generates complete projects from a Go IDL, Protobuf, or a database
schema:

```bash
microgen -idl idl.go -out ./service -import example.com/service
microgen -from-db -driver sqlite -dsn ./app.db -tables todos -out ./svc
```

- [microgen guide](MICROGEN.md): source modes, options, generated ownership,
  extend mode
- Runnable example: [examples/usersvc](examples/README.md) (IDL input)

## Part 4: Production

- [Production guide](PRODUCTION.md): lifecycle, HTTP configuration, service
  discovery, configuration, authentication and authorization, browser-facing
  security, logging, metrics, tracing, health, background jobs, deployment,
  alerting
- [Upgrade notes](MIGRATION.md): upgrade actions between releases

## Appendix

- [API reference](https://pkg.go.dev/github.com/dreamsxin/go-kit/v2)
- [Changelog](CHANGELOG.md)
- [Documentation index](DOCS_INDEX.md)
