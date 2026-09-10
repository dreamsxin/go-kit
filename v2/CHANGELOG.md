# Changelog

English | [简体中文](CHANGELOG_zh.md)

## [2.22.0] - Release Candidate

The service-discovery subtree had never been audited. Two audits of it — the load
balancing path and the resilience path — opened this milestone.

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
