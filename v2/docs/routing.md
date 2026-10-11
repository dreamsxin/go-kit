# Routing

English | [简体中文](routing_zh.md)

How to define the routes of a service: the pattern syntax, the registration
call to reach for, path and query parameters, the options that shape every
route, and how gRPC and generated projects answer the same question.

All HTTP routes live on one `*http.ServeMux` per `kit.HTTP` component, and the
pattern syntax is Go 1.22's ServeMux syntax — this chapter explains the
conventions the framework adds on top, not a different router.

## Pattern syntax

A route pattern is a Go ServeMux pattern. The framework convention is
`"METHOD /path"`:

```go
kit.HandleJSONTyped(svc, "POST /todos", createTodo)
kit.HandleJSONTyped(svc, "GET /todos/{id}", getTodo)
```

- The method prefix is optional. `"POST /todos"` answers POST and answers
  every other method with `405` and an `Allow` header — the mux does that for
  you, there is nothing to write. A pattern without a method
  (`"/api/me"`) answers every method.
- `{id}` declares a path parameter. The handler reads it with
  `r.PathValue("id")`. A `{name...}` wildcard and a trailing slash
  (`"/swagger/"`) match subtrees, per the standard ServeMux rules.
- When two patterns both match, **the most specific one wins** — `POST
  /todos/done` beats `POST /todos/{id}` for that path, whatever registered
  first.
- Registering two identical patterns panics at startup. That is deliberate: a
  route collision is a programming error you want to meet before the first
  request, not after.

The probe routes `kit.NewHTTP` registers — `/health`, `/livez`, `/readyz` —
are the one exception to the method-prefix convention: they are mounted
without a method, so they answer every method. Generated projects register
them as `"GET /health"`, which the mux serves to GET and HEAD.

## Choosing a registration

Every registration mounts on the same mux and gets the same HTTP context:
in-flight accounting, the request and writer in the context, `kit.Stopping`,
traceparent extraction, request-ID handling, the `WithTimeout` deadline, and
any `WithHTTPRecorder` recorders. The calls differ on two axes only — whether
the endpoint middleware chain and endpoint recorders run, and which
request-body limit applies.

| Call | Use it when |
| --- | --- |
| `kit.HandleJSONTyped` | The default. Typed request and response, full chain; the request struct fills from the request itself (see below). |
| `kit.HandleJSON` | Same, when the response type is dynamic. |
| `kit.HandleJSONTypedWithMiddleware` / `HandleJSONWithMiddleware` | One route needs extra middleware; it ends up *inside* the component's chain. |
| `kit.HandleJSONEndpoint` | You already hold an `endpoint.Endpoint`. |
| `kit.HandleJSONTypedWithBodyLimit` / `HandleJSONEndpointWithBodyLimit` | One route accepts a different maximum body; zero or less panics. Keeps the full chain — unlike reaching for `Handle`. |
| `kit.HandleSSETyped` | A Server-Sent Events stream. See [SSE routes](#sse-routes). |
| `svc.HandleFunc` / `svc.Handle` | The escape hatch: static files, third-party handlers, protocols kit does not model. |

The escape hatch deserves its trade spelled out. A raw handler keeps the HTTP
context — request ID, deadline, recorders — but **no endpoint middleware and
no endpoint recorders**: `WithEndpointMiddleware`, `WithMetrics`,
`WithRecorder`, rate limiting, and circuit breaking do not see it. If a route
needs those, register it as JSON.

`kit.NewJSONHandler` / `NewJSONTypedHandler` register nothing; they return a
handler you mount yourself. (`kit.JSON` / `JSONTyped` are deprecated
spellings, as is `HandleSSE` — a name promising SSE behaviour while silently
skipping the chain is how a stream loses its middleware by accident.)

## Path and query parameters

A typed route fills its request struct from the request itself, so path and
query parameters are part of the typed route — not a reason to drop to a raw
handler:

```go
type getTodoRequest struct {
	ID      int64  `form:"id"       json:"id"`
	Keyword string `form:"keyword"  json:"keyword"`
}

kit.HandleJSONTyped(svc, "GET /todos/{id}", func(ctx context.Context, req getTodoRequest) (Todo, error) {
	return todos.Get(ctx, req.ID) // req.ID came from the path
})
```

- GET and HEAD fill the struct from the query string **and** the path values.
- Every other method reads a strictly decoded JSON body and then the path
  values — so one struct can carry body fields and `{id}` at once.
- A request that carries no body at all decodes to the zero value, which is
  what lets a body-less `POST /todos/{id}/done` stay typed; the handler's
  validation answers "nothing sent" in the application's own words.
- Fields are named by the `form` tag, else the `json` tag, else the lowercased
  field name, and a path value wins over a query parameter of the same name —
  the same rules the generated transports decode with.

A raw handler stays the answer when the response is not JSON (a 204, a file)
or for hand-rolled decode; `transporthttp.DecodePathRequest` and
`DecodeQueryRequest` fill a struct from a raw request on demand, and
`server.DecodeJSONBody` judges a body by the transport rules.

The call site mirrors the route. `client.NewTypedJSONClient[Req, Resp]`
returns a typed function — no type assertion at the call:

```go
call, err := client.NewTypedJSONClient[todoRequest, todoResponse](
	http.MethodGet, "http://todos.internal/todos/{id}")
todo, err := call(ctx, todoRequest{ID: 7, Keyword: "urgent"})
```

GET and HEAD fill the URL's placeholders and query string from the struct;
every other method fills the placeholders and sends the struct as the JSON
body — the server's path decode wins on overlap, so the client need not
strip path values from the body. Non-2xx answers surface as
`client.HTTPStatusError`, classified for retry decisions by the status code.
`transporthttp.EncodePath` and `EncodePathAndQuery` remain the building
blocks for hand-rolled clients: they fail rather than send a URL with an
unresolved `{placeholder}`.

## Route groups

A group states a prefix once instead of repeating it on every route:

```go
api := svc.Group("/api/v1")
kit.HandleJSONTyped(api, "POST /todos", createTodo)
kit.HandleJSONTyped(api, "GET /todos/{id}", getTodo)
v2 := svc.Group("/api/v2") // groups nest: both prefixes join
```

A group only joins patterns by default: the route goes through the component's
HTTP context, endpoint middleware (labeled with the joined pattern), recorders,
and JSON server options exactly as if the pattern had been written out in full.
`svc.Group("")` is the component's own view; `Handle`/`HandleFunc` work through
a group for the raw escape hatch.

A group can also carry middleware for its routes, stated once instead of
repeated per route:

```go
api := svc.Group("/api")
api.UseEndpointMiddleware(security.RequireAuthenticated()) // endpoint layer
api.UseHTTPMiddleware(requestAuditor)                      // HTTP layer
```

`UseEndpointMiddleware` runs inside the component's chain (and its recorders)
and outside the route's own middleware, so a group-level rejection is still
observed with the joined pattern. `UseHTTPMiddleware` wraps the route's
handler inside the component's HTTP middleware and touches no endpoint chain.
Both are first-outermost, are inherited by nested groups, and govern the
routes registered **after** the `Use` call — the chain is fixed at
registration, which is what makes it readable.

## Beyond the JSON body

- **Pagination**: `transporthttp.ParsePage(r)` reads the page convention, and
  `transporthttp.NewPageResult(items, page, total)` writes the result
  envelope. See the transport README's pagination section.
- **Multipart**: `server.ParseMultipartForm(r, limits)` with
  `server.MultipartLimits`; oversize and wrong-media-type failures arrive
  classified (413/415) like every other error.
- **File downloads**: `server.WriteAttachment(w, name, body)` sets the
  disposition headers.
- **Request context**: `transporthttp.PopulateRequestContext` copies method,
  URI, and header fields into the context for handlers that must not touch
  the request directly. Request IDs trust no caller by default: an inbound
  `X-Request-ID` longer than `transporthttp.MaxRequestIDLength` or failing
  `kit.WithRequestIDValidator` is replaced with a fresh one.

## Options that shape every route

| Option | Effect on routes |
| --- | --- |
| `WithHTTPMiddleware(mw...)` | Wraps **every** route including probes; first argument is outermost. |
| `WithEndpointMiddleware(mw...)` | Around every JSON/SSE endpoint; each wrap is labeled with the route pattern, so metrics and logs name the route. |
| `WithJSONServerOptions(opts...)` | Component-wide response envelope and error encoder. Per-route options run after and win. |
| `WithJSONMaxBodyBytes(n)` | Component-wide strict-JSON body limit; `<= 0` disables the size check, strictness stays. One different route: the body-limit registrations. |
| `WithTimeout(d)` | Deadline in the request context of **every** route, raw handlers included. A handler that ignores cancellation still runs to completion. No per-route escape — see SSE. |
| `WithRequestID()` / `WithRequestIDValidator(v)` | Extracts and echoes `X-Request-ID`; the validator overrides the trust policy. |
| `WithProbePaths(p)` / `WithoutProbes()` | Rename or drop `/health`, `/livez`, `/readyz`; an empty path omits that route. |
| `WithHTTPRecorder(recs...)` | HTTP-level observations (method, route, status). Covers application routes, never probes. |
| `WithRecorder(recs...)` / `WithMetrics(m)` | Endpoint-level outcomes, labeled with the route pattern. |

Errors a route answers are not silent: a server constructed without an
explicit error handler logs them through slog's default logger at Warn
level, and `transport.NopErrorHandler` restores silence on purpose. Why the
HTTP recorder is installed per route rather than around the mux:
`Observation.Route` comes from `http.Request.Pattern`, which a wrapper around
the mux never sees. An unmatched URL reports an empty `Route`, which is what
keeps an unrouted path from becoming an unbounded metric dimension.

## SSE routes

```go
kit.HandleSSETyped(svc, "GET /events/{tenant}", func(
	ctx context.Context, req eventsRequest, w *httpserver.SSEStream,
) error {
	for {
		select {
		case <-kit.Stopping(ctx):
			return nil
		case evt := <-events:
			if err := w.EventJSON("todo", evt); err != nil {
				return err
			}
		}
	}
}, decodeEventsRequest)
```

The decoder takes the `*http.Request`, so a stream can read path and query
parameters itself. The stream
runs inside the endpoint chain: `WithTimeout` bounds the **whole stream** and
there is no per-route escape from it, so a long-lived stream belongs on a
second `kit.NewHTTP` component of its own. Once the stream has answered 200
an error can only be recorded, not rendered. During shutdown the handler
watches `kit.Stopping(ctx)` and ends its own response. Header semantics and
hook details live in the [transport README](../transport/README.md), the
shutdown interaction in [lifecycle](lifecycle.md).

## gRPC routes

A gRPC service registers on the `*grpc.Server`, exactly as protobuf intends:

```go
comp, err := kitgrpc.New(":8081") // kit/grpc component
if err != nil {
	log.Fatal(err)
}
idl.RegisterTodoServiceServer(comp.Server(), todoServer)
```

The `kit/grpc` component pre-installs the trace-context interceptors and
registers the gRPC health service (`Check` and `Watch`), backed by the same
probe registry as the HTTP component. The handler side is
`grpcserver.NewServer(endpoint, decode, encode)` from `integrations/grpc`,
with the same `ServerBefore`/`ServerAfter`/error-encoder option shapes as
HTTP; `grpcserver.Interceptor` is what puts the full method name into the
context. One handler, two protocols: a `transport.Binding` carries a typed
function as both `TypedEndpoint()` (for `HandleJSONTyped`) and `GRPCServer()`
(for the generated register call). Per-call metrics use the recording
interceptors labeled with the full method name.

## Routes in generated projects

`microgen` registers one route per unary method through
`transport/http/server.RouteRegistrar` — a `*http.ServeMux` satisfies it, and
registration is the only place a handler and its pattern are in scope, which
is why per-route middleware is installed there. The `-prefix` flag sets a
route prefix; with `-prefix /api` a `userservice` route becomes
`/api/userservice/...`.

Your routes go in the write-once hook `cmd/custom_routes.go`:

```go
func registerCustomRoutes(r *http.ServeMux) {
	// registrations write directly to the mux, standard library only
}
```

To surface them in `/debug/routes` and the startup listing, describe them in
`customRouteDescriptions()` as `"METHOD /path"` strings.

When `debug.routes_enabled` is set, `GET /debug/routes` answers a JSON array
of `{"Method","Path","Handler"}` entries — health, debug, generated, custom,
OpenAPI, and MCP routes alike, with the handler role of each. When
`debug.print_routes` is set, the same list is logged at startup. Per-route
metrics are wired by wrapping the registrar:
`httpserver.DecorateRoutes(registrar, httpserver.RecordingMiddleware(...))`.

## Health routes and the three healths

`kit.NewHTTP` mounts `/health` (all), `/livez` (liveness), and `/readyz`
(readiness) from its probe registry; `WithLivenessCheck` and
`WithReadinessCheck` populate it, and `Probes()` hands the registry back for
custom mounting. Three different things answer "is it healthy":

- the `health` package — **this process's** registry and probe routes (see
  its [README](../health/README.md) for mounting on your own mux or a second
  port);
- kit's probe options — the assembly of that registry onto the HTTP
  component;
- `sd/health` — whether **remote instances** pass active probes. Neither
  answers the other's question.

## Where to go next

- [core concepts](concepts.md): where a route sits in the request path
- [middleware](middleware.md): the chain a JSON route joins
- [errors](errors.md): how a returned error becomes a status code
- [transport README](../transport/README.md): codecs, SSE hooks, pagination,
  multipart in full
