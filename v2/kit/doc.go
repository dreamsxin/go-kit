// Package kit provides a high-level, zero-boilerplate API for rapid
// prototyping and small production services.
//
// Kit is a thin scaffold over the framework's normal service -> endpoint ->
// transport shape. The transport-neutral Host orchestrates lifecycle
// components; the HTTP component carries routes, health checks, and strict
// JSON transport behavior. Prefer HandleJSONTyped for concrete response
// types, HandleJSON for dynamic responses, and HandleJSONEndpoint when you
// already have an endpoint.Endpoint. Use HandleSSETyped for Server-Sent
// Events streams protected by endpoint middleware, or HandleSSE for raw
// streaming handlers. Use Handle and HandleFunc only for raw HTTP
// integrations such as static files, third-party handlers, probes, or custom
// protocol endpoints.
//
// Quickstart:
//
//	func run(ctx context.Context) error {
//	    http, err := kit.NewHTTP(":8080")
//	    if err != nil {
//	        return err
//	    }
//	    kit.HandleJSONTyped(http, "/hello", func(ctx context.Context, req HelloReq) (HelloResp, error) {
//	        return HelloResp{Message: "Hello, " + req.Name}, nil
//	    })
//	    host, err := kit.NewHost(kit.WithLifecycle(http))
//	    if err != nil {
//	        return err
//	    }
//	    return host.Run(ctx)
//	}
//
// With middleware:
//
//	http, err := kit.NewHTTP(":8080",
//	    kit.WithEndpointMiddleware(
//	        endpoint.RateLimitMiddleware(limiter),
//	        endpoint.NewCircuitBreaker().Middleware(),
//	        slogadapter.LoggingMiddleware(logger, "request"),
//	    ),
//	    kit.WithTimeout(5*time.Second),
//	    kit.WithRequestID(),
//	    kit.WithMetrics(&metrics),
//	    kit.WithReadinessCheck("database", checkDatabase),
//	)
package kit
