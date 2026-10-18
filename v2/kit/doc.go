// Package kit provides a high-level, zero-boilerplate API for rapid
// prototyping and small production services.
//
// Kit is a thin scaffold over the framework's normal service -> endpoint ->
// transport shape. The transport-neutral Host orchestrates lifecycle
// components; the HTTP component carries routes, health checks, and strict
// JSON transport behavior.
//
// # Choosing a registration
//
// Every registration below mounts on the same *http.ServeMux and every one of
// them gets the HTTP context: in-flight accounting, the request and writer in
// the context, Stopping, traceparent extraction, request-ID handling, the
// WithTimeout deadline, and any WithHTTPRecorder recorders. They differ on two
// axes only — whether the endpoint middleware chain and the endpoint recorders
// run, and which request-body limit applies.
//
// Full chain (WithEndpointMiddleware — where the endpoint package's rate
// limiting, circuit breaking, and bulkhead middleware are added —
// WithMetrics, WithRecorder, and the component's JSON server options all
// apply):
//
//   - HandleJSON — the default. Request and response types are inferred from the
//     handler, so a concrete response needs no type arguments. A typed route
//     fills its request struct from the request itself: GET and HEAD read the
//     query string and the path values, every other method reads a strictly
//     decoded JSON body and then the path values, and a request that carries no
//     body decodes to the zero value so the handler's validation answers in the
//     application's own words. A validate tag on the request struct is enforced
//     after the fill — the same package the generated transports call, so both
//     shapes of service reject the same request.
//   - HandleJSONEndpoint — when you already hold an endpoint.Endpoint.
//   - RouteBodyLimit, RouteMiddleware, RouteServerOptions — one route's
//     differences, expressed as options on those two registrations rather than
//     as another function name. A per-route body limit keeps the endpoint
//     middleware, the recorders, and the component's JSON server options, which
//     is what makes it different from reaching for Handle.
//   - HandleSSETyped — a Server-Sent Events stream. The chain observes the whole
//     stream as one request, so a timeout middleware bounds its total duration,
//     the component's JSON server options apply to it, and an error that ends the
//     stream reaches the chain rather than being reported as a completed request.
//     What cannot follow is the response: once the stream has answered 200 the
//     error can only be recorded, not rendered.
//
// Escape hatch — no endpoint middleware, no endpoint recorders:
//
//   - Handle, HandleFunc — a raw http.Handler, for static files, third-party
//     handlers, or a protocol this package does not model. Reach for Handle and
//     know what you are giving up.
//
// Neither of the above — this registers nothing and returns a handler you mount
// yourself, so no component is involved at all:
//
//   - NewJSONHandler. The returned handler carries no endpoint middleware, no
//     recorders, and no component JSON server options.
//
// Probes are mounted directly and are outside the HTTP context by design, so
// recorders never see /health, /livez or /readyz.
//
// A Group prefixes every pattern registered through it, so a versioned
// surface states its version once:
//
//	api := http.Group("/api/v1")
//	kit.HandleJSON(api, "POST /todos", createTodo)
//
// Quickstart:
//
//	func run(ctx context.Context) error {
//	    http, err := kit.NewHTTP(":8080")
//	    if err != nil {
//	        return err
//	    }
//	    kit.HandleJSON(http, "/hello", func(ctx context.Context, req HelloReq) (HelloResp, error) {
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
//
// Serving TLS is an option too: WithTLS loads a certificate and key at
// construction, so a bad path stops startup rather than failing a handshake, and
// WithTLSConfig takes a tls.Config the deployment built. Turning TLS on turns
// HTTP/2 on with it, through ALPN.
//
// Stopping is a sequence. Host.Run announces that the process is going away —
// readiness starts failing and every Draining component is told — waits
// WithDrainDelay so the routing layer can notice, then shuts down inside a budget
// and closes whatever the grace period left open. A long-lived handler watches
// Stopping(ctx) and ends its own response; a hijacked connection is outside all of
// it, and ending one is its handler's job.
package kit
