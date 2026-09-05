# Changelog

English | [简体中文](CHANGELOG_zh.md)

## [2.9.0] - Release Candidate

Layering. The goal is that the pieces be usable one at a time, so the contract
layer stops carrying a policy and the dependency direction is enforced rather
than described.

### Changed

- Trace context now crosses every transport without wiring. `kit.NewHTTP`
  extracts an incoming `traceparent` on every route, `kit/grpc.New` installs the
  extracting unary and stream interceptors — chained, so `grpc.UnaryInterceptor`
  stays free for the caller — and `integrations/grpc/client.NewClient` injects
  the context's trace context into outgoing metadata by default. Propagation
  that has to be opted into is propagation that breaks at the first hop somebody
  forgot.

- `oteladapter` instruments follow the OpenTelemetry conventions.
  `go_kit.endpoint.duration` is in seconds rather than milliseconds, and a failed
  call is the same `go_kit.endpoint.requests` series carrying `error.type` — the
  kind the error names through `interface{ ErrorKindName() string }` — instead of
  a second counter. `go_kit.endpoint.errors` and the `outcome` attribute are
  gone: error rate is now a ratio over one instrument. Dashboards reading the old
  names or the millisecond unit need updating.

- `slogadapter.Telemetry.Middlewares` is `[]NamedMiddleware`, each entry
  carrying the label it reports under, instead of `[]endpoint.Middleware` labeled
  by position. Appending a fourth middleware and calling `Apply` used to index
  past the label list and panic. `TelemetryConfig.Operation` is required only
  when the logging dimension is assembled, since it names the log record.

- The strict JSON server answers a media type it does not speak with 415, before
  the body is read, through `JSONDecodeOptions.RequireJSONContentType` — on in
  `StrictJSONDecodeOptions`, so every high-level JSON helper gets it. A request
  naming `text/plain` used to be accepted whenever its bytes happened to parse. A
  request with no `Content-Type` at all is still accepted: a body-less request
  carries none, and demanding one would refuse requests that are correct.
  `application/json`, any `+json` suffix, and a UTF-8 charset parameter are
  accepted; another charset is not, because JSON is UTF-8 by RFC 8259.

- An empty body reports itself. The message was the literal string `EOF` — the
  decoder's situation, not the caller's mistake — and the code was
  `bad_request.invalid_json`. It is now `request body is empty` with code
  `bad_request.empty_body`, and `ErrJSONBodyEmpty` is matchable with `errors.Is`.

- A `ReadinessProvider` attached to a `Host` with no component that serves
  readiness probes is now a `NewHost` error. It used to be collected and
  discarded: the point of implementing the contract is that an orchestrator can
  see the answer, and a component warming up silently behind a probe that reports
  ready is worse than one that never claimed to warm up. `kit.ReadinessSink` is
  the contract a Host looks for — `Probes() *health.Registry` — which both the
  HTTP component and the gRPC component satisfy.

- A CSRF token now authorizes one session for a bounded time. `CSRFConfig.SessionID`
  is required and `TokenTTL` defaults to 12 hours; the HMAC covers the nonce, the
  issue time, and the session, so a token minted for one caller is refused for
  another and a leaked token stops working. Previously the signature covered only
  a random nonce, which made every token the server ever minted valid for every
  user forever — enough for login CSRF or token fixation given any way to place a
  cookie in the victim's jar.

  An unsafe request whose session cannot be resolved is refused. A safe request
  without a session is served without a token, so the token arrives with the
  first request after sign-in.

- Responses that mint a CSRF token declare `Cache-Control: no-store` and
  `Vary: Cookie`. A `Set-Cookie` that identifies one session is not a response a
  shared cache may replay to the next user.

- `SecurityHeadersConfig.AssumeHTTPS` and `CSRFConfig.AssumeHTTPS` declare that
  TLS terminates upstream. HSTS is emitted for HTTPS requests and the CSRF
  same-origin check compares against the request's own origin, both of which
  needed a scheme this process cannot observe when it serves plaintext behind a
  load balancer. A forwarded scheme from `NewTrustedProxy` still wins, being a
  measurement rather than a declaration.

- Go 1.26.0 is the minimum. Every module's `go` directive, the workspace, the CI
  lanes, the README badges, and the generated project template move together.
- The reviewed API snapshot hashes exported declarations only, not doc-comment
  prose. Prose used to be in the hash, so fixing a typo in a comment was a
  release-gate event and every documentation improvement needed a snapshot
  refresh. The guarantee worth keeping is the one tests structurally cannot
  give: nothing asserts "these and only these symbols are exported", and a
  published library cannot take an accidental export back. Whether a comment is
  accurate is a review question. `TestDeclarationsOnly*` pins the new behaviour
  from both sides — prose edits do not move the hash, added, removed, renamed,
  and re-signatured symbols do.

- `endpoint` imports only the standard library. It defined what an `Endpoint`, a
  `Middleware`, and a `Chain` are while importing `apperror`, so nobody could
  take those three without also taking the framework's error taxonomy. It now
  speaks the structural classification contract — `interface{ ErrorKindName()
  string }` — which is the one `apperror` already documents for callers that must
  not depend on it, and the one the generated SDKs use.

  Consequences for an application:

  - `endpoint.ErrCircuitOpen`, `ErrBulkheadFull`, `ErrBackpressure`,
    `ErrRateLimited`, `ValidationError`, and the bulkhead's wait error implement
    `apperror.KindNamer` and no longer implement the typed `apperror.Kinder`.
    Statuses and gRPC codes are unchanged: both encoders read `Kinder` first and
    fall back to `KindNamer`. Code matching on the typed contract must switch to
    the string one.
  - `endpoint.DefaultRetryable` reads only `ErrorKindName`. An `apperror`
    implements both, so nothing changes for it; a custom error that implements
    only `Kinder` is still classified correctly on the wire but is no longer
    retried.
  - `DefaultPanicHandler` returns a framework-owned classified error instead of
    an `*apperror.Error`. Its kind (`internal`), code (`endpoint.panic`), and
    message are unchanged.

### Added

- `observability/otel` assembles the pipeline, not only the middleware:
  `Setup(ctx, Config)` builds the tracer and meter providers, the OTLP exporters
  over gRPC or HTTP, and the resource from `ServiceName`, `ServiceVersion`, and
  `Environment`; installs them globally together with the W3C trace context and
  baggage propagator; and returns `Providers` with `Tracer()`, `Meter()`, and one
  idempotent `Shutdown`. `Config.Signals` selects traces, metrics, or both, and
  `SpanExporter` / `MetricReader` replace the OTLP pipeline. Correct
  OpenTelemetry wiring used to be several dozen lines of application code, and
  the piece most often missing was the global propagator — without it a service
  emits spans no downstream service can join.

- `oteladapter.NewHTTPMetrics` records `http.server.request.duration` under the
  HTTP semantic conventions, with `http.request.method`, `url.scheme`,
  `http.route`, and `http.response.status_code`, so response status is alertable
  from metrics. `transport/http/server` gained the contract it reports through —
  `Observation`, `Recorder`, `RecorderFunc`, `RecordingMiddleware` — and
  `kit.WithHTTPRecorder` installs it per route, which is the only place the
  matched pattern exists. A request that matched no route carries no
  `http.route` rather than the raw URL path.

- `integrations/grpc` propagates W3C trace context in both directions:
  `TraceparentKey`, `ExtractTraceparent`, `InjectTraceparent`,
  `TraceparentUnaryServerInterceptor`, `TraceparentStreamServerInterceptor`, and
  `TraceparentUnaryClientInterceptor`. A gRPC service had no correlation
  plumbing at all, so a trace ended at the first gRPC hop.

- `interaction.Runtime.WithLogger` reports every tool call through the caller's
  `*slog.Logger` with the request path's own attributes — `duration`, `success`,
  `error`, `trace_id`, `request_id` — so an MCP tool call joins the HTTP request
  that carried it. A failed or rejected call logs at Error because nothing else
  reports it: a tool failure travels back to the model as a result, not as a
  transport error. A nil logger reports nothing, which is the default.

- `slogadapter.Signals` selects the telemetry dimensions `NewTelemetry`
  assembles — `SignalTracing`, `SignalMetrics`, `SignalLogging`, zero meaning
  all — so a service whose metrics come from an OpenTelemetry meter can take the
  logging dimension without recording every call twice.


  The validator's rejection of control characters is not cosmetic: the ID is
  echoed into a response header, so a value carrying CR or LF would be header
  injection. The tests say so.

- `kit/grpc` serves the standard gRPC health service. `grpc.health.v1.Health/Check`
  is answered from the component's probe registry, evaluated per call rather than
  read from a status somebody remembered to set, so `grpc_health_probe` and
  Kubernetes' native gRPC probe orchestrate a gRPC-only service on the same
  answer an HTTP service serves at `/readyz`. A named service reports `NotFound`,
  because the registry describes the process rather than one service within it;
  `Watch` is not implemented, since it would have to poll the checks to
  synthesise transitions.

- `health` holds the probe engine: a `Registry` of liveness and readiness
  checks, the concurrent evaluation with its per-check timeout, single-flight
  gating and panic containment, the `Report` shape probes have always returned,
  and an HTTP handler and `Mount` for it. It was 213 lines inside `kit`, all of
  it unexported and bound to `*kit.HTTP`, so a gRPC service had no readiness
  surface at all and a service assembled from the transport packages had to
  reimplement one. `kit` now mounts the registry rather than owning it.

  `kit.HealthCheck` is `health.Check` and `kit.DefaultHealthCheckTimeout` is
  `health.DefaultTimeout`, so existing configuration keeps compiling.
  `kit.WithProbePaths` serves the probes on routes of your choosing,
  `kit.WithoutProbes` serves none, and `kit.HTTP.Probes()` returns the registry
  — enough to add a check after construction or to put the probes on a separate
  administrative listener.

- `kit.WithRegistrar` and `kit.RegistrarLifecycle` publish a service instance for
  as long as the Host runs. `sd.Registrar` and `kit.Lifecycle` existed but did not
  meet, so every service hand-wrote the adapter — the same three lines, once per
  service, in the projects this came from.

  The doc comment carries two things a signature cannot. Attach the registration
  *after* the server that serves the traffic: components start in declaration
  order and stop in reverse, so that is what publishes the address only once the
  listener accepts and withdraws it before the listener goes away. And do not
  carry over the `Deregister(); Register()` idiom from go-kit's `sd/etcd` — it
  worked around that implementation registering with etcd's `Create`, which fails
  when the key already exists, so a restart after an unclean exit could not
  re-register. Here `Register` overwrites a per-instance key held on a lease, and
  it returns its error instead of logging it.

- `TestComponentsDoNotDependOnAssembly` in `tools`: no package outside `kit` and
  `cmd/microgen` may depend on them, so a component cannot quietly reach back
  into the assembly layer. Layering that is only written down does not survive.

### Performance

Benchmarks now exist for the paths every request crosses — `Chain`, the JSON
server round trip, `balancer.Pick`, `feedback.Table`, `Metrics.Observe`, and
`TracingMiddleware` — so an optimization can be shown to work and a regression
can be seen. Figures below are `go test -bench . -benchmem` on one machine;
they are ratios worth trusting, not absolutes.

- `feedback.Table` reads without a lock. A selection asks every candidate for its
  load or score, and each of those asks took a read lock on the one mutex the
  recording path also holds. The entry table is now published copy-on-write and
  each measurement is an atomic field; recording, resetting, and retaining still
  take the mutex, so in-flight accounting and the retirement lifecycle keep the
  ordering they had. A reader can see fields from either side of one recording,
  which is what a load heuristic can afford. Measured over nine candidates:
  566 ns → 46 ns per selection read with eight callers in flight, 543 ns → 37 ns
  with sixty-four. The whole read-then-record round trip goes 1567 ns → 534 ns at
  eight callers and 1549 ns → 533 ns at sixty-four, and `balancer.Pick` with
  least request goes 779 ns → 489 ns under twelve.
- One client-side request no longer copies the instance snapshot or allocates a
  callback it does not need. `endpointer.Cache.InstanceEndpoints` returns the
  published snapshot instead of a copy of it — the slice is replaced whole on
  every discovery update and never edited in place, so a reader keeps a
  consistent view; do not modify it. `balancer.Pick` returns a shared no-op
  callback when the strategy keeps no feedback state, and one guarded callback
  when it does. Measured for round robin: 130 ns/96 B/4 allocs → 55 ns/24 B/1
  alloc over one instance, 224 ns/552 B/4 → 102 ns/224 B/1 over nine, and
  203 ns → 87 ns across twelve callers. Least request, which consults the
  feedback table per candidate, goes 471 ns/7 allocs → 372 ns/6.
- Correlation identifiers travel in one context value instead of three.
  `TracingMiddleware` writes one node per request rather than up to three, and
  `TraceContextFromContext`, `TraceIDFromContext`, and `RequestIDFromContext`
  each do one lookup. A later `With*` still wins over an earlier value, because
  it replaces the whole set. Measured: 495 ns/296 B/10 allocs → 341 ns/192 B/5
  allocs minting a trace, 487 ns/9 allocs → 317 ns/4 allocs joining one; a
  five-middleware chain drops from 1074 ns/14 allocs to 791 ns/9 allocs.
- Request IDs come from the same allocation-lean hex path span IDs use, rather
  than `fmt.Sprintf`. The degraded path taken when the entropy source fails now
  produces an identifier of the length and alphabet the W3C specification
  requires; it previously padded a single 64-bit value out to 32 characters.
- `endpoint.Metrics` keeps its single mutex. Atomic tallies were tried and
  measured slower — 23.6 ns to 46.4 ns per record single-threaded, 60 ns to
  109 ns across twelve — because one lock covers ten field updates while atomics
  pay a contended cache line each. The figures are in the type's doc comment so
  the idea is not retried blind.

### Fixed

- `microgen -from-db` validates that it was given a `-dsn`. Without one it
  reached `sql.Open(driver, "")` and surfaced the driver's empty-DSN parse error,
  which names nothing the caller can act on. `-dbname` stays optional: a
  file-backed database such as SQLite has no database name to give.
- `NewCORS` declines the opaque `null` origin, which `NewCSRF` already declined.
  Sandboxed documents, `data:` and `file:` pages, and laundered redirects all
  present that origin, so allowing it with credentials granted them a
  credentialed cross-origin path.
- Every CORS answer declares `Vary: Origin`, including the rejections and the
  no-origin passthrough. A shared cache could otherwise store a 403 keyed without
  the origin and serve it to a legitimate one.
- `TestEndpointHasOnlyStandardLibraryImports` had an explicit carve-out
  permitting the `apperror` import it was supposed to prevent. Removed.
- `apperror.KindNamer` claimed every classification site in the framework reads
  `Kinder` first and falls back to it. The endpoint package now reads only
  `KindNamer`; the doc says so.

## [2.8.1] - 2026-09-05

A contract audit of every runtime package. Each item below is a place where the
code and its own documented contract disagreed. The version is a patch because
the fixes are corrections, not a new feature set; several of them do change
observable behaviour, and those are listed first.

### Fixed - behaviour

- `security`: an anonymous or identity-less subject no longer satisfies
  `RequireAuthenticated` or `RequireRole`. `Middleware` also stops discarding a
  subject that carries only roles or claims, which previously turned an
  authenticated caller into a 401.
- `interaction`: a nil `AuthorizerFunc` denies instead of allowing. It reaches an
  `Authorizer` field only as a typed nil, which `AuthorizationHook`'s own nil
  check cannot see, so this was a fail-open path.
- `interaction`: `Runtime.CallTool` runs `AfterToolCall` on the hooks that
  already admitted a call when a later hook rejects it, so `AuditHook` records
  the denial. Rejections were previously invisible to an audit sink.
- `kit`: a panicking health check is reported as an unhealthy check instead of
  taking the process down. Checks run in their own goroutines and probes are
  unauthenticated requests.
- HTTP error encoders: an error that implements `transporthttp.PublicMessager`
  now decides its message even when that message is empty, which means "nothing
  here is for a client". `apperror.WrapCause` therefore keeps its documented
  promise that the cause stays internal; the `err.Error()` fallback below 500
  remains for errors outside the contract.
- `TextErrorEncoder` resolves its message through the same rule as the JSON and
  plain-text encoders. It previously applied `PublicMessager` at 500 too, and
  named 499 "HTTP error" instead of "Client Closed Request".
- `integrations/grpc`: `StatusError` reports a public message that names the
  upstream code without the upstream description, mirroring
  `client.HTTPStatusError`.
- An over-limit request body classifies as 413 on both the JSON and raw codec
  paths, matching `ParseMultipartForm` and `RawBodyCodecWithMaxBytes`'s
  documentation. It was 400 through `JSONDecodeError` and 500 when surfaced
  directly.
- `endpoint.TimeoutMiddleware` and `sd/retry`: a non-positive timeout imposes no
  deadline instead of handing the call an already expired context.
- `endpoint.BackpressureMiddleware`, `endpoint.InFlightMiddleware`, and
  `sd/retry.Retry`: a non-positive limit is clamped to 1, as
  `BulkheadMiddleware` already did. A zero limit previously rejected every
  request.
- `sd`: a wrapping strategy or balancer that discards a successful inner `Pick`
  releases its `Done`. `selector.Filtered`, `feedback`, `selector`, and
  `balancer` each leaked an in-flight reservation on their refusal paths.
- Success response encoders ignore a `StatusCoder` value outside 100-999 rather
  than passing it to `WriteHeader`, which panics on it.
- `interaction`: a `Runtime` missing `Sessions`, `Events`, or `Tools` reports
  `ErrRuntimeNotConfigured` instead of panicking on the first call, and
  `StartSession` releases the session it created when the started event cannot be
  emitted.
- `observability/otel`, `observability/slog`, `integrations/zap`: a panicking
  endpoint is reported as a panic. The otel span ended Unset with no recorded
  error, zap logged "endpoint call succeeded", and slog logged nothing.
- `DefaultErrorEncoder` honors `json.Marshaler` only when the returned error
  implements it directly. `errors.As` walked the chain, so a marshalable cause
  could replace the whole body — the one path around the message rule, and
  exactly what `apperror.WrapCause` exists to prevent.
- The over-limit body error no longer says "json" on a route that is not JSON.
  The same bounded reader guards protobuf and other raw bodies, and below 500 the
  message is what reaches the client. It still matches
  `errors.Is(err, ErrJSONBodyTooLarge)`.

### Changed - generated code

Regenerate to pick these up; the runtime contracts they were violating are
described in the entries above.

- The generated middleware chain installs `endpoint.FailerMiddleware` innermost,
  so a `Failer` response is not counted as a success by the generated metrics and
  logging while the transport answers an error.
- Generated metrics record into one collector labeled per operation, through
  `endpoint.RecordingMiddleware`. They used one unlabeled collector per operation
  in an unexported map, so `SnapshotFor` and `Operations` were always empty and
  nothing in the generated project could read the tallies. A generated `Metrics()`
  accessor now exposes the collector.
- The generated SDK's `APIError` satisfies the transport error contracts:
  `StatusCode`, `ErrorKindName`, `PublicMessage`, and `Retryable`. A service
  relaying it got a blanket 500 with the upstream body embedded in the message.
  Its `StatusCode` field is now named `Status`, because `StatusCode` is the
  method name the contract requires.
- The generated SDK's `WithTimeout` applies as a per-call context deadline, so it
  holds when `WithHTTPClient` replaces the client. It was silently dead in that
  case. `WithTimeout`, `WithHTTPClient`, and `WithMaxResponseBodyBytes` now share
  one policy for bad input: ignore it and keep the default.

### Added

- `endpoint.FailerMiddleware` and `endpoint.ResponseError`, with
  `Builder.WithFailer`. A `Failer` response reaches the transport with a nil
  error, so metrics, the circuit breaker, and retry all counted it as a success.
  Installed innermost, the middleware converts it first and changes nothing a
  client observes.
- `security.Subject.Authenticated`, the check that distinguishes a subject being
  present in a context from a principal having been established.
- `sd.Release`, the helper a wrapping strategy uses to hand back a `Done` it
  cannot use.
- `interaction.ErrRuntimeNotConfigured`.

### Fixed - documentation

Doc comments are part of the reviewed API surface here, so these are recorded
rather than folded into the changes above.

- `JSONErrorEncoder` had no doc comment; the block describing it was attached to
  `JSONErrorEncoderWithKindMapper`. `RawBodyCodec`'s comment started
  mid-sentence.
- `sd/endpointer` claimed round-robin and random balancers accept the narrower
  `Endpointer`, and `sd/client.NewEndpoint` claimed to compose an `Endpointer`.
  Everything in the module returns an `InstanceEndpointer`.
- `SubjectFromContext` claimed its boolean is false for anonymous callers.
- `client.KindForStatus` claimed to be the inverse of
  `server.HTTPStatusForErrorKind`. That mapping is not injective:
  `KindAlreadyExists` and `KindConflict` both answer 409, and
  `KindDeadlineExceeded` shares 504 with a 408. Two kinds therefore do not
  round-trip over HTTP, while all of them do over gRPC.
- `apperror.Kind` did not document that the empty kind normalizes to
  `KindInternal`, which every constructor and `ErrorKind` do.
- `docs/concepts.md` attributed `endpoint.ValidationError`'s 400 to
  `PublicMessager`; it comes from `apperror.Kinder`. The prose in
  `transport/README.md` and `docs/errors.md` still described the superseded
  message rule.

## [2.8.0] - 2026-09-04

This is the first public release of the current `go-kit/v2` product line.
The repository ships as one Go module:

```text
github.com/dreamsxin/go-kit/v2
```

Runtime packages, transport adapters, service-discovery providers,
observability adapters, and `cmd/microgen` are versioned together under the
same module and root tag. `examples`, `tools`, and
`tools/contractcheck` remain repository-only workspace modules.

### Included

- `Service -> Endpoint -> Transport` service architecture.
- Typed HTTP JSON handlers, custom codecs, SSE, gRPC adapters, and generated
  Go/TypeScript clients.
- Transport-neutral application errors with consistent HTTP and gRPC mapping.
- Endpoint middleware for validation, timeout, tracing, metrics, recovery,
  rate limiting, circuit breaking, fallback, backpressure, bulkhead, and retry.
- Service discovery snapshots, endpoint caching, balancing, retry, health
  checks, passive ejection, feedback, and long-lived connection accounting.
- Interaction runtime with sessions, tools, resources, prompts, authorization,
  audit hooks, and MCP Streamable HTTP.
- Configuration, OpenAPI, JSON Schema, database scaffolding, and deterministic
  `microgen` extension workflows.
- Explicit lifecycle ownership, bounded shutdown, request correlation, and
  package-level dependency gates.

### Release Contract

- The only published tag is the root `v2.8.0` tag.
- Historical v2 module tags (root and nested) have been removed.
- Future releases use one version and one root tag.
- Behavioral and API changes are recorded in this file; no separate migration
  history is maintained for this initial public baseline.
