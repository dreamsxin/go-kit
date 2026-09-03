# Changelog
English | [简体中文](CHANGELOG_zh.md)

All notable v2 changes are recorded here. Legacy history remains available
through the immutable v0 and v1 tags.

## [Unreleased]

### Added

- Three more selection strategies in `sd/balancer`, so round robin is no longer
  the only option: `NewRandom` (uniform draw, avoids the lockstep a shared
  counter causes across many clients), `NewWeightedRandom` (probability
  proportional to a weight per instance; weight zero drains an instance
  without waiting for service discovery), and
  `NewConsistentHash` (a virtual-node ring, `WithReplicas`, `DefaultReplicas`,
  so keys move only when their owning instance leaves). Least request is
  assembled instead of constructed — see `feedback.Table.LeastRequest` — because
  it needs a measurement store and the endpoint layer must not depend on one.
- `sdclient.WithBalancer` replaces the selection strategy. Previously
  `sd/client.NewEndpoint` hardcoded round robin and a custom balancer meant
  bypassing the constructor and wiring `endpointer`, the balancer, and
  `sd/retry` by hand.
- `sd.Outcome`, `sd.Done`, and `sd.Picked`: every `Balancer.Pick(ctx, request)`
  returns the selected instance, endpoint, and a completion callback so retry
  and direct callers can feed latency, errors, and bytes back into selection.
- `endpointer.InstanceEndpoint` and `endpointer.InstanceEndpointer` report which
  instance produced each endpoint. Weighted, least-request, and hash strategies
  need that identity; round robin and random do not and still accept
  `endpointer.Endpointer`.
- Instance metadata: `sd.Instance` now carries `Metadata map[string]any`
  alongside `Address`, so registrations can report static labels (zone, version,
  protocol, capability, weight, tenant) and discovery can select on them.
  `sd.Addresses` builds a snapshot from bare addresses for tests, local
  development, and registries that expose no labels.
- `sd.MetadataString`, `sd.MetadataInt`, and `sd.MetadataBool` coerce label
  values, so a predicate written against `5` still matches the `"5"` a registry
  returns.
- Subset filtering in `sd/endpointer`, modelled on Envoy's subset load balancer:
  `Filter` (no fallback) and `Prefer` (fall back to the full set when the
  filtered set is empty), driven by the `sd` predicates. Zone-aware routing is
  composition, not a zone-aware variant of every strategy.
- `balancer.WeightFunc`, `balancer.MetadataWeight`, and
  `balancer.DefaultWeightKey` read a weight from registration labels.
- `consul.MetaRegistrarOptions` reports static labels with the registration, and
  the Consul instancer lifts each entry's service `Meta` into
  `Instance.Metadata`. A metadata-only change is broadcast as a change. Live
  load stays out of the catalog: `feedback.Table.LeastRequest` measures it in
  process.
- New `sd/selector` package: selection strategies over instance snapshots,
  independent of endpoints. `Strategy.Pick(ctx, request, instances)`,
  the strategies `RoundRobin`, `Random`, `WeightedRandom`, `Scored`,
  `LeastRequest`, and `ConsistentHash`, the sources `Static`, `Subscribe`,
  `Filter`, and `Prefer`, and `New`/`Select` to bind them. Callers that only need
  an address — a proxy that dials the instance itself, an API that answers "where
  should I connect?" — assemble this and never run an endpoint factory, so no
  connection is built for an instance nobody calls.
- `balancer.New(source, strategy)` turns any `selector.Strategy` into a
  `sd.Balancer`, so a custom strategy is usable by `sd/client` and `sd/retry`
  without reimplementing endpoint lookup. Request data is always passed through
  the one strategy method.
- `sd/feedback.Table` records EWMA latency/error rate, bytes, in-flight calls,
  and when each address was first seen. `Table.Score`, `Table.Load`,
  `Table.LeastRequest`, `Table.FirstSeen`, and `Table.Wrap` connect local
  measurements to scored selection, least-request selection, and slow start.
  `feedback.Follow(instancer, retainers...)` and `Table.Retain(instances)` drop
  measurements for addresses that have left discovery, so a long-running table
  stays the size of the service rather than the size of its deployment history.
  `Table.Reset(instance)` discards an address's measurements while keeping its
  first-seen time and in-flight count. `feedback.WithClock` injects time for
  tests.
- `feedback.Ejector` and `feedback.EjectionPolicy`: passive outlier detection
  modelled on Envoy. `Ejector.Filter()` removes instances that exceed
  `MaxErrorRate`, `MaxLatency`, or `MaxInFlight` once `MinSamples` calls have
  been observed; `MaxEjectionPercent` (default 50) refuses to eject when too
  much of the candidate set looks unhealthy, since a pool failing as a whole
  usually means a shared dependency. An ejection expires after `BaseDuration`,
  doubled per previous ejection of that address and capped at `MaxDuration`,
  and expiry resets the measurement that caused it — otherwise a decaying
  average that never sees traffic would make the first ejection permanent.
- `feedback.Retainer` and the package-level `feedback.Follow`: one subscription
  to the instancer drives the table and every ejector, so a departed address is
  forgotten in one place. A discovery error no longer looks like "every instance
  is gone".
- `sd.InstanceFilter` and `sd.Keep(match)`, plus `selector.Filtered(strategy,
  filters...)`: a set-level filter contract for policies that cannot be decided
  per instance. Passive ejection needs to know how much of the pool it is about
  to remove before it removes any of it, so `Ejector.Filter` returns an
  `InstanceFilter` and its ejection cap is measured against the candidates in
  hand.
- New `sd/health` package: active probing as an `sd.Instancer` decorator, so
  every layer downstream is unchanged. `health.Check(source, probe, options...)`
  with `health.Probe`, `health.TCPProbe`, `health.HTTPProbe`, and options for
  interval, timeout, thresholds, concurrency, logger, and initial state.
  Passive detection only sees instances that receive traffic; probes also see
  cold and unreachable ones. An instance counts as healthy until its first probe
  completes, and when nothing passes the checker republishes the unchecked set
  rather than an empty one, so a broken probe cannot black-hole the service.
- `selector.Ranker`, `selector.NewRanker`, and `selector.ScoreFunc`: rank the
  snapshot and return the best N instead of picking one. This is the shape a
  routing service needs, where the caller connects on its own. Ties break on
  address, so two processes ranking the same snapshot agree.
  `Rank(ctx, request, n)` takes the request for the same reason `Strategy.Pick`
  does — a shortlist can depend on who is asking — and is deliberately not a
  `Strategy`: a ranker owns no call, so it has no `Done` to return.
- `selector.SlowStart` and `selector.FirstSeenFunc` ramp a new instance's weight
  up over a window. A cold instance otherwise wins every least-request and
  scored comparison and takes full traffic before its caches and pools are warm.
- `selector.LeastRequest`, `selector.LoadFunc`, `selector.WithChoices`, and
  `selector.DefaultChoices` in the selector layer, so in-flight selection is
  available without endpoints. `feedback.Table.LeastRequest` binds one to the
  table that measures it.
- `sd.StateKey`, `sd.StateReady`, `sd.StateDraining`, `sd.Draining()`, and
  `sd.Serving()`: the label convention for an instance that is still registered
  but should stop receiving new work. Draining is a property of the
  registration, so it lives in metadata rather than in the feedback table.

- `balancer.NewScored` and `selector.Scored` select on a caller-supplied score,
  the seam for load signals this process did not measure: a report the
  instances push, ORCA/LRS style out-of-band reporting, or any local table.
  Returning `false` excludes an instance, which expresses a hard filter without
  a second predicate.
- `sd.Match` and the label predicates `sd.MetadataEquals`, `sd.MetadataIn`,
  `sd.MetadataMatches`, `sd.HasMetadata`, `sd.And`, `sd.Or`, and `sd.Not`, so
  the endpoint layer and the instance layer filter with one set of predicates.
- New service discovery chapter (en/zh), `docs/service-discovery.md`: the
  outbound path end to end, what each layer owns and does not own, the
  `Pick`/`Done`/`Outcome` lifecycle, strategy choice, feedback and ejection,
  slow start, active probes, provider choice, long-lived connections, shutdown
  order, and a symptom-based troubleshooting table. `sd/README.md` keeps the API
  reference and links to it rather than repeating the reasoning; `feedback`,
  `health`, and etcd now have navigation entries of their own.
- `health.WithFailOpen` chooses what to publish when every instance has been
  probed and none passed. The default stays fail-open — the unchecked set, on the
  reasoning that a probe failing everywhere is more likely broken than the whole
  service being down — but the choice belongs to the caller: reaching a dead
  instance is worse than reaching none for a writer that must not double-apply.
  The same policy was already configurable one layer down, as
  `EjectionPolicy.MaxEjectionPercent`.

### Changed

- **Breaking:** `sd.Event.Instances` is `[]sd.Instance` instead of `[]string`.
  Build snapshots with `sd.Addresses("host:port", ...)` when there are no labels
  to report.
- **Breaking:** `endpointer.Factory` takes `sd.Instance` instead of a string
  address, so a factory can honour the labels that decide how to connect —
  scheme, TLS, protocol. Read the address as `instance.Address`.
- **Breaking:** `balancer.NewWeightedRandom` takes `balancer.WeightFunc`
  (`func(sd.Instance) int`) instead of `func(string) int`.
- **Breaking:** the subset predicates moved from `sd/endpointer` to the root
  `sd` package, because both the endpoint layer and the new instance layer read
  them: `endpointer.Match` → `sd.Match`, `endpointer.MetadataEquals` →
  `sd.MetadataEquals`, and likewise for `MetadataIn`, `MetadataMatches`,
  `HasMetadata`, `And`, `Or`, and `Not`. `endpointer.Subset` and
  `endpointer.PreferSubset` are now `endpointer.Filter` and `endpointer.Prefer`,
  matching `selector.Filter` and `selector.Prefer`: the same operation should not
  have two names at two layers.
- **Breaking:** `sd.Balancer` now exposes `Pick(ctx, request) (sd.Picked, error)`
  and `Close`; `Endpoint`/`EndpointFor` and `RequestBalancer` are removed.
  `sd.Picked.Done` receives `sd.Outcome` after every call, preserving instance
  identity for feedback and retry attribution.
- **Breaking:** `sd.Instancer` now exposes `Close() error`.
- **Breaking:** `balancer.NewLeastRequest` is removed, together with the
  `balancer.LoadFunc`, `LeastRequestOption`, `DefaultChoices`, and `WithChoices`
  aliases and the older `WithTable`/`WithFeedback` options. Assemble it as
  `balancer.New(set, table.LeastRequest(...))`. Two reasons: the endpoint layer
  imported `sd/feedback` unconditionally, so even a round-robin assembly
  compiled the optional feedback layer in; and the `table == nil` shorthand
  built a table the caller had no handle to, which therefore could never be
  `Retain`ed and grew without bound across rolling deployments. Least request
  itself lives in `selector.LeastRequest(load, options...)` so the instance
  layer can use it too.
- **Breaking:** passive health checking is now `feedback.Ejector` rather than a
  method on the table. `Table.Healthy`, `feedback.HealthPolicy`,
  `feedback.Policy`, `Table.Done`, and `Table.Follow` are removed; use
  `feedback.NewEjector(table, feedback.EjectionPolicy{...}).Filter()` with
  `selector.Filtered`, `Track`, and the package-level
  `feedback.Follow(instancer, table, ejector)`. Ejection now has a recovery
  path: without one the first ejection was permanent, because the measurement
  that caused it can never decay while the instance receives no traffic.
 - `sd/balancer` is now a thin layer over `sd/selector`: the weighted, scored,
  and hash strategies live there and the balancer adds endpoint lookup. The
  public balancer API keeps `WeightFunc`, `KeyFunc`,
  `ConsistentHashOption`, `DefaultWeightKey`, `DefaultReplicas`,
  `MetadataWeight`, and `WithReplicas` now alias or forward to the selector
  equivalents. Round robin and random still read endpoints directly, since they
  need no instance identity.
- The endpointer cache reuses a live endpoint when only an instance's labels
  change, so relabelling does not reconnect, and skips duplicate addresses in a
  snapshot rather than leaking the first endpoint's closer.
- `endpointer.NewEndpointer` declares `InstanceEndpointer` as its return type.

### Fixed

- **Breaking:** `selector.Selector.Select` and the package-level
  `selector.Select` return `(sd.Instance, sd.Done, error)`. The instance layer
  discarded the strategy's callback, so a table-backed strategy used through
  `selector.New` counted every selection as permanently in flight: the instance
  looked saturated to least-request and scored selection for the rest of the
  process, `Table.Reset` does not clear in-flight by design, and `Table.Retain`
  could never drop the entry. The callback is now forwarded, is never nil on
  success, and is idempotent, so `defer done(outcome)` is safe.
- `sd/health` no longer defeats `WithInitiallyHealthy(false)`. The fail-open that
  republishes the unchecked set now requires every instance to have produced a
  probe result; at startup nothing had been probed, so the option published
  exactly the instances it was asked to hide.
- `feedback.Table` closes its entry lifecycle. An address that left discovery
  while a call was in flight was kept until the next snapshot arrived and stayed
  forever if none did; the last completion now drops it. A retired address that
  returns to discovery keeps its measurements.
- `feedback.Table.Reset` discards the results of calls already in flight instead
  of recording them against the fresh state. Reset zeroes the sample count, so a
  straggler from before an ejection expired seeded the average at full weight
  rather than decaying into it: one late failure re-ejected the instance on the
  next selection, with a doubled window, and a call longer than the ejection
  window could hold an instance out of service indefinitely.
- `feedback.Table.Track` claims the entry and the in-flight slot in one critical
  section. Taking them separately let a concurrent `Retain` observe an in-flight
  count of zero for a call that had already picked its entry and delete that
  entry underneath it, which silently reset the instance's first-seen time and
  left a stray entry until the next snapshot.
- `feedback.Table.Retain` registers the arrival time of addresses in the snapshot
  that the table has not seen. `selector.SlowStart` ramps against
  `Table.FirstSeen`, and an instance nobody had called yet was unknown, which
  slow start treats as brand new — so the ramp began at the first call instead of
  at the moment the instance joined the service, which is the timestamp Envoy and
  NGINX both use.
- `sd/health` probes on a fixed worker pool instead of one goroutine per
  instance per round, and stops feeding the pool as soon as `Close` cancels.
  `health.Probe` now documents that it must return on context cancellation,
  since `Close` waits for the round in flight.
- Documentation stopped describing the removed `integrations/circuitbreaker` and
  `integrations/ratelimit` modules as if they still existed. `endpoint/README`
  told readers those modules "remain independently selectable components", the
  release runbook in `internal/docs/RELEASE.md` listed tag commands for both and
  omitted `integrations/etcd`, and the dependency report and roadmap still
  counted them. The breaker defaults, `CircuitBreaker.State()`,
  `RateLimiterFuncs`, and the multi-replica limiter note that only the deprecated
  READMEs documented now live in `endpoint/README`.

### Removed

- The `integrations/circuitbreaker` and `integrations/ratelimit` directories.
  Both modules were deleted in v2.4.0 when the middleware moved into `endpoint`;
  what remained was a deprecation README, and `go get` resolves old requirements
  through the module proxy and the existing tags, not through a file in the
  repository.


## [2.7.0] - 2026-09-01

### Added

- Route-level endpoint middleware in `kit`: `HandleJSONWithMiddleware` and
  `HandleJSONTypedWithMiddleware` compose per-route middleware closest to the
  handler, inside the component-level chain installed through
  `WithEndpointMiddleware`.
- MIGRATION documents the construct mapping and recommended order for moving
  legacy go-kit (v0/v1 style) codebases to v2.
- `server.AccessLogMiddleware`: standard-library access logging at the
  transport boundary (method, path, status, bytes, duration, trace ID);
  install with `kit.WithHTTPMiddleware`.
- The middleware chapter documents where each cross-cutting concern (logging,
  tracing, metrics, errors) belongs across the service, endpoint, and
  transport layers.
- New troubleshooting chapter (en/zh): symptom-based diagnosis — request
  correlation by request_id/trace_id, status-code causes (including which
  protection rejected a request), startup failures, readiness failures,
  database pool settings, logging configuration, and debug switches. The
  configuration chapter gains a full generated-sections reference.
- New customization chapter (en/zh): choosing log destinations (file
  storage, multi-writer, rotation guidance), writing custom endpoint and HTTP
  middleware with installation scopes, and an error-customization decision
  guide.
- `endpoint.Recorder` and `endpoint.Observation`: a metrics extension point.
  `RecordingMiddleware(operation, recorders...)` times each call and reports it
  under an operation label, so a Prometheus or OpenTelemetry bridge is an
  interface implementation instead of a fork of the middleware.
  `endpoint.Metrics` implements `Recorder` and gained `SnapshotFor` and
  `Operations` for per-operation numbers alongside the existing total.
- `kit.WithRecorder` registers external recorders, and both it and
  `kit.WithMetrics` label every observation with the route pattern, so
  `metrics.SnapshotFor("POST /users")` reports one route.
- `endpoint.RetryMiddleware` (Builder: `WithRetry`): retries transient failures
  with exponential backoff and full jitter. `DefaultRetryable` retries
  `apperror.KindUnavailable` and errors that classify themselves through
  `interface{ Retryable() bool }`, never context errors, local rejections, or
  unclassified errors; `WithRetryable` and `WithRetryBackoff` replace either
  policy. A retry hint carried by the error overrides the schedule, capped by
  `endpoint.MaxRetryAfterHint`.
- `client.HTTPStatusError` classifies itself, so a client endpoint composes with
  the same middleware as a server endpoint. It implements `apperror.Kinder` by
  mapping the response status back to a kind (exported as
  `client.KindForStatus`) and `endpoint.RetryAfterReporter` by parsing the
  `Retry-After` response header (delay-seconds and HTTP-date). `WithRetry` on a
  client endpoint now works without a custom classifier, and a relayed upstream
  503 keeps its status instead of degrading to 500.
- `grpc.StatusError`, `grpc.ClassifyError`, and `grpc.KindNameForCode` give the
  gRPC client the same treatment: the client wraps a failed `Invoke` so the
  error reports a kind name, `Retryable`, and the `google.rpc.RetryInfo` delay,
  while `status.FromError` and `status.Code` keep working.
- `grpcserver.NewErrorEncoder` with `WithKindMapper` and `WithErrorDomain`
  replaces the growing set of encoder constructors with one options-based
  factory. The stable application code now travels in a `google.rpc.ErrorInfo`
  detail (AIP-193) alongside the existing `RetryInfo`, so a gRPC hop no longer
  drops the code the way the HTTP body always carried it.
  `ErrorEncoderWithKindMapper` stays as shorthand.
- `grpc.StatusError.ErrorCode` and `ErrorDomain` read that `ErrorInfo` back, and
  `client.HTTPStatusError.ErrorCode` parses the JSON body's `code`. Both satisfy
  `transport/http.ErrorCoder`, so relaying an upstream error unchanged reproduces
  the upstream code instead of a status-derived name. The upstream message is
  deliberately not relayed as a public message.
- `endpoint.RetryAfterReporter`, `endpoint.RetryAfterError`, and
  `endpoint.NewRetryAfterError`: a transport-neutral retry hint. HTTP encoders
  emit `Retry-After`; the gRPC encoder attaches `google.rpc.RetryInfo`. An open
  circuit breaker reports the time left in its open window, and a `RateLimiter`
  that implements the interface has its delay attached to `ErrRateLimited`.
- `apperror.KindCanceled` (HTTP 499 / gRPC Canceled) and
  `apperror.KindUnimplemented` (HTTP 501 / gRPC Unimplemented), with matching
  constructors. `server.StatusClientClosedRequest` names the non-standard 499.
- Builder shortcuts for the rest of the catalog: `WithRateLimit`,
  `WithDelayRateLimit`, `WithCircuitBreaker`, `WithRecording`, `WithRetry`.

### Changed

- **Breaking:** the endpoint rejection errors now classify themselves through
  `apperror`, and the HTTP encoders no longer special-case them.
  `ErrRateLimited` stays 429 (`KindResourceExhausted`), but `ErrCircuitOpen`,
  `ErrBulkheadFull`, and `ErrBackpressure` became 503 (`KindUnavailable`)
  instead of 429: load shedding is a server condition, not a caller quota
  problem, which is how Envoy and Resilience4j report it. The gRPC adapter
  previously mapped all four to `Internal` and now agrees with HTTP. Clients
  that treated a tripped breaker as 429 must accept 503.
- **Breaking:** `endpoint.RateLimiterFunc` is now `RateLimiterFuncs`. It is a
  struct of two functions, not a function type, so the plural name matches what
  it is and the `XxxFunc` convention keeps meaning "function type".
- **Breaking:** a `client.HTTPStatusError` returned unchanged from a handler now
  encodes as the upstream status (a dependency's 404 answers 404) instead of
  500, because the error classifies itself. Translate upstream failures
  deliberately —
  `apperror.WrapCause(apperror.KindUnavailable, "upstream.users", err)` — when
  they mean something else to your own callers.
- The HTTP error encoders resolve the error kind through `apperror.KindNamer`
  when `apperror.Kinder` is absent, so a module that classifies structurally to
  avoid importing `apperror` — the gRPC integration, for one — maps to the right
  status instead of 500. The gRPC encoder already worked this way.
- HTTP error encoders map an unclassified `context.Canceled` to 499 instead of
  500, so a client disconnect stays out of the 5xx rate. The gRPC adapter
  already mapped it to `Canceled`.
- `kit.WithMetrics` installs the collector as a route-labeled recorder placed
  outermost in the chain rather than as a middleware in option order, so it now
  also counts requests that a rate limit or a breaker rejects.
- `DecodeRequestFunc`, `EncodeResponseFunc`, `EncodeRequestFunc`,
  `DecodeResponseFunc`, `ServeGRPC`, and `Interceptor` spell their dynamic
  parameters `any` instead of `interface{}`. The types are identical; no call
  site changes.
- HTTP error encoders map an unclassified `context.DeadlineExceeded` to 504
  instead of 500, so an endpoint timeout from `TimeoutMiddleware` agrees with
  `apperror.KindDeadlineExceeded` and with the gRPC adapter, which already
  mapped context deadlines to `DeadlineExceeded`. Explicit classification
  still wins: an `apperror` wrapping a deadline keeps its own kind. Clients
  that treated an endpoint timeout as 500 must accept 504.
- `Fallback` no longer runs the fallback endpoint when the caller's context is
  already cancelled or expired, and joins the primary and fallback errors when
  the fallback fails too. Previously the primary cause was discarded.
- Middleware constructors that require a dependency — `MetricsMiddleware`,
  `RateLimitMiddleware`, `DelayRateLimitMiddleware`, `Fallback`,
  `InFlightMiddleware` — now panic on nil at assembly time instead of on the
  first request, matching `NewBuilder` and `Chain`.
- `NewCircuitBreaker` normalizes non-positive settings to the exported
  defaults, so `WithBreakerFailureThreshold(0)` selects 5 as documented rather
  than tripping on the first failure.
- The complete kind-to-status table now lives only in the error-handling
  chapter (`docs/errors.md`), which gained the gRPC column and the
  non-classified mappings (rejection errors, context deadlines); core concepts
  links to it instead of repeating a partial copy.
- `make verify` gained a `test-fmt` gate that fails when any Go file is not
  gofmt-formatted, and the repository is now fully formatted.

### Fixed

- `WithBreakerSuccessThreshold` had no effect: the half-open state closed the
  breaker on the first successful probe regardless of the configured
  threshold. The breaker now counts consecutive probe successes and resets the
  count when a probe fails or the breaker reopens.
- `make test-runtime` referenced the removed `./log` package and the removed
  `integrations/circuitbreaker` and `integrations/ratelimit` modules, which
  broke `make test` and `make verify`.

### Removed

- **Breaking:** the exported counter fields on `endpoint.Metrics`
  (`RequestCount`, `ErrorCount`, `SuccessCount`, `TotalDuration`,
  `LastRequestTime`). They allowed unsynchronized reads of state guarded by an
  internal mutex. Read metrics through `Snapshot()`, whose
  `MetricsSnapshot` keeps the same field names. See MIGRATION.md.
- **Breaking:** `endpoint.TypeAssertError.Want` is a `reflect.Type` instead of
  a zero value, so interface and pointer targets report a usable type name.
  Construct it with the new `endpoint.NewTypeAssertError[T](got)`.
- `endpoint/metrics_prometheus.go`, a `//go:build ignore` file whose body was
  entirely commented out. Use `observability/otel` for metric export.

### Added

- `apperror` convenience constructors for the remaining kinds: `Internal`,
  `AlreadyExists`, `FailedPrecondition`, `ResourceExhausted`, and
  `DeadlineExceeded`. Every kind now has a same-named constructor.
- `endpoint.DefaultBreakerFailureThreshold`, `DefaultBreakerSuccessThreshold`,
  and `DefaultBreakerOpenTimeout` expose the circuit breaker defaults.
- `tools/documentation_api_test.go` verifies that every framework symbol the
  documentation names is still declared by its package, catching removed-API
  references the curated blacklist misses.

## [2.6.0] - 2026-08-27

### Added

- `security` root package: transport-neutral subject contracts for
  authentication boundaries — `Subject`, `Authenticator`, `WithSubject` /
  `SubjectFromContext`, and endpoint middleware `Middleware`,
  `RequireAuthenticated` (401), `RequireRole` (403), all classified through
  apperror so every transport maps them uniformly.
- `kit.NamedLifecycle` and `kit.ReadinessProvider`: lifecycle components can
  name themselves for startup/failure/shutdown diagnostics and bridge
  asynchronous warm-up into the `/readyz` and `/health` readiness checks.
- `apperror` convenience constructors: `InvalidArgument`, `Unauthenticated`,
  `PermissionDenied`, `NotFound`, `Conflict`, `Unavailable`, and `WrapCause`
  for cause-preserving errors without a public message.
- `endpoint.ValidationError` implements `apperror.Kinder` and
  `apperror.KindNamer`, so transports map validation failures through the
  standard apperror path.
- `sd/client.NewEndpoint` accepts a nil logger and falls back to
  `slog.Default()`.
- `slogadapter.NewTelemetry` assembles the built-in observability dimensions
  into one middleware chain with the canonical order (tracing → metrics →
  logging) and feeds one `endpoint.Metrics` collector; `Telemetry.Apply`
  installs the chain on a Builder with stable labels.
- Server-Sent Events moved to the transport layer: `server.NewSSEServer` and
  `server.NewSSEServerTyped` adapt a streaming handler to `http.Handler`
  with the standard Server hooks (ServerBefore, decode-before-headers,
  ServerErrorHandler, ServerFinalizer); `kit.HandleSSETyped` registers typed
  streams through the endpoint middleware chain, so authentication, tracing,
  metrics, and validation now apply to streams. One stream counts as one
  request; decode failures map to regular error responses.
- ARCHITECTURE documents the module dependency layers (L0–L5) and context
  key conventions.

### Fixed

- `kit` lifecycle watchers now consume asynchronous component errors until
  shutdown; previously only the first error per component was reported.

### Removed (breaking)

- `server.HTTPError`, `server.NewHTTPError`, and `server.WrapHTTPError`.
  Carrying an HTTP status out of the endpoint or service layer crossed the
  layer boundary. Classify failures with `apperror` (transport-neutral, works
  over gRPC too); protocol-specific customization remains available through
  `transporthttp.StatusCoder`, `ErrorCoder`, `PublicMessager`, and `Headerer`.
- The deprecated `log` compatibility facade. Use `log/slog` directly.
- `kit.SSEWriter` and the stream-function signature of `kit.HandleSSE`.
  Streaming now lives in the transport layer: use `server.SSEStream` (same
  method set) with `kit.HandleSSETyped` (endpoint middleware applies) or
  the `HTTP.HandleSSE` method for raw streaming handlers.
- `kit.Service`, `kit.New`, `kit.MustNew`, and `Service.Run` (breaking).
  Assembly is now split into a transport-neutral `kit.Host` orchestrating
  `kit.Lifecycle` components and a `kit.HTTP` component owning routes,
  health checks, and the HTTP server. Use `kit.NewHTTP` / `kit.MustNewHTTP`
  for the component and `kit.NewHost(kit.WithLifecycle(...))` + `Host.Run`
  for the process. `HandleJSON*` and `HandleSSETyped` take `*kit.HTTP`;
  `Handle`, `HandleFunc`, and `HandleSSE` are methods on `*kit.HTTP`. Pure
  worker or gRPC-only services can now run through a Host without HTTP.

### Changed

- Documentation now references the core `endpoint` circuit breaker and rate
  limiter instead of the removed `integrations/circuitbreaker` and
  `integrations/ratelimit` adapters.
- microgen (breaking for generated projects): the user-owned
  `cmd/custom_routes.go` hook is standard-library only now —
  `func registerCustomRoutes(r *http.ServeMux)` registers routes directly and
  `customRouteDescriptions() []string` ("METHOD /path" entries) feeds
  `/debug/routes` and the startup route listing, replacing the old return of
  generator-internal route entries. Generated projects write manifest schema
  `microgen.project.v3`; extend rejects pre-v3 projects with an actionable
  migration hint.

## [2.5.2] - 2026-08-22

### Added

- Custom body formats over HTTP: `server.RawBodyCodec` and
  `server.RawBodyCodecWithMaxBytes` turn two pure functions into the
  transport codec (bounded body, preserved StatusCoder/Headerer), and
  `server.TextErrorEncoder` keeps error responses in the route's format
  instead of defaulting to JSON. apperror is optional on such routes; the
  framework forces no error model.
- `examples/customcodec`: a runnable custom-format service (length-prefixed
  binary body over HTTP, format-matched error responses).

## [2.5.1] - 2026-08-22

### Added

- Dual-protocol binding: `transport.Binding[Req, Resp]` carries one
  middleware-built endpoint and serves HTTP and gRPC without duplicated
  assembly. `Binding.TypedEndpoint()` feeds the typed JSON servers directly;
  `grpcserver.NewServer` with the two protobuf mapping functions covers the
  gRPC side. The same middleware chain runs on both protocols.

## [2.5.0] - 2026-08-22

### Added

- gRPC custom error-kind mapping: `grpcserver.ErrorEncoderWithKindMapper`
  resolves the gRPC code through an application mapper first and falls back
  to the built-in mapping; `grpcserver.CodeForErrorKind` exposes the built-in
  mapping for composition.
- MCP interaction error mapping: `mcp.RPCCodeForInteractionError` and
  `mcp.ErrorMapperForInteraction` map the interaction sentinel errors to
  JSON-RPC codes, so custom mappers build on a documented contract instead of
  reverse-engineering the handler.
- Response assembly combinator: `server.WrapJSONResponse` wraps the response
  value (envelope, post-processing) while preserving the original response's
  StatusCoder and Headerer behavior.
- Middleware chain introspection: `endpoint.Builder.UseNamed` labels
  middleware and `Builder.Describe` returns the chain in application order;
  the built-in `With*` shortcuts record labels automatically. Startup logs
  can now print the assembled chain.
- Startup-time request validation: `transporthttp.ValidateQueryStruct[T]`
  checks query/path request struct tags and supported field types at
  assembly, surfacing unsupported types at startup instead of the first
  request.

## [2.4.4] - 2026-08-22

### Added

- Custom error-kind status mapping: `server.JSONErrorEncoderWithKindMapper`
  resolves the HTTP status through an application mapper first and falls back
  to the built-in mapping for unknown kinds;
  `server.HTTPStatusForErrorKind` exposes the built-in mapping for
  composition. Applications can now define their own `apperror.Kind` values
  with custom statuses without replacing the whole error encoder.
- `client.DecodeJSONResponse` and `client.DecodeJSONResponseWithMaxBodyBytes`
  export the default JSON response decoder, so a custom client composed with
  `NewExplicitClient` reuses the same status handling and body limit.

## [2.4.3] - 2026-08-22

## [2.4.2] - 2026-08-23

## [2.4.1] - 2026-08-22

## [2.4.0] - 2026-08-21

### Added

- Transport-level response assembly: `server.ServerResponseEncoder` overrides
  the success encoder for the JSON entry points, and
  `kit.WithJSONServerOptions` applies server options (envelope, error format,
  hooks) to every JSON route in one place. Per-route options take precedence.
- `examples/envelope`: response assembly at the transport boundary - business
  handlers stay envelope-free while `{code, message, data}` and its matching
  error format are defined once at assembly.
- Documentation for composition and nesting: the transport guide explains
  accumulating versus replacing components and how to combine body, path,
  query, and multipart parsers; the endpoint guide documents the four
  middleware flow-control patterns (short-circuit, branch, repeat, replace).

## [2.3.0] - 2026-08-20

### Added

- W3C Trace Context propagation in the core packages: `endpoint.TraceContext`
  with `ParseTraceparent`, and `transport/http` `ExtractTraceparent` /
  `InjectTraceparent` RequestFuncs for servers and clients.
  `endpoint.TracingMiddleware` now joins an incoming trace context under the
  same trace ID and mints W3C-conformant 32-hex-character trace IDs otherwise.
- `kit.HandleSSE` and `kit.SSEWriter` for Server-Sent Events streams with
  per-event flushing and client-disconnect cancellation; named, JSON, and
  multi-line data events, comment heartbeats, and retry hints.
- `server.ParseMultipartForm` for bounded multipart/form-data uploads
  (total-body, per-file, and in-memory caps; 413/415/400 classification) and
  `server.WriteAttachment` for sanitized file downloads.
- Request validation convention: `endpoint.Validatable`,
  `endpoint.ValidationMiddleware`, and `endpoint.ValidationError` with
  field-level failures; the HTTP error encoder maps them to 400 with the
  stable code `bad_request.validation`.
- Pagination convention in `transport/http`: `ParsePage` (defaults 1/20,
  size capped at 100, invalid query values rejected as validation errors),
  `Page.Limit/Offset`, and the generic `PageResult[T]` wire shape.
- `examples/auth`: application-owned authentication and authorization
  middleware with Bearer API keys, 401/403 responses classified by
  `apperror`, and public health routes.
- `examples/todosvc`: an end-to-end SQLite CRUD service with a CGO-free
  repository, `apperror` classification, path-parameter routes, and
  database closure during graceful shutdown.
- Resilience middleware: `endpoint.Fallback` answers with a fallback
  endpoint when the primary fails, and `endpoint.BulkheadMiddleware`
  isolates concurrency per resource key; `ErrBackpressure` and the new
  `ErrBulkheadFull` now encode as HTTP 429 instead of 500.
- PRODUCTION.md gains Deployment (static containers, probe wiring,
  termination-budget alignment, config injection), Alerting (starter
  alert set mapped to the documented metric signals), and Background
  Jobs (directory structure and `kit.Lifecycle` wiring) sections.

### Changed

- Trace IDs minted by `endpoint.TracingMiddleware` are 32 lowercase hex
  characters (W3C format) instead of 16. Existing callers that treat the ID
  as an opaque string are unaffected.

### Fixed

- `microgen` no longer registers the `-add-tables` flag: it was parsed but
  never consumed, misleading users into thinking extend mode could append
  tables. MICROGEN.md now states the covered generator version and lists the
  source-mode and `-service` options with defaults.

## [2.2.0] - 2026-08-14

This development cycle contains one explicitly approved, narrowly scoped v2
SemVer exception: `Metrics.Snapshot()` now returns `MetricsSnapshot` instead of
`Metrics`. The migration is documented in [MIGRATION.md](MIGRATION.md).

### Added

- Transport-neutral `apperror` classification with consistent HTTP and gRPC
  mappings, including a default gRPC error encoder.
- Fully typed JSON assembly helpers in `kit` and `transport/http/server` for
  concrete request and response types.
- A lock-free `endpoint.MetricsSnapshot` value with `AverageDuration()`.
- A bounded request-ID validator option and conservative validation for
  caller-supplied request IDs.
- Application-owned generated configuration extensions in `config/custom.go`,
  including defaults, environment, and validation hooks preserved on rerun.

### Changed

- **Approved SemVer exception:** `Metrics.Snapshot()` returns
  `MetricsSnapshot`. Field access through an inferred local variable is
  unchanged; code explicitly declaring the result as `Metrics` must update its
  type.
- `microgen` uses a pure-Go SQLite introspection driver, so installing and
  running the CLI no longer requires CGO or GCC.
- Generated demo clients delegate to the generated Go SDK instead of maintaining
  a second HTTP/gRPC implementation.
- Health checks execute concurrently with per-check timeouts and allow at most
  one active invocation of each named check.
- Quickstart and composition examples use typed handlers and propagate listener
  and shutdown errors through the process lifecycle.
- The `main` branch now contains only the maintained v2 product line; legacy v1
  source and documentation remain available through immutable release tags.
- The repository-root README is now a concise v2 entry point instead of a
  duplicate v1 usage guide.

### Fixed

- Default HTTP and gRPC error encoders no longer expose unclassified internal
  error messages to clients, while classified application errors retain stable
  public codes and messages.
- Concurrent health probes no longer accumulate serial latency or launch
  overlapping timed-out checks.
- Example metrics reads use atomic snapshots, and snapshots no longer carry an
  internal mutex that triggers `go vet` copylock failures.
- Public API snapshot collection ignores tool download diagnostics written to
  stderr.

## [2.1.0] - 2026-08-14

This release is the explicitly approved direct-refactor v2 SemVer exception. It is
not source compatible with `v2.0.0`; follow [MIGRATION.md](MIGRATION.md) before
upgrading.

### Added

- Executable architecture dependency gates and a reviewed dependency closure
  comparison against the published `v2.0.0` baseline.
- A provider-neutral `kit.Lifecycle` contract and optional `kit/grpc`
  component for multi-server applications.
- Manifest-driven verification for the core and all independently versioned
  modules through `make verify-published-core` and `make verify-published`.
- A 4 MiB default success-response limit for `transport/http/client.NewJSONClient`,
  with an explicit constructor for larger contracts.
- Transport-owned interaction session release through `Runtime.ReleaseSession`.

### Changed

- The repository owner approved publishing the direct incompatible refactor
  under `/v2` as a documented direct-refactor SemVer exception; `v2.0.0` remains
  immutable and the published root refactor release is `v2.1.0`.
- The root runtime module now has no third-party requirements. gRPC, Consul,
  Gobreaker, rate limiting, Zap, and OpenTelemetry live in independent modules.
- `microgen` implementation packages are internal, generated code uses direct
  `slog`, and minimal HTTP projects omit optional provider and database modules.
- The recommended example is now `examples/quickstart` using `kit`; explicit
  endpoint/transport wiring lives in `examples/manual_composition`.
- Core `kit` is an HTTP-only assembly layer. Endpoint middleware with external
  dependencies is installed explicitly through `kit.WithEndpointMiddleware`.
- HTTP and gRPC transports default to a no-op error handler; error reporting is
  application-owned through the `observability/slog` or `integrations/zap`
  adapters.
- `microgen` now defaults config, model/repository, and database runtime wiring
  to off. `-from-db` still always emits the introspected models.
- Full regeneration preserves user-owned service implementations, `cmd/main.go`,
  config YAML, and project README; endpoint and transport artifacts are tracked
  as generator-owned in the manifest.
- Generated debug route registration and route printing are opt-in, rate limiting
  is disabled by default, middleware timeout comes from config, and inbound retry
  configuration has been removed.
- Generated HTTP and gRPC listeners bind before serving; generated database
  handles are checked and closed during shutdown.
- MCP tool calls reuse the runtime session bound to the MCP transport session,
  which is released on DELETE or TTL expiry.
- Go IDL generation fails on an invalid interface method instead of silently
  omitting the method.

### Fixed

- Consul remote config loading now honors its timeout and response size limit
  without pulling a second Viper-based provider stack into generated projects.
- The documented v2 release tag is the root `v2.0.0` tag required by Go module
  resolution; the historical incorrect `v2/v2.0.0` tag was removed.

### Removed

- Old provider-owned package paths under `endpoint`, `transport/grpc`, and
  `sd/consul`; use the corresponding independent integration modules.
- `kit.WithGRPC`, `Service.GRPCServer`, `kit.WithRateLimit`,
  `kit.WithCircuitBreaker`, `kit.WithLogging`, and the root transport
  `NewLogErrorHandler` convenience API.
- Dead combined config template and obsolete Swagger 2.0 Make targets/tooling.

## [2.0.0] - 2026-07-20

First stable v2 release. Exported runtime APIs, the `microgen` CLI and
configuration, generated ownership, and documented protocol behavior now follow
the compatibility policy in [RELEASE.md](internal/docs/RELEASE.md).

### Added

- Independent `github.com/dreamsxin/go-kit/v2` module.
- Error-returning `kit.New`, context-driven `Service.Run`, configurable graceful
  shutdown timeout, and `kit.MustNew` for explicit panic-on-invalid setup.
- Final generated configuration validation for server, logging, database,
  middleware, and remote-provider settings.
- Deterministic formatting and text normalization for generated output.
- Repository-wide UTF-8 validation that rejects BOMs, invalid byte sequences,
  and Unicode replacement characters in maintained text files.
- External generated-project smoke coverage using `go mod tidy` and
  `go test ./...`.
- Shared HTTP path/query codec for generated transports, clients, and SDKs.
- OpenAPI 3.1 generation and a standalone JSON Schema 2020-12 bundle directly
  from the common `microgen` IR.
- Zero-runtime-dependency TypeScript Fetch clients generated from the same IR,
  with strict compiler settings and external type-check coverage.
- Shared non-GET path parameter encoding and decoding for generated transports,
  clients, and SDKs.
- Versioned `.microgen/manifest.json` project identity with source, capability,
  route, service, model, middleware, and generator-owned artifact metadata.
- OpenAPI 3.1 parser validation and JSON Schema 2020-12 compilation for Go IDL,
  Protobuf, and database generation integration paths.
- A release contract check that type-checks generated SDKs with a pinned
  TypeScript compiler.
- Shared executable HTTP behavior coverage for generated Go and TypeScript SDKs,
  including path, query, body, headers, and non-2xx errors.
- Reviewed deterministic contract snapshots for Go IDL, Protobuf, and database
  generation paths.
- Optional `observability/slog` endpoint logging and independent
  `observability/otel` tracing/metrics adapters with application-owned provider
  setup.
- Optional standard-library `security/http` middleware for trusted proxy and
  client IP resolution, IP policy, CORS, signed double-submit CSRF, and security
  response headers.
- `kit.WithHTTPMiddleware` for whole-server standard-library middleware across
  health, endpoint, raw HTTP, and generated routes.
- Release gates for reviewed public API drift, maintained Markdown links,
  module tidy state, focused race tests, vet, and committed v2-scope cleanliness.

### Changed

- `kit` no longer installs process signal handlers or calls fatal logging during
  service lifecycle.
- `Service.GRPCServer` returns an error when gRPC is not configured.
- Generated config precedence is local YAML, optional remote config, final
  environment overrides, then validation.
- Service-discovery registration returns its initial snapshot synchronously and
  publishes later updates without closing consumer channels.
- In-memory interaction providers copy mutable resources, blobs, templates,
  prompts, and render arguments.
- Generated HTTP servers use the standard library `http.ServeMux`; generated GET
  clients and servers share one tagged query contract and do not send JSON bodies.
- Generated Go clients and SDKs use the same complete HTTP paths as server route
  registration and OpenAPI output.
- Generated OpenAPI projects embed Swagger UI 5 assets and serve both
  `/openapi.json` and `/schema.json` without CDN dependencies.
- HTTP JSON client timeout construction is explicit through
  `NewJSONClientWithTimeout`.
- Service-discovery retry defaults to one attempt and only retries explicitly
  classified transient errors when additional attempts are configured.
- Service-discovery endpoint constructors return an owned closer and validate
  required dependencies and timing options before starting background work.
- v2 documentation is task-oriented and no longer duplicates v1 release history,
  temporary roadmaps, or session snapshots.
- Extend scanning uses the project manifest as its primary capability source and
  reports filesystem or ownership drift before mutation.
- Generated Go SDKs expose `APIError` with stable status-code and response-body
  fields, aligned with the TypeScript SDK error contract.
- MCP Streamable HTTP now enforces the initialization lifecycle, protocol
  version, browser Origin policy, and client sampling capability. Logging levels
  are session-scoped, and server messages use one active SSE stream.
- `kit` and generated HTTP servers use streaming-safe defaults with bounded
  header reads and no default response write deadline.
- Consul registrar operations return errors; instancer shutdown cancels and
  joins the active blocking query.
- Generated Go SDKs resolve URLs structurally and bound response bodies.
- Generated repositories accept only model-derived ordering fields.

### Fixed

- Prompt render callbacks no longer run while the provider lock is held.
- Consul retry waits respond to shutdown and repeated `Stop` calls are safe.
- Endpointer shutdown no longer races with producer sends on a closed channel.
- Endpointer shutdown waits for its update loop and releases every client
  resource still owned by the endpoint cache.
- Endpoint caches no longer sort caller slices in place or expose their internal
  endpoint slice to callers.
- Generated environment values remain the highest-priority config source after
  remote loading.
- Generated Go files fail generation before a malformed partial file is written.
- Append-service and append-model refresh OpenAPI, JSON Schema, and TypeScript
  client artifacts instead of leaving generated contracts stale.
- Service, model, and middleware append operations refresh the project manifest
  last and reject projects with unresolved manifest drift.
- Database-derived contract IR now matches generated Go IDL for optional create
  fields, update fields, list query parameters, and response JSON shapes.
- HTTP response-writer interception preserves optional streaming interfaces,
  ignores repeated status writes, and accounts for `io.ReaderFrom` bytes.
- Buffered HTTP client decode failures close the response body and cancel the
  request context.
- gRPC clients preserve caller-provided outgoing metadata while applying hooks.
- MCP tool-triggered sampling now uses the transport session from request
  context, and tool execution failures return `isError: true` results.

### Removed

- v1 compatibility claims and v1.0/v1.6 release planning from v2 documentation.
- Duplicate architecture, generator design, project snapshot, roadmap, stability,
  observability, security, and maintainer documents.
- Duplicate HandyBreaker and built-in Hystrix implementations; Gobreaker is the
  single circuit-breaker adapter in core.
- Redundant `sd.NewEndpointCloser`; lifecycle ownership is part of every
  `sd.NewEndpoint` construction.
- Swagger 2.0 annotation output, `swagger_host`, and `APP_SWAGGER_HOST`; Swagger
  UI now reads the generated `/openapi.json` contract.
- The non-standard `microgen -skill` option, generated `skill/` package,
  `/skill` discovery endpoint, repository AI `SKILL.md`, and dedicated skill
  example. OpenAPI/JSON Schema remain the general contract formats, while the
  optional interaction runtime exposes tool discovery and execution through MCP.
