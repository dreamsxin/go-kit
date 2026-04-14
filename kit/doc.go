// Package kit provides a high-level, zero-boilerplate API for rapid
// prototyping and production microservices.
//
// Quickstart:
//
//	func main() {
//	    svc := kit.New(":8080")
//	    svc.Handle("/hello", kit.JSON[HelloReq](func(ctx context.Context, req HelloReq) (any, error) {
//	        return HelloResp{Message: "Hello, " + req.Name}, nil
//	    }))
//	    svc.Run()
//	}
//
// With middleware:
//
//	svc := kit.New(":8080",
//	    kit.WithRateLimit(100),
//	    kit.WithCircuitBreaker(5),
//	    kit.WithTimeout(5*time.Second),
//	    kit.WithRequestID(),
//	    kit.WithLogging(logger),
//	    kit.WithMetrics(&metrics),
//	)
package kit
