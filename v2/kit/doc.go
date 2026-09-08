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
// Full chain (WithEndpointMiddleware, WithMetrics, WithRecorder, WithRateLimit,
// WithCircuitBreaker and the component's JSON server options all apply):
//
//   - HandleJSONTyped — the default. Concrete request and response types.
//   - HandleJSON — the same, when the response type is dynamic.
//   - HandleJSONEndpoint — when you already hold an endpoint.Endpoint.
//   - HandleJSONTypedWithMiddleware, HandleJSONWithMiddleware — add route-local
//     middleware, which ends up inside the component's chain.
//   - HandleJSONTypedWithBodyLimit, HandleJSONEndpointWithBodyLimit — one route
//     accepts a different maximum body. A limit of zero or less panics.
//   - HandleSSETyped — a Server-Sent Events stream. The chain observes the whole
//     stream as one request, so a timeout middleware bounds its total duration.
//     Two caveats specific to it: a middleware rejection on the route is rendered
//     with the built-in JSON error encoder rather than a component encoder, and
//     the endpoint chain sees the stream as a success even when the stream itself
//     fails, because the bridge reports the handler's completion rather than its
//     outcome.
//
// Escape hatch — no endpoint middleware, no endpoint recorders:
//
//   - Handle, HandleFunc — a raw http.Handler, for static files, third-party
//     handlers, or a protocol this package does not model. HandleSSE is a
//     deprecated alias for Handle: a name promising SSE behaviour while silently
//     skipping the chain is how a stream loses its middleware by accident. Reach
//     for Handle and know what you are giving up.
//
// Neither of the above — these register nothing and return a handler you mount
// yourself, so no component is involved at all:
//
//   - NewJSONHandler, NewJSONTypedHandler. JSON and JSONTyped are the deprecated
//     spellings; they differed from HandleJSON and HandleJSONTyped by one verb.
//
// Probes are mounted directly and are outside the HTTP context by design, so
// recorders never see /health, /livez or /readyz.
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
