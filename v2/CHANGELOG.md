# Changelog

English | [简体中文](CHANGELOG_zh.md)

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
