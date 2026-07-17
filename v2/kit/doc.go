// Package kit provides a high-level, zero-boilerplate API for rapid
// prototyping and small production services.
//
// Kit is a thin scaffold over the framework's normal service -> endpoint ->
// transport shape. Prefer HandleJSON for concise handlers and
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
//	    kit.HandleJSON[HelloReq](svc, "/hello", func(ctx context.Context, req HelloReq) (any, error) {
//	        return HelloResp{Message: "Hello, " + req.Name}, nil
//	    })
//	    return svc.Run(ctx)
//	}
//
// With middleware:
//
//	svc, err := kit.New(":8080",
//	    kit.WithRateLimit(100),
//	    kit.WithCircuitBreaker(5),
//	    kit.WithTimeout(5*time.Second),
//	    kit.WithRequestID(),
//	    kit.WithLogging(logger),
//	    kit.WithMetrics(&metrics),
//	    kit.WithReadinessCheck("database", checkDatabase),
//	)
package kit
