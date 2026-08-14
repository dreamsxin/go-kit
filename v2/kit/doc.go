// Package kit provides a high-level, zero-boilerplate API for rapid
// prototyping and small production services.
//
// Kit is a thin scaffold over the framework's normal service -> endpoint ->
// transport shape. Prefer HandleJSONTyped for concrete response types,
// HandleJSON for dynamic responses, and
// HandleJSONEndpoint when you already have an endpoint.Endpoint. Use
// Service.Handle and Service.HandleFunc only for raw HTTP integrations such as
// static files, third-party handlers, probes, or custom protocol endpoints.
//
// Quickstart:
//
//	func run(ctx context.Context) error {
//	    svc, err := kit.New(":8080")
//	    if err != nil {
//	        return err
//	    }
//	    kit.HandleJSONTyped(svc, "/hello", func(ctx context.Context, req HelloReq) (HelloResp, error) {
//	        return HelloResp{Message: "Hello, " + req.Name}, nil
//	    })
//	    return svc.Run(ctx)
//	}
//
// With middleware:
//
//	svc, err := kit.New(":8080",
//	    kit.WithEndpointMiddleware(
//	        ratelimit.NewErroringLimiter(limiter),
//	        circuitbreaker.Gobreaker(breaker),
//	        zapadapter.LoggingMiddleware(logger, "request"),
//	    ),
//	    kit.WithTimeout(5*time.Second),
//	    kit.WithRequestID(),
//	    kit.WithMetrics(&metrics),
//	    kit.WithReadinessCheck("database", checkDatabase),
//	)
package kit
