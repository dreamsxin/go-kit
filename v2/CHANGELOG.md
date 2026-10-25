# Changelog

English | [简体中文](CHANGELOG_zh.md)

## [2.28.0] - 2026-10-25

### Added

- `grpcclient.NewTypedClient[Req, Resp]` carries a unary RPC's proto types in
  the Go type system: `Invoke` takes the request message and returns a freshly
  allocated response message — no `endpoint.Endpoint`'s `any`, no
  `endpoint.Unwrap` at the call site (`grpc.typed-client`). It embeds the
  untyped client, so the option surface and the default traceparent
  injection are identical, and transport failures classify the same way.
  Generated clients adopt it in this release: the gRPC SDK's unary methods
  call `Invoke` on typed fields and no longer wrap through
  `endpoint.Unwrap`, and the per-method exported encode/decode pair the
  untyped client needed is gone from the generated transport (the server
  keeps its own). The contract snapshots record the change.
- `examples/tracingtree` runs one request through the full stack — SERVER
  span, endpoint span, CLIENT span — and prints the tree it exported, using
  a hand-written in-memory exporter that makes the application-owned
  provider story visible.
- The gRPC transport gets the same observability pair as HTTP:
  `oteladapter.GRPCServerSpanInterceptor` starts one SERVER span per unary
  RPC named `package.Service/Method` with `rpc.system.name`, `rpc.service`,
  `rpc.method` and `rpc.grpc.status_code`, Error on any non-OK code
  (`otel.grpc-server-span`); `GRPCClientSpanInterceptor` is the CLIENT side
  under the same name (`otel.grpc-client-span`); and `NewGRPCMetrics`
  records `rpc.server.call.duration` for the transport's recording
  interceptors, agreeing with the built-in exposition by test
  (`otel.grpc-metrics-agree-with-the-recorder`) — the gRPC transport can
  now push through OTel instead of only being scraped.
- The OpenTelemetry adapter starts spans at the transport boundary:
  `oteladapter.HTTPServerMiddleware` wraps a whole server and starts one
  SERVER span per request — named `GET /route` when the mux matched, carrying
  `http.request.method`, `url.scheme`, `url.path`,
  `http.response.status_code`, with only 5xx marked Error
  (`otel.http-server-span`) — and `oteladapter.NewRoundTripper` wraps any
  `http.RoundTripper` to start one CLIENT span per outbound call with
  `server.address`, `server.port` and `url.full`, inject the trace context
  through the global propagator, and mark transport errors and 4xx/5xx as
  Error (`otel.http-client-span`). Together with the endpoint span a request
  is now a three-level tree; database and retry-attempt spans remain
  roadmap work.

- `validation.Defaults(&req)` and `validation.DefaultsMiddleware()` honor the
  contract's `default=` keyword on the server: a field whose value is the
  zero value is filled with the tagged default (strings verbatim, numbers and
  bools parsed, a nil pointer allocated) before the field's own rules run
  (`validation.default-fill`, `validation.defaults-middleware`). A generated
  transport fills in its decoder, before checking, so the promise the
  OpenAPI document makes to a form is kept by the service too. Zero value
  and absent value are the same thing — JSON cannot send an unset
  non-pointer field — and `Validate` itself never fills.

### Removed

- `transport/http.PopulateRequestContext` and its thirteen request-header
  `ContextKey*` constants are gone. The function copied request headers —
  the Authorization header included — into the context for any layer to
  read, against the architecture's rule that protocol objects stay in the
  transport layer, and nothing in the repository called it. A `RequestFunc`
  that needs a header reads the `*http.Request` it is handed; the response
  keys (`ContextKeyResponseHeaders`, `ContextKeyResponseSize`) stay.

### Changed

- `grpc.StatusError` is now `grpc.GRPCStatusError`, naming the protocol the
  way its HTTP counterpart `client.HTTPStatusError` does. A deprecated type
  alias keeps the old name compiling; it goes away no earlier than the
  compatibility freeze.

## [2.27.0] - 2026-10-24

### Added

- `resource` is the query vocabulary a resource route speaks. A filter is a
  serializable tree (`resource.Filter`), not a string handed to a database
  driver, because a string cannot be narrowed by a permission policy, compared in
  an audit log, or replayed from a bug report. `ParseFilter` reads the dialect
  clients send — `{"age":{"$gte":18,"$lt":65}}`, `$and`/`$or`/`$not`, `$in`,
  `$dateRange`, `$empty` — visiting keys in sorted order, so one document always
  produces one tree. `Filter.Validate(schema)` answers an undeclared field, an
  unfilterable one, an operator that means nothing on the field's type, or an
  operand of the wrong type with a `filter.*` code, and never narrows silently.
  The operator set is closed: an application cannot register its own, because an
  operator is a promise every adapter must keep and a policy layer must be able
  to reason about. `And` performs exactly one simplification — dropping zero
  operands — since merging by field or flattening a nested `and` is how a
  constraint disappears; `Or` keeps an empty operand and validation rejects it,
  because `or`'s identity is "match nothing" and dropping one would turn the
  condition into "match everything". Trees are bounded at 32 levels and 256
  nodes, and numbers travel as `json.Number`, so an identifier wider than 2^53
  stays exact.
- `resource` now carries the other half of the resource layer: the request, the
  merge, and the repository contract. `Params` describes one layer's
  contribution — a default, a client, a policy — and `MergeParams` combines them
  per key with one rule each, all of them narrowing: the filter conjuncts, the
  projection intersects, `Appends` unions, `Sort` is overwritten, the window
  takes the smaller size, and `Writable` intersects whitelists while unioning
  blacklists. `nil` and an empty slice are different values: the first is a layer
  with no opinion, the second a layer that allows none, so two disjoint
  whitelists fail closed instead of collapsing into "unrestricted"; two disjoint
  projections are `resource.fields_disjoint` rather than records with no fields.
  `Repository` (`List`, `Get`, `Create`, `Update`, `Destroy`, `Aggregate`) is the
  contract, with `Record`, `Key`, `WriteMask`, `Query`, `Page` and
  `AggregateQuery`; `Page` serializes exactly as `transport/http.PageResult`
  does, and a test in the transport pins the two payloads byte for byte.
  `Matcher` is the executable semantics of a filter, using SQL's three-valued
  logic: a comparison on a missing field is unknown, and `not` does not turn
  unknown into true, so "everything except X" does not return records that have
  no X. `Schema` gained `Readable` and `Project`, so the default projection comes
  from the declarations and a `SELECT *` cannot return a write-only column.
  `resource/memory` is the reference implementation and the semantic oracle for
  every future adapter.
- `resource.Collection` and `resource/rest` turn a declaration into routes. A
  collection gathers a name, a schema, the fields that identify a record and a
  repository, as a value rather than a registry entry; the HTTP binding turns it
  into six routes. A route is a value — pattern, endpoint, decoder — so one table
  mounts on a kit component and on a plain `http.ServeMux` alike:
  `kit.HandleEndpoint` is the new registration path for an endpoint with its own
  decoder, keeping the component's endpoint middleware, recorders and JSON server
  options, while `Route.Handler()` gives a bare handler. Both answer byte for
  byte the same. The table is `GET`/`POST {prefix}/{name}`,
  `GET {prefix}/{name}:aggregate`, and `GET`/`PATCH`/`DELETE {prefix}/{name}/{id}`;
  the aggregate route is a suffix rather than `/aggregate`, because a wildcard
  fills a whole segment and a suffix cannot shadow the record whose identifier is
  `aggregate`. Parameters are `filter` (repeatable, conjunctive), `sort`,
  `fields`, `append`, `page`/`size`, and `function`/`field`/`group_by` for an
  aggregate; a parameter a route does not read is refused with
  `rest.parameter_not_allowed`, never ignored, because a dropped filter answers a
  broader question. Bodies decode with `UseNumber`, so an integer beyond 2^53
  arrives exactly as sent; names and types are checked against the schema and
  then against the write mask, which refuses with 403 instead of dropping a
  field. A column restriction holds on every answer: a write returns the record
  projected, and an aggregate over a restricted column is
  `rest.aggregate_field_not_allowed`, because summing or grouping a column
  reveals what listing it would.
- `rest.WithPolicy` is how a collection says who may use it, and a collection that
  says nothing is refused. A policy answers with a `rest.Decision` —
  `rest.Allow(params)` or `rest.Deny(reason)` — and the zero `Decision` refuses, so
  an unfinished policy, a `switch` that falls through and a value that was never
  filled in all mean no. Installing a policy closes the surface: an action no
  policy grants is `rest.action_not_allowed`, several policies are several
  authorities that must all allow (a union would let one overrule another's
  refusal), and a policy returning an error stops the request as the error it is
  rather than disguising an outage as a 403. `rest.WithOpenAccess()` is the other
  answer — everybody, everything — and `Routes` refuses a collection with neither
  option (`rest.access_required`) or with both (`rest.access_conflict`), because a
  default that grants everything is a default nobody chose. `rest.WithLayer` is
  gone: a layer could narrow and could not refuse, so the same function now returns
  a decision.
- A policy's condition reaches the record, not only the list. `Get`, `Update` and
  `Destroy` on `resource.Repository` take a `resource.Scope` — the key together
  with the condition the caller was granted — and apply both as one statement,
  because a lookup followed by a check is a window in which the row can change. A
  record the condition excludes answers `resource.record_not_found`, the same
  answer a missing record gets, so a client cannot enumerate the rows it may not
  see; a 403 there would confirm the row exists. A create is checked the other way
  round: the record being stored must satisfy the same condition, and a field it
  does not carry makes the condition unknown, which refuses with
  `rest.constraint_unsatisfied` instead of storing a row that would never be
  readable through the surface that stored it. `resource/memory` and the store
  `microgen -resource` emits both answer a scope with a `WHERE` clause, and the
  generated-project gate drives the typed endpoints and the resource surface
  against one table in one process. A condition that must never move is a field
  the schema marks unwritable: the write mask refuses it on every adapter, which is
  where re-parenting a record out of a scope is stopped. A generated project's
  `cmd/main.go` now states the decision in `resourceAccess()` — open access, with a
  comment saying so — instead of leaving it to an option nobody passed.
- `kit` can be assembled from modules. `kit.WithModules` accepts values
  implementing `kit.Module`: every `Register` runs before any `Init`, and both
  phases run in an order derived from `Requires` and — for modules that require
  nothing — from the module name, so the same set always starts the same way
  whatever order it was declared in. Both phases run with `Run`'s context, so
  initialization is cancellable and a failure leaves no listener open. A nil,
  unnamed, duplicated or unknown module, and a dependency cycle (reported as a
  path, `a -> c -> b -> a`), are rejected by `NewHost`. `Host.Init` is
  idempotent and a failure is terminal. `kit.Registry` belongs to one `Host` and
  dies with it, which is what separates it from the package-level registries
  `ARCHITECTURE.md` rules out.
- `examples/usersvc/userservice.proto` is the protobuf twin of the existing Go
  IDL example, so the documented gRPC workflow has a contract to start from.
- A generated project now has a test that runs it: `TestGeneratedCRUDAgainstSQLite`
  generates a project from a real schema, fills the five stubs microgen leaves
  in the user-owned service file, builds it, starts it as a process, and drives
  create, read, partial update, pagination, delete, and the 404 that follows a
  delete — over HTTP, against SQLite, through the generated repository. Every
  other generated-project test runs with `-db=false` and starts the binary only
  to read `/health`, so this is the first time the generated gorm layer, the
  generated pagination window, and the `errors.Is` branch that turns a unique
  violation into 409 ran together. It found the SQLite introspection gap below
  on its first run.
- `microgen -resource` declares every generated table as a `resource.Collection`
  and mounts its REST surface, so the resource layer is something a generated
  project runs rather than something a package contains. The flag writes three
  files into the generated `repository` package: the shared query translation,
  one declaration per table, and the list the mounting code iterates. The
  translation is the first `resource.Repository` over a real driver. Filters
  become gorm predicates through the clause builders rather than SQL text, so an
  identifier is quoted by the dialector and can only have reached a statement by
  being declared in the schema; `$like` and `$prefix` compile to `LIKE` with an
  explicit `ESCAPE '!'`, since a backslash is also the character MySQL reads
  inside the literal and the statement the other two drivers accept is the one it
  rejects; `$empty` reads as `IS NULL` on a non-text column and
  `IS NULL OR = ''` on text, rather than comparing an integer to the empty
  string; a window's offset is clamped rather than cast, because `int(MaxInt64)`
  is negative on a 32-bit build and a negative offset means "from the top" to
  gorm, which answers the first page to a client that asked for the last; and the
  primary key is appended to every `ORDER BY` as a tiebreaker, so two pages sorted
  by a repeating column can neither overlap nor skip a row. A projection narrows
  the `SELECT` rather than only the response: a column that was never loaded
  cannot be leaked by a later layer. Writes go through the typed model, because a
  map insert cannot report the key the database assigned, and the stored record is
  read back instead of echoed, so a column with a default or a trigger value is
  answered as it was stored. A table whose primary key is composite, or absent, is
  left out of the surface with the reason written into the generated file.
  `TestGeneratedCRUDAgainstSQLite` drives both surfaces over one process and one
  table, and checks that a row deleted through a collection is gone from the
  endpoint that answers the same table.
- Modules can now be read and released. `kit.Host.Registry()` returns the values
  the attached modules published, which is what lets the part of `main` that
  registers routes use a handle a module opened; without it `Provide` was only
  addressable from another module, so publication was a private conversation
  rather than a contract. `kit.ModuleShutdowner` is the other end of `Init`:
  `Host.Shutdown` stops every component first and releases the modules in reverse
  initialization order, because handlers read what the modules published and a pool
  closed before the listener stops fails the requests the drain was still
  finishing. A module that never initialized is not released, a second `Shutdown`
  releases nothing twice, and a `Run` that fails in `Init` releases what did
  initialize instead of leaving it to process exit.
- `kit.HTTP.HandleFunc` takes `func(http.ResponseWriter, *http.Request)` rather
  than `http.HandlerFunc`, which is what makes the component a
  `httpserver.RouteRegistrar` — a named function type and its underlying type are
  different for interface satisfaction. Callers are unaffected, because the two
  types are assignable; the change is that a generated assembly can hand its route
  registrars the component itself.
- Four fuzz targets cover the two parsers that read text a client wrote:
  `FuzzParseFilter` and `FuzzFilterCanonicalForm` in `resource`, and
  `FuzzListQuery` and `FuzzQueryString` in `resource/rest`. Their seeds are the
  documented examples plus a document on either side of each budget, so `go test`
  — and therefore `make verify` — replays the dialect, both limits and the round
  trip through the canonical form on every run, while `make fuzz` is the half that
  lets the fuzzer grow new inputs. The assertions are the contracts and not the
  implementation: a refusal is a classified invalid-argument error with a
  `filter.*` code, an accepted tree is inside the limits the refusals advertise,
  one document parses to one tree, and the canonical form is a fixed point. All
  four survive a rewrite of either parser, which is what makes them a gate rather
  than a description.

### Fixed

- `microgen -from-db -driver sqlite` keeps UNIQUE constraints. SQLite reports
  uniqueness through an index rather than through `PRAGMA table_info`, and the
  introspector only read column metadata, so every UNIQUE column reached the
  generated model without `uniqueIndex`. On a fresh database `-auto-migrate`
  then built a table the source schema never had — duplicate inserts succeeded,
  and the repository's `ErrDuplicated` branch was unreachable outside
  production, where the real schema still enforced it. Reintroducing the
  constraint is what makes the generated CRUD path testable at all, which is
  how it was found.

- The generated `.ai/PROJECT_GUIDE.md` ran the `GET /debug/routes` bullet into
  the preceding `sdk/typescript/client.ts` line whenever both config and
  OpenAPI output were enabled; the two are separate bullets again.
- The generated test template imported the module root as `idl` for every
  source, so a proto-generated project with `-tests` failed to compile
  (`pb` is where its types live); it now selects the same import the other
  templates already did.
- `microgen extend -append-service` works on a project generated with model
  output. It used to copy the new service's scaffold without its
  `generated_repos.go`, so the package referenced `GeneratedRepos` and nothing
  declared it; it also wrote repository wiring for models the combined contract
  declared but the project did not have yet, and left the existing services'
  wiring stale while `cmd/generated_services.go` was regenerated to use the new
  fields (`unknown field ProductRepo`, `undefined: repository.ProductRepository`).
  The plan now carries the appended service's repository wiring, adds the
  model/repository/hooks files the contract declares and the project lacks, and
  refreshes every existing service's wiring in the same step, so appending a
  service before its models still leaves a project that compiles.
- Two generated services put `isScaffoldError` and `expectScaffoldOrOK` into the
  same `test` package, so a project extended beyond one service no longer built;
  the helpers are named per service.

### Changed

- A query string that cannot be read in full is refused with
  `rest.query_malformed` (`rest.query-malformed`) instead of being answered with
  the parameters that happened to parse. `http.Request.URL.Query` is
  `url.ParseQuery` with the error thrown away: it keeps the pairs it could read
  and drops the rest. `?filter=%zz` therefore arrived at the resource surface as a
  request with no filter and was answered with every record, and
  `?filter={"status":"active"};size=10` — a semicolon separator, which Go stopped
  accepting — arrived with neither parameter, so a client that asked for active
  rows ten at a time was answered with all of them twenty at a time. The routes
  check the query string where they are assembled, once, before any decoder reads
  half of it. A client that sends a query the URL parser rejects now gets a 400
  naming the parse failure; one that sends a well-formed query is unaffected.
- A generated project runs on one lifecycle. `cmd/main.go` builds a `kit.HTTP`
  component and a `kitgrpc.Component`, attaches the database as a module that
  implements `Init` and `Shutdown`, and calls `Host.Run`. From there the sequence
  is the framework's: readiness fails before anything is torn down, the configured
  drain delay holds the process open while whatever routes traffic notices, and
  each component gets a share of the shutdown budget. What this replaces was a
  second, hand-written copy of the same sequence — its own `&http.Server{}`, its
  own readiness flag and probe handlers, its own TLS reload duplicating
  `kit.CertificateFiles`, its own gRPC health server and graceful stop — and the
  copy is where the two behaviours were going to drift apart. Generated routes
  register on the component, so per-route metrics come from `kit.WithHTTPRecorder`
  instead of a `DecorateRoutes` wrapper, and `registerCustomRoutes` takes a
  `httpserver.RouteRegistrar` instead of a `*http.ServeMux`, which is what makes a
  custom pattern collide with a generated one at startup rather than be shadowed.
  The probes and the gRPC health service now answer from the framework's probe
  registry, so `/readyz` reports the drain at the moment the process announces it —
  and the body is that registry's (`health.report-shape`: `status`, optional
  `requests`, `checks`) instead of the generated `{"status","service"}` line, so a
  probe configuration that read `service` reads `checks` instead.
- `microgen -protocols http,grpc` now requires a `.proto` IDL. With a Go IDL or
  `-from-db` the command used to write a project that could not compile: the
  generated transport registered through `pb.Register<Service>Server` and the
  Go SDK called `pb.New<Service>Client`, and neither symbol exists because a Go
  struct is not a protobuf message. The CLI now refuses the combination before
  parsing and names the input that works; `make gen-grpc`/`gen-full` build from
  the new `examples/usersvc/userservice.proto` instead, and `-protocols` in
  `-h` says so.
- `microgen -model`/`-db` now requires model types. A contract with no message
  carrying `gorm` tags (a `.proto` contract, or a Go IDL without tags) used to
  write runtime and service wiring that imported `model` and `repository`
  packages with no files, so the project could not compile. The CLI refuses it
  after parsing and names the alternatives: a `gorm`-tagged Go IDL struct,
  `-from-db`, or generating without those flags. Extend refresh is unchanged: a
  contract that drops its last model still warns about the obsolete artifacts
  instead of refusing.
- The generated project README's Project Map now lists every directory the
  project has — `transport/`, `config/`, `model/`+`repository/`, `test/`,
  `docs/`, `sdk/`+`client/`, `.microgen/`, `.ai/` — with an explicit owner for
  each, gated on the features the project was generated with, so a reader can
  learn the module layout from the directory alone. The README is written once
  and is not refreshed for existing projects.
- The release suite asserts exactly what it did and costs about half as much:
  `make verify` fell from 8m21s to roughly 5m on a twelve-core machine. Every
  saving came from removing duplicated work rather than from weakening a check.
  The API-surface snapshot and the compatibility gate each walked forty packages
  with one `go doc` process apiece — 117 seconds for the second one alone; they
  now collect once per checkout, eight processes at a time, and share the read,
  because they ask different questions of the same declarations. The
  component-layering gate asked `go list -deps` once per package to read a
  dependency graph that one `go list -deps -json ./...` already returns. The
  example smoke tests bound fixed ports, which put them in the serial phase
  ahead of every other test; a free port per case lets them overlap. The
  generator's unit tests and the integration subtests declare `t.Parallel` —
  each already owned its own temporary directory and a free port, so the only
  thing serialising them was the absence of the declaration. One step was
  deleted instead of sped up: a generated-proto build check reported failure
  with `t.Logf`, so it cost seconds and could not fail its case.
- The gate suite also stopped paying for the generator forty times over.
  `go run ./cmd/microgen` links a fresh executable on every call — two seconds
  with a warm build cache — and the tools tests invoke the generator forty
  times, once or more per integration case. One build now serves the whole test
  binary; the tree each call reads stays per-call, in `MICROGEN_GO_KIT_ROOT`
  and in the command's working directory, so no case can observe another's
  fixture. The two cases that had been building their own private copy for a
  single extra invocation use the shared one as well.
- The `standalone` release suite compiles the published module under
  `GOWORK=off` instead of re-running its tests. The test suite already ran them;
  what this gate exists to prove is that the module stands without the
  workspace, and asserting the behaviour a second time said nothing about that
  boundary.

- The JSON constructor families were the same call multiplied by the
  differences between callers, so the differences are values now and the names
  are gone. In `transport/http/server`, the nine
  `NewJSONServer`/`NewTypedJSONServer`/`NewJSONEndpoint` constructors (and their
  body-limit, decode-options, and middleware variants) became `NewJSON`,
  `NewTypedJSON`, and `NewJSONEndpoint`, each taking one `JSONDecodeOptions`
  (`DefaultJSONDecodeOptions`, `StrictJSONDecodeOptions(n)`, or your own). In
  `transport/http/client`, `NewJSONClient`,
  `NewJSONClientWithMaxResponseBodyBytes`, `NewJSONClientWithTimeout`,
  `NewTypedJSONClient`, and `NewTypedJSONClientWithMaxResponseBodyBytes` became
  `NewJSON` and `NewTypedJSON`, each taking the successful-response cap
  (`DefaultMaxJSONResponseBytes`); a deadline is `endpoint.TimeoutMiddleware`,
  which is what the removed `WithTimeout` variant wrapped. In `kit`, the eight
  `HandleJSON`/`HandleJSONTyped` registrations became `HandleJSON` — request
  and response types are inferred from the handler, so a concrete response
  needs no type arguments — with per-route differences as `RouteOption`s:
  `RouteBodyLimit`, `RouteMiddleware`, and `RouteServerOptions`. The deprecated
  spellings `kit.JSON`, `kit.JSONTyped`, `kit.NewJSONTypedHandler`, and
  `kit.HTTP.HandleSSE` are removed. The reviewed API snapshot shrank by
  sixty declarations; existing call sites need the new argument or option.
- Resilience left `endpoint` for `endpoint/resilience`. Retry, circuit breaking,
  rate limiting, backpressure, bulkhead and fallback now live one package above
  the vocabulary they use, so `endpoint` keeps `Endpoint`, `Middleware`, `Chain`,
  the clock, metrics, correlation, validation and the error contracts, and an
  application that wants only those does not compile a breaker it never
  configured. The rejection sentinels (`ErrRateLimited`, `ErrBackpressure`,
  `ErrBulkheadFull`, `ErrCircuitOpen`), `RetryAfterError`,
  `RetryAfterReporter` and `KeyedRetryAfterReporter` stay in `endpoint`, because
  transports read them as the contract for status and `Retry-After` headers.
  `Builder` lost its `WithRetry`, `WithCircuitBreaker`, `WithRateLimit`,
  `WithKeyedRateLimit`, `WithDelayRateLimit`, `WithDelayKeyedRateLimit`,
  `WithBackpressure`, `WithBulkhead` and `WithFallback` shorthands; compose the
  same middleware with `Use(resilience.X(...))`, in the same order. The moved
  behaviour markers were renamed into the `resilience.` namespace, and
  `resilience.clock-nil-is-wall-clock` moved with the retry test that proves it.
- The Zap logging middleware now speaks the same record schema as the slog
  adapter: `operation`/`duration`/`success` plus `trace_id`, `span_id` and
  `request_id` when the context carries them (`zap.record-fields`), and a
  failed or panicking call logs at Error instead of Info, so a deployment
  mixing the two adapters can run one query and failures reach error alerts.
  The old `op`/`took` field names are replaced; log queries over them need
  updating.

### Added

- `grpc.TraceparentStreamClientInterceptor` propagates the W3C trace context
  on streaming client calls (`grpc.traceparent-stream-client`), closing the
  asymmetry where only unary calls and the server stream interceptor existed;
  a context without a trace still invents none.
- The reviewed generated contract now includes `generated_chain.go`, so the
  middleware chain every fresh project receives is part of the signed-off
  snapshot rather than invisible to contract gates.
- The protocol-behaviour gate fails when a stable promise is covered only by
  tests that call `t.Skip` unconditionally at function top level. A guarded
  live/integration skip is still allowed, as long as the promise also names
  a test that runs in CI.

### Fixed

- `AccessLogMiddleware` read `trace_id` only from the inbound header, so an
  access line written inside a chain that continued the trace named the
  remote parent rather than this node and never carried `span_id`. It now
  reads `trace_id`/`span_id` from the continued request context first and
  falls back to a parseable inbound header for a raw mount (which keeps no
  `span_id`, the parent must not be misnamed).
- An SSE server constructed without an explicit error handler now logs
  transport errors through slog's default logger, the same default
  `NewServer` already had; it used to fall back to the no-op handler while
  the `transport.default-error-handler` promise said unconfigured servers
  log, so a mid-stream failure on a hand-assembled `SSEServer` left no
  record. The promise test now covers the SSE server too;
  `ServerErrorHandler(transport.NopErrorHandler)` still restores silence.
- Extend mode's generated-middleware detection matched the pre-rename
  `MetricsMiddleware(generatedMetrics(name))` string while the template
  emits `RecordingMiddleware(name, generatedMetricsCollector)`, so every
  current generated project looked metrics-less and a later
  `-append-middleware metrics` duplicated the middleware. Both generations
  are now recognized.
- The flagship examples now model the production checklist: the profilesvc
  request decoders cap bodies with `http.MaxBytesReader`, the best-practice
  server sets read/header/idle timeouts and header size, and the MCP
  examples set the read-side bounds that are safe beside SSE (no write
  timeout that would close a stream).
- A burst of asynchronous component failures could still be truncated by
  `kit.Host.Run`. The retained set recorded every error the watcher read,
  but the watcher also exited on Shutdown's stop announcement, and when the
  announcement and buffered errors were both ready its select could take
  the stop and leave the rest of the burst unread in the component's
  `Errors` channel — those failures were emitted before shutdown began, so
  they belong to the `kit.serve-errors-retained` record. The watcher now
  drains buffered errors non-blockingly when the stop signal fires before
  it leaves. Pinned by `TestRunRetainsAWideBurst` (fifty pre-shutdown
  failures), which loses the tail of the burst on the previous behaviour;
  the existing burst and drain-window tests pass under a 200-run race
  stress.
- Correlation at the framework's own boundaries no longer loses the trace.
  A trace extracted from an inbound `traceparent` was stored in one context
  field while every logger read a second field that only
  `endpoint.TracingMiddleware` synchronized, so a raw handler mount
  (`kit.Handle`, the built-in MCP server, a generated service without the
  tracing middleware) received a valid inbound trace and logged records with
  no `trace_id`. `TraceIDFromContext` now derives from the single W3C
  `TraceContext` (`endpoint.trace-id-derives-from-context`); the new
  `endpoint.WithIncomingTraceContext` marks an extracted context as a remote
  parent, and `endpoint.SpanIDFromContext` reports a span ID only after this
  node continued the trace. `endpoint.ContinueIncomingTrace` exposes the
  identity step the tracing middleware performs for a boundary with no
  endpoint chain.
- Outbound HTTP now propagates the trace by default, matching the gRPC
  client: `transport/http/client` installs the `traceparent` injection hook
  in every constructed client, and the generated standalone HTTP SDK writes
  it on every call (`httpclient.traceparent-by-default`). A context without
  a trace leaves the header unset; `ClientBefore` hooks and request hooks
  still run after the built-in one and can overwrite it.
- The built-in MCP HTTP server (`mcp.NewHTTPServer`, `mcp.Serve`) continues
  the inbound W3C `traceparent` before dispatch
  (`mcp.server-extracts-traceparent`), so a tool call's log joins the
  caller's trace; an absent or malformed header starts a fresh trace. A
  handler mounted by hand on its own mux still owns its extraction.
- Endpoint and tool-call log records now carry `span_id` when this node
  started or continued a span (`slog.record-span-id`,
  `mcp.tool-log-span-id`), distinguishing nested calls that share one trace
  in a flat-log replay without misnaming a remote parent as the local span.
- An asynchronous component failure could be lost in a `kit.Host`. The
  watchers forwarded every component error through a non-blocking send into a
  buffer one wider than the component count, and `Run` consumed a single value
  and then stopped listening — so a burst wider than the buffer, or any
  failure arriving while Run announced the stop and tore the components down,
  vanished silently. That contradicted the architecture's own promise that
  asynchronous component errors are reported until shutdown. Every serve error
  is now retained until Shutdown completes and joined into `Run`'s result
  (`kit.serve-errors-retained`); the `Errors` channel is the wake-up stream a
  caller selects on. The HTTP and gRPC components' serve goroutines now wait
  for the consumer that owns their channel instead of dropping a terminal
  serve failure, and the wait ends when the stop is announced. Pinned by
  `TestRunJoinsEveryAsyncComponentFailure`, which fails on the previous
  behaviour with four of five failures lost.
- An OpenTelemetry span started by `observability/otel`'s tracing middleware
  was always a fresh root. The framework's transports extract the incoming W3C
  `traceparent` into an `endpoint.TraceContext` in the context, which the
  OpenTelemetry tracer cannot see, so every service's spans carried a new
  trace ID and the cross-service tree the header already connected was
  invisible to any tracing backend — the docs claimed interoperability the
  wiring did not have. The middleware now bridges both ways
  (`otel.traceparent-bridged`): a framework-extracted trace context parents
  the span (and its sampled flag decides sampling through the parent-based
  sampler), and a span with no framework correlation publishes its identity as
  the correlation values — so the exported trace, the outgoing `traceparent`
  header, and the logged `trace_id` are one trace across every hop. An
  OpenTelemetry parent already in the context still wins, and an existing
  correlation is never overridden. `PRODUCTION.md`, both otel READMEs, and
  the architecture's lifecycle contract now describe the mechanism that
  actually runs.

- Unique-constraint violations are classified on every generated path. The
  generated main opens gorm with `TranslateError`, so a driver's
  duplicate-key error arrives as `gorm.ErrDuplicatedKey`; the generated
  repository's write paths (Create, Update, Updates) wrap it in the new
  `repository.ErrDuplicated` sentinel, and the CRUD tutorial shows the one
  `errors.Is` branch that maps it to `apperror.KindAlreadyExists` — a 409,
  never an unclassified 500. Found by the replication's full API sweep,
  where an unclassified UNIQUE violation surfaced as 500.

## [2.26.2] - 2026-10-12

### Fixed

- The generated contract's `example` for slice-typed fields was `{}` — an
  object — so the Swagger UI's "Try it out" prefilled a body the service
  rightly refused (`json: cannot unmarshal object into ... []string`).
  `schemaExample` now emits an array prefilled with one element example
  (recursively, so `[][]string` nests), while `map` fields keep the object
  default they genuinely decode from, `[]byte` gives a base64 string, and
  `time.Time` an RFC3339 timestamp. Contract snapshots refreshed and
  reviewed.

## [2.26.1] - 2026-10-12

### Fixed

- The generated startup banner advertised the Swagger UI at
  `/swagger/index.html`, a path the embedded UI does not answer — the page is
  served at the mount path `/swagger/` itself. The banner now names the URL
  that works.

## [2.26.0] - 2026-10-12

### Added

- A method's IDL doc comment can declare its exact HTTP route with a
  `route: <METHOD> <path>` line (`route: POST /user/page`,
  `route: DELETE /user/{id}`). The name-based derivation stays the default;
  the override exists so a generated service can match an existing frontend's
  contract path-for-path. The verb is validated against the ServeMux set, the
  path must start with `/`, and a malformed directive fails generation rather
  than bending a route silently. A `route:` line never reaches the public
  description — the parser strips it the way it strips `auth:`.
- Generated HTTP transports spread a user-owned server-options seam after
  their per-route defaults: `transport/<service>/custom_options.go` (written
  once, never overwritten) returns extra `server.ServerOption` values, and
  because later options win, an application can replace the success encoder
  (a transport envelope such as `{"code":200,"msg":"success","data":...}`),
  the error encoder, or attach finalizers without editing a generator-owned
  file. Extend treats the seam as user-protected. The
  `ServerResponseEncoder` option is documented — and now tested — to
  override the constructor's encode argument of `NewServer`, which is the
  property the seam relies on.

### Changed

- `-prefix` now names the gRPC surface too: the prefix's segments become the
  leading segments of the generated proto package (`-prefix /api/v1` for
  `UserService` declares `package api.v1.userservice;`), so the gRPC full
  method path moves to `/api.v1.userservice.UserService/...` in lockstep with
  the HTTP routes. The generated client and server derive the path from the
  same fields and stay in sync; the pb Go package and file layout are
  unaffected. Prefix segments must be lowercase identifiers — a segment
  protoc would reject is refused at generation time. Existing projects
  regenerating with a prefix move their gRPC wire path; clients generated
  from the refreshed contract follow.

## [2.25.0] - 2026-10-12

### Added

- A method's IDL doc comment can declare its authentication requirement, and
  one `auth:` line reaches three places. `auth: required` and
  `auth: roles=admin,operator` (any-of) are parsed from the Go IDL at
  generation time — an unknown directive or an empty role list is refused,
  never silently ignored. The generated OpenAPI document carries the
  per-operation `security` requirement, the http bearer scheme in
  `components.securitySchemes`, and 401/403 responses with the shared error
  schema, so a frontend generated from the document knows the route needs a
  signed-in caller. The generated endpoints enforce the declaration through
  the new `security.Contract` middleware, backed by the application's
  authenticator hook (`NewAuthenticator` in the user-owned service file,
  returning nil while unimplemented). Fail-closed is the promise: until the
  hook returns an authenticator, every secured method answers 401 with
  `auth.not_configured` — no request ever runs unauthenticated by accident.
  Undeclared methods get no generated enforcement; securing the whole
  surface stays the deployment's decision. Promised as
  `security.contract-middleware`.
- `security.RequireAnyRole(roles...)` — the any-of reading of a role list,
  the shape IDL-declared roles need.
- Both features reach the `.proto` source: protovalidate field options
  (`string.min_len`/`max_len`, numeric `gte`/`lte`, `in` lists, in either the
  aggregate or separate-option spelling) become the same contract
  constraints, and `auth:` directives parse from the RPC's comment — refused
  at generation time when malformed. A multi-line RPC comment is read in
  full. Verified against a generated proto project: the contract carries the
  minimum bound and the security requirement, and the endpoints enforce the
  declared auth.
- `client.HandleSSE[T]` — the client-side mirror of the server's typed SSE
  routes: connects with GET and Accept: text/event-stream, decodes each
  framed event's JSON data into T, consumes comment/id/retry fields, surfaces
  non-2xx answers as the classified status error, and blocks with the same
  shape the server's stream function has. A stream typed string receives the
  raw payload. Promised as `http.sse-client`.

- `client.NewTypedJSONClient[Req, Resp]` is the client-side mirror of the
  server's typed JSON routes, one rule on both sides of the wire: GET and
  HEAD fill the URL's path placeholders and query string from the request
  struct, every other method fills the placeholders and sends the struct as
  the JSON body (the server's path decode wins on overlap, so the client need
  not strip path values), and the call returns Resp without a type assertion.
  Non-struct requests are sent as the body with the URL left as written;
  non-2xx answers surface as the classified `HTTPStatusError`. Promised as
  `http.typed-json-client`.

- MICROGEN now answers the question every generated-project author asks next:
  **where does framework component wiring go?** A new chapter maps the
  intents to their one right place — readiness checks and metrics in
  `cmd/main.go`, the endpoint middleware catalog and transport-neutral
  security in `endpoint/<name>/custom_chain.go`, downstream retry and
  discovery in `service/<name>/service.go`, settings in `config/custom.go` —
  with code that compiles against the framework. The generated AI guide
  carries the same map, the custom-middleware hook documents the catalog it
  composes, and kit's package doc no longer names options that do not exist.

- `microgen extend` without append flags is **refresh**: the project's own
  contract (`idl.go`) is the default `-idl`, the capabilities come from the
  manifest, and every generator-owned artifact is regenerated — the loop for
  a service that grew a method, a type that changed, validate tags that were
  added. User-owned files are never written; the compiler then guides the
  edits inside `service.go` (interface, implementation, logging wrapper).
  Artifacts the new contract no longer produces are warned about and left in
  place, and the contract copy is updated to the contract the refresh ran
  with, so the next extend reads the same loop. `-check` and `-out` default
  to running from the project root. Promised as `extend.refresh`.

- Route groups carry middleware, in both layers the framework distinguishes.
  `group.UseEndpointMiddleware(...)` governs the routes registered through the
  group from here on, running inside the component's chain (and its
  recorders) and outside the route's own middleware — a group-level rejection
  is still observed with the joined pattern. `group.UseHTTPMiddleware(...)`
  wraps the route's handler inside the component's HTTP middleware. Both are
  first-outermost, inherited by nested groups, and govern only the
  registrations that follow the `Use` call — the chain is fixed at
  registration, which is what makes it readable. The semantics follow the
  framework-agnostic convention (Echo's groups) adapted to the two-layer
  model.
- The Routing chapter explains how a gRPC method is addressed (the full
  method path from the proto contract), that the HTTP `-prefix` moves only
  the HTTP surface — the gRPC full method path is unaffected — and how
  hand-written gRPC services register through the kit/grpc component with
  health and metrics wired. The generated-project section and MICROGEN's
  option table state the same scope for `-prefix`.
- A fresh generated project's `go test ./...` is green. The generated tests
  skip every not-yet-implemented method — the skip message names the method
  and the file to implement in, turning the stub list into a checklist — and
  each skip becomes a real assertion the moment its method is implemented. A
  fresh project no longer greets its author with a red suite; a failing
  generated test now means the implementation misbehaves, not that work
  remains.

- An SSE example joins the catalog: `examples/sse` registers a typed stream
  route whose decoder reads the path value, pushes from a channel, and ends
  the stream through `kit.Stopping(ctx)` during shutdown — the last transport
  feature without runnable code beside it.

### Changed

- An error a transport answers is no longer silent. The default error handler
  of both serving transports logs through slog's default logger at Warn level
  (`transport.DefaultErrorHandler`), so a rejected body or a failed decode is
  recorded where the deployment's logs go instead of vanishing behind a
  correct status code. `transport.NopErrorHandler` restores silence on
  purpose, and `observability/slog.NewErrorHandler` stays the rich adapter.
  Promised as `transport.default-error-handler`.

### Changed (documentation)

- The errors chapter names who sees the errors, and the routing chapter
  states the same default beside the route-shaping options.

### Deliberately not done

- A `pattern` rule for the validate tag: Go struct tags cannot carry
  backslashes reliably — reflect's tag conventions consume them as escapes,
  and the patterns a form needs are full of them — so the rule cannot survive
  the journey from source to struct tag without a double-escaping convention
  nobody should have to learn. The validation package documents the finding.

- A `validate` struct tag closes the loop between the contract, the form built
  from it, and the service that answers it. The subset maps one-to-one onto
  JSON Schema keywords — `enum`, `min`/`max` (numeric), `minLen`/`maxLen`
  (string length), and `default` as a form pre-fill hint — and one tag has
  three readers: the generated OpenAPI document and JSON Schema bundle carry
  the keywords, the generated transports validate each decoded request and
  answer a violation with 400 and a code naming the field and the rule
  (`validation.username.minLen`), and a typed kit route enforces the same tags
  through the same package, so a hand-assembled service and a generated one
  reject the same request with the same error. An MCP tool's input schema
  carries the enum of a constrained field, so an AI client sees the options.
  A malformed tag — an unknown rule, a missing value, min greater than max —
  is refused at generation time, never silently ignored at request time.
  Promised as `validation.tag-semantics`.
- Projects generated from a database derive the constraints from the DDL
  itself: a `varchar(n)` column bounds its string, an `enum(...)` column
  lists its values — truth the schema already states, with no tag to write —
  and the written contract copy (`idl.go`) carries the same `validate` tags,
  so the extend loop, the contract, and the service agree. A from-db table
  that already owns its audit columns no longer collides with the injected
  audit fields: the db source now suppresses the injection the same way it
  always suppressed it for hand-annotated models.

### Changed

- The generated interaction file validates each tool call's decoded input and
  carries the constrained field's enum in the tool descriptor, so an MCP
  client cannot offer an option the service would refuse.

- A typed JSON route fills its request struct from the request itself, so path
  and query parameters are part of the typed route instead of a reason to drop
  to a raw handler. GET and HEAD read the query string and the path values;
  every other method reads a strictly decoded JSON body and then the path
  values; a request that carries no body decodes to the zero value, so a
  body-less POST stays typed and the handler's validation answers in the
  application's own words. Fields are named by the form tag, else the json tag,
  else the lowercased field name, and a path value wins over a query parameter —
  the same rules the generated transports decode with. A non-struct request
  (a map, a slice) decodes from the body only. Promised as
  `kit.typed-request-decoding`.
- The typed JSON registration family (`HandleJSON`, `HandleJSONTyped` and
  their middleware, endpoint, and body-limit variants, and `HandleSSETyped`)
  now takes a `Router`: an HTTP component, or a `Group` cut from one. This is
  the one incompatible edge of the change — every caller that passed an
  `*HTTP` still compiles, because an `*HTTP` is a `Router` — and it is
  recorded in the compatibility gate's deliberate-breaks ledger.
- Routes group: `svc.Group("/api/v1")` returns a prefixed view whose
  registrations carry the prefix, nestable, with the component's full
  registration surface (`HandleJSONTyped`, the raw escape hatch, SSE streams).
  A group only joins patterns — the route keeps the component's HTTP context,
  endpoint middleware (labeled with the joined pattern), recorders, and JSON
  server options. Promised as `kit.route-groups`.

### Changed (documentation)

- The Routing chapter's path-and-query section now shows the typed route doing
  the filling and the raw handler as the escape hatch it always was, and gains
  a Route groups section. The CRUD tutorial's routes are typed end to end —
  including the path-parameter routes and the body-less done route — and its
  raw delete sends errors through the framework encoder, so its hand-rolled
  error-to-status mapping is gone; the todosvc example is the same file, and
  both tutorials were reassembled block by block and driven through every
  documented response.

- `-openapi` now defaults to true: every generated project answers with the
  OpenAPI 3.1 contract, the JSON Schema bundle, the embedded Swagger UI, and
  the TypeScript client without asking for it — the input an AI agent needs
  to build a frontend from the API. Pass `-openapi=false` for the old
  behavior. Projects already generated keep whatever their manifest records;
  extend reads the capability back from `.microgen/manifest.json` and does
  not retrofit the new default onto them.

### Documentation

- The generated `.ai/PROJECT_GUIDE.md` gains "Generate A Frontend From The
  Contract": where operations come from (`operationId`, `$ref`s), how to read
  message shapes from `docs/schema.json` (`required`, `format`, `example`,
  `additionalProperties: false`), calling through the TypeScript client or
  plain fetch, and the repeating list/form/delete page recipe — with the
  `MICROGEN_API_GAP` rule extended to the TypeScript SDK. MICROGEN.md carries
  the same workflow for developers, and the documentation indexes route the
  task. A constraint syntax for the IDL (enum, length, ranges) remains
  deliberately out of scope; the contract carries what a form generator
  needs.

- A [Routing](docs/routing.md) chapter now teaches route definition end to
  end: ServeMux pattern syntax and its semantics (specificity, conflicts,
  405+Allow), the registration call to reach for with the trade each makes,
  path and query parameters (including combining a typed JSON body with path
  values), pagination, multipart, downloads, the options that shape every
  route, SSE route definition with its timeout and shutdown rules, the gRPC
  registration story, and how generated projects register, extend
  (`cmd/custom_routes.go`), and expose `/debug/routes`. The chapter's code
  was verified by compiling it against the module and running it.
- The transport-neutral `security` package and the standalone `health`
  package each gained the README their contracts deserve, and the
  documentation indexes now reach them, `kit/grpc`, and `integrations/zap`.
  `docs/configuration.md` states how a hand-built service configures itself,
  MICROGEN.md shows how to call the generated Go and TypeScript SDKs, the
  authentication tutorial links the security package's README instead of
  security/http's, and the testing chapter covers SSE, multipart, and
  outbound-call substitution. A runnable SSE example remains deliberately
  out of scope; the routing chapter and the transport README carry the
  behavior until then.

- The CRUD and authentication tutorials are now complete walkthroughs for a
  beginner: each carries a module-setup step, and every layer (storage,
  service, HTTP, wire-up) is shown as finished code, so assembling the
  sections in order yields a runnable `main.go` — the same file as the
  corresponding example, minus its package comment. Both were verified the
  way a beginner reads them: the Go blocks were extracted from the tutorial
  in order, compiled into a fresh module, run, and exercised against every
  documented response. The MCP tutorial's elicitation example no longer
  calls an undefined helper; it now shows the second round doing the real
  work with the returned state.

## [2.24.0] - 2026-10-11

### Added

- Generated projects now answer "which files may I edit?" from the files
  themselves. Every file the generator rewrites on each run carries the
  standard Go generated-code marker `// Code generated by microgen. DO NOT
  EDIT.` as its first line — including the endpoint set, HTTP and gRPC
  transports, client demo, service tests, proto contract, model, repositories,
  and the generated endpoint chain — and every file microgen writes once
  (`cmd/main.go`, `service.go`, the custom chains, `config.yaml`, the model
  hooks, `README.md`) states in its opening comment that it is user-owned and
  never overwritten. The generated `.ai/PROJECT_GUIDE.md` gains a Directory
  Map covering every directory of a generated project plus the two
  machine-checkable ownership markers, and MICROGEN.md's File Ownership lists
  name every user-owned and generator-owned path. A generator test pins the
  contract: every artifact the manifest declares as generator-owned carries
  its marker on the first line, and no write-once scaffold claims one. JSON
  contract files (`openapi.json`, `schema.json`, `tsconfig.json`) carry no
  in-file marker — a comment would break strict JSON consumers — so their
  ownership is carried by the manifest artifact list and the guide.

### Fixed

- The generated service tests no longer panic on a fresh project. Their
  middleware demos passed a nil logger to `LoggingMiddleware`, so the first
  `go test ./...` died in a nil-pointer dereference inside the logging
  middleware instead of reporting the not-implemented stubs. The demos now
  pass a discard logger.
- The CRUD and authentication tutorials' example curl calls left out
  `Content-Type: application/json`, so the documented commands were answered
  415 by the strict JSON decoding they teach. All five tutorials were then
  executed step by step against the released version: getting-started,
  generating a service (install, generate, run, extend), the CRUD service,
  authentication (all five rows of its goal table), and the MCP server on
  both protocol revisions.

### Documentation

- MICROGEN.md and PRODUCTION.md gained a table of contents (13 and 18
  sections respectively), in both languages. The audit behind it: every
  relative documentation link resolves, every English/Chinese document pair
  has matching structure, and the task router, book index, and examples
  catalog cover every chapter and every example directory.

## [2.23.0] - 2026-10-08

### Added

- `sd/client.NewEndpoint` now exposes `WithRetryClock` and `WithRetryBackoff`,
  delegating to the configurable retry executor while preserving its one-attempt,
  500ms defaults, classifier and caller-owned discovery source. Nil values restore
  the standard timing policy; typed-nil clocks fail before subscribing.
- `sd/retry.New` configures a retry endpoint with `WithMaxAttempts`, `WithTimeout`,
  `WithAttemptCallback`, `WithErrorClassifier`, `WithBackoff`, and `WithClock`.
  New defaults to one attempt and no added deadline. A configured clock controls
  backoff waits, while the total timeout and measured latency remain real-time.
  Existing `Retry`, `WithCallback`, and `WithClassifier` signatures and policies
  remain compatible. Per-call schedules, cancellation, independent attempt caps,
  and legacy defaults are covered by executable tests.
- The promise markers now cover the packages that carried none: `health`,
  `sd/balancer`, `sd/endpointer`, `observability/slog` including its telemetry
  assembly, `integrations/zap`, `integrations/consul`, `integrations/etcd`, and
  the CORS, security-headers, trusted-proxy, IP-policy, and chain surfaces of
  `security/http` — 76 new `Stable:` entries plus one recorded `Unstable:` (the
  Consul registrar emulates no conflict mode), each naming the tests that hold
  it. The Consul instancer test now also asserts the client-side AND filter
  across `TagsInstancerOptions` tags, and the slog adapter pins the panic record
  its documentation promises with a test of its own.
- `endpoint.OperationIntent` carries the declared why of one endpoint call — a
  query's purpose on reads, an audit reason on writes — through the standard
  context pair `WithOperationIntent` and `OperationIntentFromContext`. The
  framework applies no policy to an intent: it is not required, validated, or
  recorded by the framework itself, an absent or zero intent reads back as the
  zero value, and the transport mapping and the consumers stay
  deployment-owned. `examples/best_practice` demonstrates both halves; the
  promise is pinned as `endpoint.operation-intent`.
- The generated `.ai/PROJECT_GUIDE.md` gains an API Discovery Protocol: the
  reading order (guide, README, godoc), the rule against hand-editing or
  grepping generator-owned files, and a named stop signal —
  `MICROGEN_API_GAP` with the language, service, missing operation, and
  compiler diagnostic — for an operation the generated API does not expose.
  The guide now ships with every generated project, not only
  interaction-enabled ones: an agent working on a plain HTTP or gRPC service
  needs the ownership map and the discovery protocol just as much. The project
  manifest lists it as a generator-owned artifact in every project, and the
  contract snapshots record the addition.

### Fixed

- Client construction now rejects typed-nil balancer results as well as nil,
  releases the endpoint subscription and factory resources before returning,
  and includes any cleanup failures in the returned error.
- `endpoint.ManualClock` removes stopped timers immediately, so `Pending()`
  excludes cancelled waits and cancelled retries do not accumulate timers until
  their original deadlines. Stopping and advancing share one synchronization
  boundary; completed timers are also released from the pending slice.
- The active health checker now preserves a persistent service-discovery error
  while periodic probes continue, so downstream invalidation and stale-snapshot
  policies can still see the registry outage. A successful source snapshot clears
  the error.
- Instance snapshot de-duplication now safely compares slice, map, interface,
  and non-comparable error values instead of panicking.
- Least-request selection now performs a true full scan when its choice count
  covers the snapshot, so it cannot miss the lowest-load instance through
  repeated random samples.
- `sd/retry` runs a single-attempt call on the caller's goroutine instead of
  spawning an attempt goroutine and a buffered channel for a call that can
  never retry. The default one-attempt configuration now costs only the call
  itself on the hot path; the result shape, Done reporting, callback Final, and
  no-attempt-after-budget handling are unchanged. A failure whose budget ends in
  the same instant still reports the attempt and the endpoint's kind rather than
  masking it behind a bare context error.

## [2.22.0] - 2026-09-10

The service-discovery subtree had never been audited. Two audits of it — the load
balancing path and the resilience path — opened this milestone.

### Added

- **`retry.Error` now implements `Is` and `As`, so every cause of a retried failure is
  reachable.** `Unwrap` can only offer one, and `Final` wins when it is set: after a
  budget expiry the only reachable cause was `context.DeadlineExceeded`, so an
  upstream's `apperror` kind — recorded in every attempt — was invisible to
  `errors.As`, and a mapper keyed on kinds emitted a generic timeout for a failure
  whose real kind had been in hand all along. A `Callback` replacement hid the actual
  error the same way. `errors.Is` and `errors.As` consult these methods before walking
  the unwrap chain, so the attempts and the final cause are both reachable, newest
  attempt first. Changing `Unwrap` to return `[]error` would have achieved the same
  thing by breaking a published signature — the compatibility gate refused it, and it
  was right to.

### Fixed

- **A retried call could throw away a response it had already received.** The
  loop selected on the call's context and on the attempt's result channel, and a
  select with two ready cases chooses uniformly: an attempt completing in the same
  instant as the budget expiring had a coin-flip chance of being discarded and
  reported as `context.DeadlineExceeded`. For a non-idempotent request that is the
  worst available report — the write happened, the caller was told it did not, and
  it cannot compensate for what it never learned about. An attempt now hands its
  result to the loop before the outcome callback runs, so deployment code in `Done`
  cannot widen the window, and the loop drains a delivered result before honouring
  the context.
- **A call whose budget was already spent still dispatched one more attempt.** The
  loop launched the attempt goroutine as its first statement and only then looked
  at the context, so a caller that had given up — or a backoff timer that returned
  after the budget expired — still caused a `Pick` and an upstream request whose
  result nothing would read. The context is now checked before dispatching.
- **Concurrent applications of an ejection policy could take more of a pool out of
  service than the cap allows.** `feedback.Ejector` judged its candidates and checked
  `MaxEjectionPercent` outside the lock and re-acquired it to apply, so two picks
  over a four-instance pool could each pass a 50% check against a view that excluded
  the other's decision and eject together — 75% of the pool gone, in the situation
  the cap exists to prevent, with panic mode never firing. The same window let both
  picks eject the *same* instance, and ejections are counted to grow the window, so a
  first offence was recorded as two and the instance stayed out for twice its
  configured base duration. Deciding and applying now happen in one critical section;
  measurements are read before it, because they come from the `Table`'s own mutex and
  `Table.Follow` drives `Ejector.Retain` in the other direction.
- **A discovery flap resurrected a failing instance as healthy.** `health.Checker`
  deleted an absent address's state and recreated it on return with
  `initiallyHealthy` — true by default — and no recorded failures, so a backend it
  had just taken out of service was republished as serving and could not be ejected
  again for `unhealthyThreshold` probe rounds. Registries flap: a heartbeat TTL
  expiring a beat early, a paginated or partial list, a provider briefly reporting a
  subset. State now survives a single absence and is dropped on the second, so a
  flap costs nothing and a real departure is still forgotten. Note that a source
  which suppresses an event identical to the one before it — `instance.Cache` does —
  delivers that second absence only when the set changes again.
- **Concurrent updates could leave a subscriber holding an older instance list than
  the one the cache reports.** `instance.Cache.Update` stored the new state under its
  lock and broadcast after releasing it, so two callers updating at once could store A
  then B and deliver B then A. `sendLatest` made it worse rather than better: finding
  the buffer full it drains and rewrites, so the older event replaced the newer one.
  `State` then disagreed with every subscriber until the next update, and because an
  event equal to the stored one is dropped, a repeat of the stale list kept the
  divergence instead of correcting it — requests kept going to a withdrawn address.
  The broadcast now happens under the lock, which is safe because every channel
  operation in `sendLatest` has a default case and cannot block on a slow subscriber.
- **Three smaller findings from the same audits.** A weighted pick summed weights
  without a bound, so two instances registered near `MaxInt` wrapped the total
  negative and a fully healthy pool reported `ErrNoEndpoints` — classified as
  temporary, so callers burned their retry budget on it first; weights are now clamped
  so the total cannot overflow. `endpointer.Filter` and `Prefer` accepted a nil source
  and deferred the panic to the first request, while every sibling constructor
  validates at assembly time. And `feedback.Follow` kept the provider's instance slice
  by reference, at the boundary where `health.accept` copies and explains why — a
  provider that reuses its backing array could rewrite a set already handed to a
  retainer.

## [2.21.0] - 2026-09-08

What the generator emits is part of the framework. This milestone comes from two
audits: the first ever look at the code-generation path, and a look at what happens
when features are combined rather than used alone.

### Changed

- **The API compatibility gate no longer passes green on a tagless checkout.**
  `TestAPICompatibilityWithLastRelease` skipped when no `v2.*` tag was reachable,
  and a skip is green — so a shallow clone, a `git archive` export, or a CI job
  that fetches without tags silently disabled the gate that guards the entire
  exported surface. It now fails and says which of the two situations you are in;
  the honest case, nothing released yet, is `-allow-missing-release-tag` and has to
  be typed.
- **The generated-contract snapshots can no longer be refreshed out of band.**
  `UPDATE_CONTRACT_SNAPSHOTS=1` refreshed them alongside the flag, so a value left
  in a shell profile or leaked into CI made all three permanently self-blessing
  with nothing on any command line to notice. Only the flag remains.
- `TestEveryContractSnapshotHasALiveCaller` refuses a snapshot nobody reads. The
  comparison lives in a helper the integration tests call, and deleting one call
  line produced no failure anywhere: the `.sha256` file stayed in the tree unread,
  and the tests hosting those calls are not among the gates `RELEASE.md` names, so
  the contract test that checks gates still exist could not see it either. The new
  gate walks the snapshots instead of the callers, and is itself named in
  `RELEASE.md` so deleting it is caught.
- **The API surface gate now names the packages that moved.** It stores one digest
  per package, and its failure printed two blocks of thirty-five hex strings, so the
  reviewer's first task was to diff hex by eye before they could begin on the
  question the gate exists to ask. The failure now lists the changed packages,
  marking any that appeared or disappeared, and points at `go doc -all`. The stored
  form is still a digest: this makes the failure legible, not the snapshot
  reviewable.

### Fixed

- **The generated MCP wiring was broken by v2.20.0.** Tightening the Origin check
  refused browser clients that used to be served, and no template mentioned
  `AllowedOrigins` or `TrustRequestHost` — a grep across `cmd/microgen` found zero
  hits for either. The generated `main` now carries the two fields and the reason
  the request's own `Host` is not trusted, so the person reading the generated code
  can see the decision instead of debugging a 403.
- The generated shutdown never called `mcpHandler.Shutdown`, so MCP sessions and
  their goroutines were not drained. It now runs before the HTTP shutdown: a session
  holds an open SSE stream, and closing the sessions is what lets `Shutdown` finish
  instead of burning the whole budget and falling through to `Close`.
- The generated success encoder was `json.NewEncoder(w).Encode(response)`. That sets
  no `Content-Type` — so `net/http` sniffs and answers `text/plain` for JSON, which
  contradicts the OpenAPI document the same generator writes beside it — and it
  writes the body before any `StatusCoder` or `Headerer` on the response type is
  consulted, pinning every answer to an implicit 200 and losing the 204-no-body
  rule. It now calls `server.EncodeJSONResponse`, which is the encoder that carries
  those promises.
- A generated project with `--grpc` did not build: `go.mod` required neither
  `google.golang.org/grpc` nor `google.golang.org/protobuf` while the generated
  `main` imports both. Both are now required, at the versions this module itself
  uses.
- `kit.HandleSSETyped` never passed the component's JSON server options to the SSE
  server, while `kit`'s package documentation listed it under "the component's JSON
  server options all apply". A deployment that installed `ProblemJSONErrorEncoder`
  component-wide got `application/problem+json` on every JSON route and a plain
  envelope on an SSE decode failure — one service with two error contracts. The
  options are now passed, component first so a route's own still win.
- **An SSE route answered its own errors two different ways.** A decode failure went
  through the encoder the route's options resolved to, while a middleware rejection
  was hardcoded to `JSONErrorEncoder` — so one route rendered a 400 as text and a
  401 as JSON, and a deployment that installed its own encoder got it on one and not
  the other. The rejection now uses the stream's own encoder, which
  `server.SSEServer.ErrorEncoder` reports. A component that installed no encoder
  therefore answers a rejection the way the rest of its routes answer errors, which
  is a visible change if you were relying on the JSON envelope: install
  `ServerErrorEncoder(server.JSONErrorEncoder)` through `WithJSONServerOptions` to
  keep it, and get it on every route rather than on one path of one route.
- **A stream that died on its third event recorded as a success.** The bridge that
  makes one stream one request for the endpoint middleware returned
  `(struct{}{}, nil)` no matter what the stream did, so every metric, log and trace
  saw a completed request. `server.SSEServer.ServeStream` is `ServeHTTP` that returns
  the error that ended the stream, and the bridge returns it to the chain. The
  response cannot change once 200 and some events have been flushed — the error is
  reported, not rendered — and a rejection that happened before the stream started
  is still the one case that writes an error response.
- Text from an IDL or a database schema reached generated Go string literals,
  struct tags and comments unescaped. The generator had an `escape` helper that
  replaced only the double quote — not a backslash, not a newline — and no template
  used it. A doc comment containing a quote produced `Description: "a "user"
  record"`, which is *valid Go syntax*, so the generator's own format check passed
  it through to fail in the user's build; a newline or a trailing backslash aborted
  generation after earlier files had already been written; a backtick in a column
  name terminated a struct tag's raw literal. `escape` is replaced by `quote`
  (`strconv.Quote`, which handles all of them together), `comment` (folds every
  line terminator, because a `//` comment ends at the newline and `.proto` output
  never reaches a formatter) and `tag` (removes what a raw string cannot escape),
  and all nineteen interpolation sites across `interaction`, `model`, `service`,
  `sdk` and `proto` templates now use them.
- **The generated OpenAPI document contradicted the generated handler.** Every
  message schema was silent about `additionalProperties` while the decoder sets
  `DisallowUnknownFields`, and silence means extra properties are allowed — so a
  client generated from the document could be rejected by the service generated
  beside it. Message schemas now say `false`. `ErrorResponse` deliberately does not:
  the error encoder is a deployment's choice and `ProblemJSONErrorEncoder` writes a
  wider document, so closing that schema would promise something the generator does
  not write.
- **Pointer fields lost their nullability in all three outputs.** The OpenAPI schema,
  the JSON Schema bundle and the TypeScript SDK each named the value's type, while a
  nil pointer marshals to `null` because generated structs carry no `omitempty` — the
  document described a payload the service does not produce, and the TypeScript
  consumer's guard was written against the wrong shape. A plain type is now
  `["string", "null"]`, a `$ref` is wrapped in `anyOf` (keywords beside a `$ref` are
  not honoured everywhere), and the TypeScript field is `T | null`.

### Documentation

- `MICROGEN.md` now states the three rules that connect the generated document to
  the generated handler, including the one that is a derivation rather than a fix:
  `required` comes from the type, a pointer is how optional is declared, and it is a
  statement about the payload rather than a promise that the server rejects an
  omission. Presence cannot be told from a zero value on a non-pointer field, so a
  missing `count` arrives as `0` — declare the field as a pointer when absence has to
  be a different answer from zero.
- `kit`'s registration guide over-claimed for `HandleSSETyped`, which this
  repository wrote two releases ago. Both caveats it grew in the meantime — the
  hardcoded rejection encoder and the stream that always looked successful — are
  fixed above rather than documented, so the guide now states what the function
  does and names the one limit that is real: once a stream has answered 200, an
  error can be recorded but not rendered.
- The canonical SSE example ignored the `Stopping` announcement and selected only on
  `ctx.Done()`, contradicting the drain example in `kit/drain.go`. A stream written
  from it kept running until the grace period cancelled its context, which is the
  client seeing a cut connection rather than the end of a stream — and the SSE
  example is the one an SSE author reads.

## [2.20.0] - 2026-09-08

Nothing in a request can vouch for itself.

### Changed — action may be required

- **`mcp.StreamableHandler` no longer allows an `Origin` that matches the
  request's own `Host`.** `Host` is supplied by the caller, so comparing one
  against the other defeats the attack Origin validation exists to stop for a
  locally bound MCP server: under DNS rebinding a browser sends
  `Origin: http://evil.example` alongside `Host: evil.example`, and the comparison
  said yes.
- The behaviour is now a seam rather than a policy. Set
  `StreamableHandler.TrustRequestHost = true` where the deployment knows `Host` is
  trustworthy — a reverse proxy that overwrites it, or a network no browser can
  reach — and it behaves exactly as before. Leave it off otherwise.
- **If a browser client stops being served after upgrading**, name its origin in
  `AllowedOrigins`. That is the answer that always works, because it does not
  depend on a header the caller controls. `TrustRequestHost` restores the previous
  behaviour verbatim if that is what you want.
- Unaffected: a request with no `Origin` header is still served — it did not come
  from a browser, so there is no browser-imposed origin to check — and an origin
  listed in `AllowedOrigins` is still served.
- The audit that found this classified it as "declared, but the consequence was
  not": the struct comment did say same-origin requests are always allowed. It
  said nothing about what `Host` is worth. No test exercised the shortcut, which is
  how the consequence stayed unnoticed. Declares
  `mcp.request-host-is-not-trusted-by-default`.

## [2.19.0] - 2026-09-08

What the promise did not cover. Every behaviour in this framework is marked in the
source as `// Stable: <id> — <promise>` with the test that covers it, and a gate
checks that the named test exists. This release opens a milestone from asking the
next question: does the test assert what the promise says? Where it did not, the
gap was not in the prose — it was a path nobody had walked.

### Fixed

- MCP `GET` and `DELETE` never reached the `MethodAuthorizer`, while
  `mcp.method-authorization` promised that every request does. A caller holding a
  session ID could attach to the stream the server pushes notifications and
  sampling requests down, or terminate any session, with the deployment's policy
  never consulted. Both now reach it as `mcp.MethodOpenStream` (`stream/open`) and
  `mcp.MethodDeleteSession` (`session/delete`) — namespaces no MCP method uses, so
  a policy decides on them without matching HTTP verbs — and a refusal is 403 with
  nothing done, because there is no request id to answer. Authorization runs before
  the session lookup, so a refused caller learns nothing about which sessions
  exist. The covering test drove ten stateless POST methods and no `GET` or
  `DELETE`; it does now. Declares
  `mcp.transport-operation-authorization`.
- SSE framing split lines on LF alone. The specification ends a line on CRLF, CR,
  or LF, so a payload carrying a bare CR was emitted inside a `data:` line, where a
  conforming client ends the field and reads the remainder as a field with no name
  — the payload arrived silently truncated. All three terminators now start a new
  data line. Declares `http.sse-line-terminators`.
- SSE data, comment text and event names were unescaped, so any of them could end
  its own frame and inject an event of its choosing into the stream — reachable by
  any handler that streams caller-influenced text. Data and comments now stay
  entirely data and comments (a blank line inside a comment becomes another comment
  line), and an event name carrying a line terminator is an error rather than a
  frame: a field value cannot hold one, so the name is a caller bug, and neither
  stripping it nor passing it through is an honest answer. Declares
  `http.sse-no-frame-injection`.
- An error response could carry two `Content-Type` values. The encoder set one and
  then merged the headers a `Headerer` error reported with `Add`, so an error whose
  `Headers()` included a `Content-Type` produced a response no client can
  interpret. The encoder chose the body, so it names the type — and names it last,
  after everything else the error asked for is merged. Three call sites collapse
  into one helper. Declares `http.error-single-content-type`.
- An MCP cursor this server never issued was treated as offset 0, so a client that
  had persisted one across a catalogue change was quietly handed page one as though
  it were its page. A cursor is the offset the server returned as `nextCursor`, so
  one that is not a number, is negative, or is at or past the end is invalid: all
  four list methods now answer `-32602`, which is what the specification requires
  and the only answer a client can notice. One `listError` now backs every list
  method, so a bad cursor cannot be invalid params on one and an internal error on
  another. Declares `mcp.invalid-cursor-is-invalid-params`.
- An MCP response that could not be serialised reached the caller as an empty SSE
  event or a truncated body, and the caller waited for a reply that never came with
  nothing logged. Both paths now marshal before writing anything, so either the
  whole response arrives or an internal error naming the fault does. Declares
  `mcp.response-is-whole-or-an-error`.
- `mcp.error-codes` enumerated the codes the server emits and omitted `-32021`,
  which `stateless.go` returns and its own marker promises. Two markers disagreed
  about the vocabulary; the enumeration now names it.
- A behaviour marker in a `_test.go` file was never reviewed by the behaviour gate,
  so it read in the source as a promise while sitting outside the freeze. The one
  that existed — `otel.metrics-agree-with-the-exposition` — moves onto `NewMetrics`,
  where it belongs, and `TestBehaviourMarkersLiveInNonTestSource` makes the next one
  fail loudly rather than be skipped.

### Changed

- `http.multipart-limits` said "an over-limit body or file is 413
  request_too_large". The file case has always emitted `request_too_large.file`,
  which is the more useful answer — it names which limit was hit — so the promise
  now states both codes rather than the behaviour degrading to the vaguer one.
- `MultipartLimits` now states which field bounds work. A file's size is known only
  once its part has been read, so `MaxFileBytes` bounds the answer while
  `MaxBodyBytes` bounds how much a request can make the process write to disk. That
  asymmetry was real and undocumented; enforcing per-part limits while streaming
  was considered and rejected, with the reason recorded in the roadmap. A refusal
  does take its temporary file and its parsed form with it, which is now declared:
  `http.multipart-file-refusal-leaves-nothing-behind`.

## [2.18.0] - 2026-09-08

Read by somebody else. This release opens a milestone that came out of reviewing
the framework from five users' points of view — someone arriving for the first
time, someone writing business logic, someone operating it, someone extending it,
and someone writing tests against it. The operator's view turned out to be the
best served and the test author's the worst, and the documentation gaps were not
where the prose is but where the godoc is.

### Added

- `endpoint.Clock` makes time an input a test can decide. `time.Now()` was called
  directly from production code, so a deployment could not test a backoff
  interval or a token expiry without sleeping through it — and neither could this
  repository. A nil Clock means the wall clock, so a service that does not care
  writes nothing and nothing about its behaviour changes.
- `endpoint.NewManualClock` returns a clock a test moves by hand, with `Advance`,
  `Set` and `Pending`. It ships beside the contract rather than in a test-only
  package, because a seam nobody can reach is not a seam: an application accepts
  the same clock this framework's components accept. It is safe to advance from
  the test goroutine while the code under test reads it from others.
- The seams: `endpoint.WithRetryClock` for the wait between retry attempts,
  `endpoint.Metrics.Clock` for the `LastRequestTime` a snapshot reports, and
  `httpsecurity.CSRFConfig.Clock` for when a token is minted and when its TTL is
  checked. The CSRF one is declared structurally rather than imported, because
  that package deliberately depends on nothing else here — any value with a `Now`
  method fits, `ManualClock` included.
- What the clock deliberately does not decide is how long real work took.
  `Observation.Duration` and logged request durations stay measured: a clock that
  could shorten them would make a recorder report something untrue about the
  system. Declares `endpoint.clock-nil-is-wall-clock`,
  `endpoint.manual-clock-advance-fires-due-timers`, `endpoint.retry-clock`,
  `endpoint.metrics-clock` and `security.csrf-clock`. `docs/testing.md` has the
  section, in both languages.

### Deprecated

- `kit.HandleSSE` is deprecated in favour of `Handle`. Its body was always one
  line — `h.Handle(pattern, handler)` — so it carried none of the SSE behaviour
  its name implies, and a reader who followed the name from `HandleSSETyped`
  silently lost the endpoint middleware chain and the recorders. `Handle` is the
  same call under a name that says what it skips. It stays reachable rather than
  being deleted, because the API-compatibility gate is right that a published
  symbol is a promise.
- `kit.JSON` and `kit.JSONTyped` are deprecated in favour of
  `kit.NewJSONHandler` and `kit.NewJSONTypedHandler`. They return an unregistered
  handler and differed from `HandleJSON` / `HandleJSONTyped` by one verb, which is
  not enough to tell apart the function that mounts a route from the one that does
  not. The `New` prefix matches `httpserver.NewJSONServer`, which is what they
  wrap.

### Changed

- `kit`'s package documentation now groups all eleven registration entry points by
  what they actually do — full chain, escape hatch, or neither — so the API states
  the distinction instead of relying on the reader having found the customization
  table first.

### Documentation

- `apperror`'s package comment was four lines for the package business code
  imports first. It now answers what a reader arrives with: which of the three
  things an error carries to be deliberate about, why the empty kind becomes
  KindInternal, what happens to the message at 500, when to reach for `WrapCause`
  over `Wrap`, and that the structural `KindNamer` contract means nothing has to
  import this package to be classified correctly.
- `sd/endpointer` and `sd/instance` had no package comment at all, and
  `sd/balancer`, `sd/retry` and `sd/client` had one line each. All five now explain
  what the package is for and how it differs from its neighbour — why balancing is
  a separate package from selection, why `sd/retry` picks again per attempt while
  `endpoint.RetryMiddleware` repeats one endpoint, and what `InvalidateOnError`'s
  zero value actually does.
- `DOCS_INDEX.md` and `docs/index.md` are now supersets of each other. Each was
  missing documents the other listed, so finding one depended on which index you
  opened.

## [2.17.0] - 2026-09-08

Defaults that are not wrong. This release opens a milestone that came out of a global
audit rather than a feature idea: after sixteen milestones of pinning behaviour, the
remaining weakness is not missing capability, it is the few places where a default
arrives by inheritance from the standard library.

### Added

- `kit.HandleJSONEndpointWithBodyLimit` and `kit.HandleJSONTypedWithBodyLimit` let one
  route accept a different maximum request body. This closes a trap rather than adding a
  knob: `kit.WithJSONMaxBodyBytes` is component-wide, so a service with one upload route
  previously had to register it through `kit.Handle`, which silently skips the endpoint
  middleware, the recorders, and the JSON server options the component installs — losing
  observability as a side effect of setting a body size. The new registrations take the
  same path as every other JSON route and differ only in the limit. A limit of zero or
  less panics: an unbounded body is not a value to smuggle in per route. Declares
  `kit.per-route-body-limit`.
- `server.ProblemJSONErrorEncoder` writes RFC 9457 `application/problem+json`.
  It is offered, not installed: `ErrorResponse` is declared stable and is what
  every service on this framework already emits, and which error format a service
  speaks is a decision its clients live with — a seam, not framework policy. The
  status mapping, the redaction at 500, `Headerer` and `Retry-After` are
  unchanged, and the machine-readable code survives as an extension member, so a
  client switching on `code` keeps working. What it buys is the field list a
  validation failure has always carried and the envelope has nowhere to put:
  `endpoint.ValidationError`'s `[]FieldError` becomes `errors`, instead of being
  flattened into one `message`. The `type` URI identifies a document somebody has
  to publish and keep resolvable, so none is invented — `nil` writes
  `about:blank`, and a `ProblemTypeResolver` supplies real ones.
  `server.ProblemFromError` and `server.WriteProblemJSON` are exported for the
  encoder a deployment writes itself, including one with its own kind mapper.
  Declares `http.problem-media-type`, `http.problem-document-shape`,
  `http.problem-redaction` and `http.problem-encoder-parity`. `docs/errors.md`
  has the section, in both languages.
- `endpoint.KeyedRateLimiter` makes a per-caller limit expressible. `RateLimiter`
  limits the process as a whole — `Allow()` takes no key — so per-tenant or
  per-API-key limiting was not merely absent, it was structurally impossible
  through the supported contract, and one noisy caller could reject everybody.
  The new contract is `AllowKey(ctx, key)` / `WaitKey(ctx, key)`, installed with
  `KeyedRateLimitMiddleware`, `DelayKeyedRateLimitMiddleware`,
  `Builder.WithKeyedRateLimit` or `WithDelayKeyedRateLimit`, and the key comes
  from a `RateLimitKeyFunc` because what identifies a caller is the deployment's
  to decide. It is a second contract rather than two more parameters on the
  first: an unkeyed limiter has no per-key state to consult and must not be able
  to claim otherwise by ignoring an argument. An empty key is limited under the
  empty key, never exempted, so a missing header is not a way out, and a nil key
  function panics at assembly because it would silently be the process-wide limit
  again. `KeyedRetryAfterReporter` reports a per-key wait; the unkeyed
  `RetryAfterReporter` still works when every key shares a refill rate. The token
  bucket is still the application's. Declares `endpoint.keyed-rate-limit` and
  `endpoint.keyed-rate-limit-shares-one-bucket`.

### Changed

- A client built by `transport/http/client` no longer uses `http.DefaultClient`. Two
  things about that were wrong for a service, neither of them policy. It is a
  package-level variable any library in the process can reconfigure, so somebody else's
  timeout or transport swap could change these calls. And `http.DefaultTransport` allows
  two idle connections per host — right for a tool calling many hosts once, wrong for a
  service calling one upstream on every request, where it surfaces as latency the
  application code cannot explain.
- The client this package uses is now its own, with `MaxIdleConnsPerHost` 100, a
  60-second idle timeout, and bounded dial and TLS handshake. `client.DefaultClient()`
  exposes it so calls made outside this package can share the pool, and
  `client.NewTransport()` returns a fresh transport to start from when a deployment needs
  a proxy or a `tls.Config` but wants the pool sizing. The constants are exported and
  named.
- It still sets no `Timeout`, and that is now stated rather than implied: a timeout there
  would cap every call in the process at a number the framework invented, invisibly from
  the call site. The deadline belongs to the call —
  `NewJSONClientWithTimeout`, `endpoint.Builder.WithTimeout`, or a context you derive.
  `SetClient(nil)` now falls back to this package's client rather than the process-wide
  default.

Declares `httpclient.pool-is-ours` and `httpclient.no-invented-deadline`.
`PRODUCTION.md` says what changed and what is still yours to set, in both languages.

## [2.16.0] - 2026-09-07

A correction, and the thing it excused.

### Fixed

- `kit/grpc` implements `grpc.health.v1.Health/Watch`: the current serving status
  immediately, then one message per change, from the same probe registry `Check`
  evaluates. Milestone 14 declared `Watch` unimplemented because "the tools that
  orchestrate on gRPC health call `Check`" — true of `grpc_health_probe` and of
  Kubernetes' native gRPC probe, and false of grpc-go's own client-side health
  checking, which calls `Watch` and, on `UNIMPLEMENTED`, marks the connection ready and
  stops asking. A client with health checking enabled therefore never learned that an
  instance had begun draining, which is exactly what the drain announcement exists to
  tell it.
- `kit/grpc.HealthWatchInterval` names the resolution of that stream: one second, and
  the readiness checks are evaluated once per interval and shared by every watcher
  rather than once per watcher per message — a fleet of clients should not turn a
  database ping into load. The poller stops when the last watcher leaves. It is a
  constant rather than an option because the component registers the health service
  itself; a deployment that needs different behaviour builds its own `grpc.Server`, and
  that limit is now written down.
- The transport parity table in `docs/lifecycle*.md` says what is true.

## [2.15.0] - 2026-09-07

Rotation without a restart. Milestone 13 shipped in-process TLS and then wrote its own
gap down: certificate files are read once, so renewing one meant restarting the
process. That sentence was honest and the behaviour was poor — a certificate expires on
a schedule, and the platform that renews it replaces files under a running service and
expects it to notice. This release makes the certificate replaceable while the listener
serves, and keeps a broken renewal from costing anyone their listener.

### Added

- `kit.CertificateSource` is the seam: one method, asked on every handshake, so nothing
  about the certificate is captured when the listener starts. Where the certificate
  comes from stays the deployment's — a file, a secret manager, an ACME client, a
  per-name map for SNI — and `kit.CertificateSourceFunc` adapts a closure.
  `kit.WithTLSCertificateSource` installs it, imposing nothing beyond the minimum
  version `WithTLSConfig` already defaults.
- `kit.CertificateFiles` is the file-backed source a mounted secret needs.
  `NewCertificateFiles` loads the pair up front, so a bad path still fails startup with
  the path in the error rather than a client's handshake, and `Reload` re-reads both
  files when the deployment says so. When to say so is deliberately not the framework's:
  no filesystem watch, no timer, no signal handler is installed, because each is a
  policy a deployment would have to work around. `Certificate` never touches the disk,
  so the handshake path cannot be slowed or failed by it.
- A failed reload returns the error and keeps serving the pair already loaded. A
  half-written secret is a log line, not an outage, and the test asserts the listener
  still answers with the previous certificate.
- A generated service serves its certificate through the same per-handshake seam and
  re-reads both files on `SIGHUP`, logging the paths on success and keeping the previous
  certificate on failure. Renewing is `kill -HUP`, not a deploy. `SIGHUP` because it is
  the signal an operator already reaches for and a renewal job can send; the process
  does not poll, and nothing watches the filesystem on your behalf.
- The configuration guide now says which of the things you configured are read again and
  which are read once: the certificate on your trigger, the rest of the `tls.Config` at
  construction, the address and timeouts and routes at `Start`, and the config file and
  environment once per process. Everything outside the first group means a new listener,
  which means the rolling restart the drain sequence exists to make uneventful.

## [2.14.0] - 2026-09-07

Two transports, one contract. Thirteen releases were spent making the HTTP surface
honest about readiness, draining, streaming, metrics, and TLS; the gRPC surface came
along for some of that and not the rest. The RPC layer itself is not the gap — it has
classified errors, metadata hooks, and trace propagation — the operational contract
around it is: `Host.Drain` skips the gRPC component, `kit.Stopping` returns nil to a
gRPC stream, a scrape says nothing about RPCs, and generated code builds a bare
`grpc.NewServer()` with no credentials and no health service. This release closes
that gap where it can be closed and declares the difference where it cannot.

### Added

- `kit/grpc.Component` implements `kit.Draining`, so `Host.Drain` announces the stop
  to the gRPC server in the same reverse-attachment order it uses for everything
  else. Before this it type-asserted, found nothing, and skipped it silently.
- A gRPC handler reads the stopping signal with the same call an HTTP handler uses:
  `kit.Stopping(stream.Context())` closes when draining begins, so a stream can end
  itself inside the grace period. Unary and streaming interceptors carry it, ahead of
  trace extraction and ahead of the caller's own options, so a handler reached
  through any of them has it.
- `kit.WithStopping` is now exported: the seam a transport implements against.
  `kit`'s HTTP component uses it, the gRPC component uses it, and a custom transport
  that wants `kit.Stopping` to work for its handlers calls it with a channel it
  closes. `Stopping` still returns nil where nothing installed one, and receiving
  from a nil channel blocks forever, so one `select` is correct everywhere.
- Draining a gRPC component announces without refusing calls, which is deliberate: a
  client that got `UNAVAILABLE` during the drain delay would retry, possibly at this
  same instance, because the routing layer has not caught up. Failing readiness is
  the signal that moves traffic, and the gRPC health service already reports it.
  `Component.Shutdown` also announces, so a component stopped directly still tells
  its handlers, and it now bounds the wait itself rather than through grpc's
  `GracefulStop`: that call holds the server's mutex while it waits for handlers to
  return, and `Stop` needs the same mutex, so a handler that never returns made the
  pair deadlock — a bounded shutdown built on it was not bounded at all. The
  component counts calls through its own interceptors, closes the listener to stop new
  connections arriving, waits for the calls in flight until the budget expires, and
  then closes the transports, reporting how many it interrupted by wrapping
  `kit.ErrShutdownIncomplete` — the same error the HTTP component reports.
- `grpcserver.Observation`, `grpcserver.Recorder`, `grpcserver.RecorderFunc`,
  `grpcserver.RecordingUnaryInterceptor` and `grpcserver.RecordingStreamInterceptor`
  give the gRPC transport the recording contract the HTTP one has had. The label is
  the full method name, which needs none of the defending an HTTP route does: the set
  of methods is fixed by the service definition, and a call to a method that does not
  exist is refused before an interceptor runs, so traffic cannot add a series. A
  stream is recorded when it ends and carries `Stream: true`, because a stream's
  duration is a lifetime rather than a latency and averaging the two together
  produces a number that describes neither.
- `observability/metrics/grpc.Recorder` bridges those observations into the
  `endpoint.Metrics` collector the exposition endpoint renders, so a scrape of a
  gRPC-only service finally says something. It is a separate package on purpose, with
  its own dependency gate: `observability/metrics` stays reachable with the standard
  library alone, so an HTTP-only service does not acquire the gRPC libraries by asking
  to be scraped. A status code that describes the caller — `NotFound`,
  `InvalidArgument`, `PermissionDenied`, `Canceled` and their kin — is not an error,
  the same reasoning that keeps 4xx out of the HTTP error rate.
- A generated service's gRPC port is now configured like its first one. The
  certificate pair from 2.13.0 secures both listeners — one `grpc.Creds` built from
  the same `tls.Config`, so a mismatch still fails startup with its path. The
  standard `grpc.health.v1` service is registered and reports the readiness state the
  HTTP probes report, including `NOT_SERVING` the moment draining begins, so a
  gRPC-only deployment finally has something to point a probe at. Trace extraction
  and, when `server.metrics_path` is set, the recording interceptors are installed.
  And each server gets its own share of the shutdown budget instead of racing for one
  deadline: a slow HTTP drain used to leave gRPC nothing.
- The difference between the two transports is declared rather than discovered.
  `docs/lifecycle.md` gains a table of what each one answers — readiness, drain
  announcement, stopping signal, shutdown budget, TLS, metrics, tracing — including
  the honest rows: only HTTP can have a connection escape shutdown, and gRPC's health
  `Watch` is not implemented because the tools that orchestrate on it call `Check`.
  A gate holds it in place: compile-time assertions fail when a contract is dropped
  from either transport, and a test fails when `kit` declares an exported interface
  nobody has classified for both, with the reason recorded for each "no".

## [2.13.0] - 2026-09-07

A listener you can put on the internet. v2 serves plaintext HTTP and nothing else:
there is no TLS anywhere in the library or in generated code, so every deployment
terminates it somewhere else and no document says so. That is a defensible position
and an undeclared one, which is the part that gets a service deployed with an
assumption instead of a decision. This release makes the terminating listener
possible, states what it does and does not do about protocol negotiation, and pins
down what a hijacked connection means for a shutdown that promised to end.

### Added

- `kit.WithTLS`, `kit.WithTLSConfig`, `kit.DefaultTLSMinVersion`, and
  `HTTP.ServesTLS` let a component terminate TLS itself. A certificate that cannot be
  loaded fails construction with the offending path, rather than becoming a handshake
  error in somebody else's client log, and HTTP/2 arrives with TLS through ALPN — a
  service that turns TLS on changes protocol version at the same time, which is worth
  knowing before a streaming client finds out.
  Policy stays with the deployment: a supplied `tls.Config` is served as given, cipher
  suites, `ClientAuth`, SNI and `GetCertificate` included. The single imposed value is
  a zero `MinVersion` becoming TLS 1.2, because Go's zero value there means TLS 1.0
  for a server and no deployment means to ask for that. The config is cloned, so
  mutating the caller's value later does not change what is already being served.
  Plaintext remains the default, and is now a documented position rather than an
  assumption.
- Generated projects gained the `server.tls_cert_file` and `server.tls_key_file`
  configuration keys (`APP_TLS_CERT_FILE`, `APP_TLS_KEY_FILE`, both empty and off by
  default), and the generated entry point serves TLS when they are set. Both or
  neither: setting one fails validation instead of serving plaintext on the port a
  client is about to speak TLS to, and the pair is loaded before the listener serves,
  so a wrong path stops startup with the path in the message. The startup banner now
  reports the scheme it is actually serving rather than assuming `http`.
  `PRODUCTION.md` gained a TLS termination section saying when in-process termination
  is the right choice and when a proxy is, and what each implies for readiness,
  rotation (files are read once — a rotation means a restart unless you supply
  `GetCertificate`), health probe scheme, and upgraded connections.

### Declared

- A hijacked connection is not drained, and that is now stated beside the code that
  makes the promise it breaks. `http.Server.Shutdown` does not track a connection a
  handler took over, closing the listener does not close it, and the hard close added
  in 2.11.0 cannot reach it — so `Shutdown` returns `nil` promptly while the upgraded
  connection keeps carrying bytes. Milestone 11's promise still holds as written (no
  shutdown path returns while a connection *it owns* is open); what was missing was
  saying that a hijacked connection is not one of them. The seam is the handler that
  upgraded: it receives `kit.Stopping` like any other, which closes when draining
  begins, so it can end its own socket inside the grace period. Tests pin both halves,
  including the uncomfortable one.
- Protocol negotiation is stated instead of discovered. Over TLS, Go offers HTTP/2
  through ALPN: a flushing response — SSE, the streaming MCP transport — still streams,
  framed by the h2 stream layer rather than chunked transfer encoding, and a test now
  fails if that stops being true. What does not survive is the upgrade: h2 has no
  `101 Switching Protocols`, so a hijack-based protocol works only on the plaintext
  listener. Cleartext HTTP/2 is deliberately not offered — negotiating it needs a
  prior-knowledge client or an upgrade exchange, and where it is genuinely wanted the
  proxy in front already owns that decision.

## [2.12.0] - 2026-09-07

Numbers someone can act on. v2 can already push telemetry through OpenTelemetry,
but the pull model most deployments actually run — a scrape endpoint — was left to
each application to wire, which meant every service reported slightly different
numbers under slightly different names. This release gives the scrape surface a
seam, keeps the client library out of the core dependency path, and states what the
labels may contain, because an unbounded label is how a metrics endpoint takes down
the thing scraping it.

### Added

- Generated projects gained the `server.metrics_path` configuration key
  (`APP_METRICS_PATH`, empty and off by default), and the generated entry point wires
  it: per-route recording is installed at registration through
  `httpserver.DecorateRoutes`, and the exposition is mounted on the mux itself so a
  scrape reports on the service rather than on itself. Off by default because the
  endpoint publishes route names and traffic shape — put it on an admin listener or
  behind network policy. Custom routes in `cmd/custom_routes.go` are registered on the
  mux directly and are not recorded: that file is yours, and wrapping handlers you
  wrote is your call to make there.
- `httpserver.RouteRegistrar` and `httpserver.DecorateRoutes` name the place per-route

  middleware has to be installed: registration, where a handler and its route pattern
  are both in scope. A registrar takes the two methods `http.ServeMux` offers and
  nothing else, so `*http.ServeMux` satisfies it and a caller can pass a decorator
  instead — the handler the mux dispatches to is then the wrapped one, which is what
  makes `http.Request.Pattern` the matched route rather than empty. Middleware wrapped
  around a mux cannot see a pattern at all, which is the difference between per-route
  metrics and one series called `/`.
- `metrics.HTTPRecorder` bridges the HTTP transport's recorder to an
  `endpoint.Metrics` collector, so a service that wires only the HTTP layer — the
  generated projects have no endpoint chain around every handler — can feed the same
  numbers the exposition reads. It is a translation, not a second measurement: one
  request produces one observation. A request that matched no route is not recorded
  at all, so a vulnerability scan cannot appear as traffic the service served, and
  the outcome is the one the transport can honestly report — 5xx counts as an error,
  4xx does not, because a caller being told no is a working server.
- The pull and push paths are held to agreement by a test, not by intent: the same
  observations go through `endpoint.RecordingMiddleware` into both the exposition and
  the OpenTelemetry adapter, and the counts, the total duration, and the operation
  labels are compared. Where the two genuinely differ, the package documentation says
  so — counters here start at zero on restart because the collector is in memory, and
  duration is a sum and a count rather than buckets, so a p99 comes from the
  OpenTelemetry histogram and not from a scrape.
- Cardinality has a test rather than a caveat: fifty distinct URLs under one route
  pattern produce one series per outcome, no request path reaches a label, and a
  request that matched no route is not recorded at all — so scanning for `/.env`
  cannot grow the series count.

## [2.11.0] - 2026-09-07

Stopping on purpose. A process that is going away should say so before it goes,
finish what it accepted, and end its own grace period rather than return while a
connection it owns is still open. This release turns shutdown from one cancelled
context into a sequence with a declared order — and leaves what draining *means*
to the components that know.

### Added

- Generated projects gained the `server.drain_delay` configuration key
  (`APP_DRAIN_DELAY`, default `0s`), and their entry point now uses the same
  sequence the framework does: fail readiness, wait the drain delay, then stop. A
  graceful shutdown that runs out of budget closes the remaining connections
  instead of logging the deadline error and exiting with them open. The key is
  documented, validated (a negative delay fails startup), and covered by the gate
  that applies every documented `APP_*` key to a built binary.
- `kit.Host.Drain`, `kit.Draining`, `kit.WithDrainDelay`, `kit.Host.Draining`, and
  `kit.ErrDraining` make stopping a sequence: readiness starts failing and every
  attached `Draining` component is told, in reverse attachment order, before
  anything is torn down. `Run` then waits the configured drain delay — zero by
  default, which is the behaviour a Host had before — so whatever routes traffic
  here has time to re-read readiness or discovery before the listener closes.
  Liveness keeps passing while draining, because a process finishing in-flight work
  should not be restarted. `Shutdown` announces too, so a caller assembling a Host
  by hand cannot tear down a component that is still claiming to be ready. A
  component's `Drain` error is reported and the sequence continues: something that
  cannot stop accepting work still has to be shut down.
- `kit.Stopping`, `kit.ErrShutdownIncomplete`, and `kit.HTTP.Addr` end the grace
  period instead of hoping it ends. `Stopping(ctx)` closes when draining begins, so
  a stream or long poll ends its own response and the client sees the end of a
  stream rather than a broken connection; outside a kit HTTP component it is nil,
  which a `select` handles correctly. For a handler that watches nothing,
  `HTTP.Shutdown` no longer returns a deadline error with the connection still open:
  it cancels the request contexts, gives handlers a moment to unwind, closes what is
  left, and reports how many requests it interrupted through
  `ErrShutdownIncomplete`. `Addr` reports the address the listener actually bound,
  which is the only way to learn the port after binding `:0`.

### Changed

- A registration attached with `kit.WithRegistrar` now deregisters when the
  process announces the stop instead of when it tears down, so an instance leaves
  discovery before the drain delay rather than after it. Deregistering and then
  immediately closing the listener left every peer that had already cached the
  address talking to a closed port; the delay exists to cover that window, so the
  withdrawal has to come first. `Shutdown` still deregisters for a caller that
  skipped the announcement, and never twice.
- `Host.Shutdown` gives each component an equal share of the budget that is left
  instead of passing one shared deadline down the line. A shared deadline let the
  first component stopped spend all of it, and every component behind it was then
  handed a context that had already expired — a teardown that reads as graceful in
  the code and behaves as a hard close in production. The share is recomputed each
  time, so a component that returns early leaves more for the rest, and a caller
  that passed no deadline still has none imposed.

## [2.10.0] - 2026-09-07

Promises you can check. The compatibility contract stopped being prose: every
surface it covers names the gate that enforces it, and the protocol behaviours v2
promises are declared beside the code that keeps them. The first thing that check
showed is that the MCP promise was two revisions out of date, so this release
speaks the current specification as well as the one it froze.

### Added

- MCP 2026-07-28, the stateless revision, served beside 2025-06-18. The
  `MCP-Protocol-Version` header selects the request model per request; an absent
  header selects 2025-06-18, which predates the header being mandatory, so
  existing clients keep their behaviour for the twelve-month deprecation window.
  On the new revision no session is minted, read, or required: a request carries
  its protocol version, client identity and client capabilities in `params._meta`,
  reachable from a tool through `mcp.IdentityFromContext` and
  `mcp.ClientCapabilityFromContext`. `server/discover` replaces the handshake for
  a client that wants capabilities before it commits; `initialize` and
  `notifications/initialized` are answered as retired, naming it. POST is the
  whole transport there, so GET and DELETE — which existed for sessions — are
  answered 405 with `Allow: POST`.
- Header-based routing on 2026-07-28: `Mcp-Method` repeats the JSON-RPC method
  and `Mcp-Name` the target it addresses, so a gateway can route, meter and
  authorize without parsing a body. A request whose headers disagree with its body
  is refused with 400 `invalid_routing_header`, because a per-tool decision made
  on headers would otherwise apply to a different call than the one that runs.
- Cacheable catalogs: `tools/list`, `prompts/list`, `resources/list`,
  `resources/templates/list` and `resources/read` carry `ttlMs` and `cacheScope`
  on 2026-07-28, configured by `StreamableHandler.ListCacheTTL` and
  `ListCacheScope`. The scope defaults to `private`, because a catalog may be
  authorization-filtered.
- Multi Round-Trip Requests, so a tool can ask its caller something without a
  stream to ask over. A tool returns `interaction.InputRequired` with the
  questions and the state it wants echoed; the transport answers `resultType:
  "input_required"` with `inputRequests` and an opaque `requestState`, and the
  caller repeats the call with `inputResponses`. The tool runs again with
  `interaction.InputAnswersFromContext` returning the answers and state, which
  makes it a guard rather than a suspended call — nothing is held open between
  rounds. A question the caller never declared it can answer is refused with
  `-32021` and `data.requiredCapabilities` instead of asked. On 2025-06-18 the
  same return value is an error naming the revision that carries it.
- `interaction.EventToolInputRequired`, emitted for a call that stopped to ask
  something. An unfinished call is neither a result nor an error, and its log line
  says the call needs input at `Info`: a conversation working as designed should
  not page whoever watches error events.
- `StreamableHandler.RequestStateKey` and `RequestStateTTL` authenticate the
  `requestState` a mid-call question hands to the caller. With a key configured,
  a state that was edited or that has aged out is refused before a tool sees it,
  and verification is driven by the server's configuration rather than by what the
  token carries, so a caller cannot opt out by dropping the signature. Without a
  key the state is accepted as returned, which the documentation states plainly.
  Every instance serving the same address needs the same key.
- `mcp.MethodAuthorizer`, `mcp.MethodRequest`, `mcp.MethodAuthorizerFunc`, and
  `StreamableHandler.Authorizer` decide which callers may reach which MCP methods.
  Every request on both revisions — notifications included — reaches the
  authorizer with its method, its target name, the HTTP header, and the context
  the request is served under, before any registry, provider or tool is consulted;
  a refusal is `-32001`, or 403 for a request with no id to answer. The policy
  reads the principal from the context, so `mcp` keeps no opinion about how an
  identity is represented, and it never takes one from the request body. Nothing
  is authorized by default: `Authorizer` is nil, and what the framework owns is
  the shape of the question and of a refusal, not the answer. On 2025-06-18 the
  SSE stream and session deletion are not authorized per request — the session
  they act on was authorized when `initialize` created it.
- `mcp.Extension`, `mcp.ExtensionMethod`, `StreamableHandler.RegisterExtension`,
  `mcp.ClientExtensionFromContext`, and `mcp.MetaFromContext` implement the
  2026-07-28 extension framework without implementing any extension. A deployment
  declares its own — reverse-DNS id, its own version, its own configuration object,
  its own namespaced methods — and the transport declares it in capabilities and in
  `server/discover`, routes its methods on both revisions, and authorizes them like
  any other method. Extensions are off until one is registered; an unregistered
  extension's methods stay `-32601`, so a client sees a server that never had it
  and falls back to core protocol. The specification's `io.modelcontextprotocol/`
  namespace is refused to an application, a method that leaves its own namespace or
  takes over a core one is refused at registration, and an unrecognised
  `params._meta` key is neither refused nor discarded: it reaches the
  implementation, because it belongs to an extension someone else wrote.

### Changed

- The generated `main` applies every command-line flag before `Config.Validate`.
  `-auto-migrate` was written back afterwards, making it the one flag whose value
  never reached validation.
- The compatibility contract in `internal/docs/RELEASE.md` names a gate for each
  of its six surfaces, and `TestCompatibilityContractNamesItsGates` fails when a
  named gate stops existing. Forty-nine protocol behaviours are declared where
  they are implemented, as `// Stable: <id> — <promise>` beside the code, with the
  reviewed set in `tools/testdata/protocol_behaviour.txt`; two behaviours
  deliberately left unstable say so in the same form.
- The API compatibility gate tells an addition from a break instead of reporting
  both as "changed": a removed or reordered struct field and a new interface
  method fail, while a new struct field and a re-aligned constant block pass.
  Reordering is reported with its reason, because an unkeyed composite literal
  keeps compiling and assigns to a different field.
- [Licensing](docs/licenses.md) states what the MIT license covers and the license
  of every direct dependency a consumer inherits, with the two MPL-2.0 ones named
  and the packages that pull them in. `TestDependencyLicensesAreDocumented`
  classifies each license from the module's own license file and fails when the
  document and the module graph disagree, and `TestProjectLicenseIsOneText` keeps
  the repository and module copies of `LICENSE.txt` identical.
- Generated projects carry no licensing obligation. What `microgen` writes is the
  user's — no notice, no attribution, nothing flowing back, and no `LICENSE` file,
  so the choice stays theirs — while the framework the project imports stays MIT.
  The generated README says so, and `TestGeneratedOutputCarriesNoLicenseNotice`
  fails if a template starts emitting a notice or a license file appears in the
  reviewed generated layout.
- The TypeScript type-check gate tells a compiler that disagreed from a compiler
  that died. Type errors fail on the first attempt as before; a process that was
  killed or that failed inside its own runtime — tsc 7.0.2 exiting `0xC0000409`,
  for instance — is retried once and, if it dies again, reported as a crashed
  compiler rather than as a type error in the generated SDK.

## [2.9.0] - 2026-09-05

Layering. The goal is that the pieces be usable one at a time, so the contract
layer stops carrying a policy and the dependency direction is enforced rather
than described.

### Changed

- Measurement-driven balancing is assembled by `sd/feedback.Measure`, and the
  combinations that silently did nothing are gone. A `Measured` hands out
  `LeastRequest`, `Scored`, `SlowStartWeighted`, `Balancer` and `Ranking` over a
  `Table` it has already bound to a discovery subscription, and `Measured.Eject`
  joins that subscription instead of opening a second one that could report a
  different snapshot. `Table.Score`, `Table.Load` and `Table.FirstSeen` are
  removed, because a bare function was the one shape of them that did not work: a
  score handed to `selector.Scored` without `Table.Wrap` recorded nothing, so
  every instance scored the same and selection degraded to random, and a
  slow-start ramp over a table following nothing saw every instance as brand new
  forever and collapsed every weight to one. `Table.Scored` replaces the first as
  an already-wrapped strategy; `Table.LeastRequest`, `Table.Wrap`, `Table.Stats`
  and `feedback.Follow` are unchanged for a caller assembling the pieces itself.
  `sd/balancer` is unchanged and remains the layer for strategies that need no
  measurement — `NewScored` still takes an out-of-band ORCA or LRS style report
  and needs no table — which is why it has no `NewLeastRequest`.

- Feedback accounting follows registrations rather than health verdicts on its
  own. `sd.DerivedInstancer`, which `health.Checker` now implements, lets
  `feedback.Follow` and `Measure` resolve a filtered view to the Instancer it
  derives from. Handing the checked view to `Follow` used to cancel active and
  passive health checking against each other: a withdrawal is indistinguishable
  from a deregistration to a retainer, so the measurements that ejected an
  instance were dropped the moment probing withdrew it, and it returned with a
  clean record.

- The subscribe, error-grace, and invalidation state machine behind service
  discovery has one implementation. `sd/selector.Subscription`,
  `sd/endpointer.Cache` and `feedback.Follow` share it, `sortInstances` exists
  once instead of five times, and the etcd and consul providers broadcast through
  `sd/instance.Cache` rather than a private copy of it. No exported API changed
  and no behaviour changed; the reviewed API snapshot gained only the new
  internal package.

- An `sd.Registrar` states its conflict semantics. `sd.Conflict` names the three
  — `ConflictOverwrite`, `ConflictCreateOnly`, `ConflictCompareAndSwap` — and
  `etcd.ConflictRegistrarOptions` selects one; a refused registration returns an
  error wrapping the new `sd.ErrConflict`, and the etcd supervisor stops
  re-registering rather than retrying an identity another writer holds. Overwrite
  stays the default, because it is the only setting that recovers from a key an
  unclean exit left behind. Create-only and compare-and-swap are enforced by an
  etcd transaction: create-only writes while the key is absent, compare-and-swap
  writes while it is absent or still holds what this client wrote, so a lost lease
  is still recovered without stealing a key that has moved on. Consul supports
  overwrite alone and now documents that — its agent API upserts by service ID and
  cannot compare before it writes. `etcd.Client.Register` takes the semantics as a
  parameter, so a custom implementation of that interface needs the new argument.

- Generated code classifies a type mismatch instead of panicking on it. The
  endpoint adapters, gRPC codecs, gRPC server methods, and the SDK's gRPC client
  convert through `endpoint.TypedEndpoint.Wrap`, `endpoint.Unwrap`, or a checked
  assertion reporting `endpoint.NewTypeAssertError`. A middleware that replaced a
  response with another type — a cache layer, a fallback returning nil — used to
  panic inside a request handler, which read as a framework crash rather than as
  the wiring mistake it is. Regenerate to pick this up; hand-edited generated
  files keep their old behaviour.

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
