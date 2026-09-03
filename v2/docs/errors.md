# Error Handling

English | [简体中文](errors_zh.md)

go-kit v2 separates error classification (business) from error encoding
(transport). Use this page when an error crosses an HTTP or gRPC boundary.

## The Rule In One Minute

1. Return an `apperror` from service code; do not return HTTP or gRPC types.
2. Let the transport map the kind to a protocol status.
3. Use `PublicMessage` for text safe to expose. Keep `Error()` for diagnostics.
4. Built-in HTTP encoders redact status 500. `DefaultErrorEncoder` additionally
   lets an error implementing `json.Marshaler` own the complete body below 500;
   that explicit escape hatch bypasses `PublicMessage`.
5. Keep the status code meaningful. An envelope must not turn a failure into HTTP 200.

For a custom envelope, jump to [Custom error formats](#custom-error-formats). For
client retry behavior, see [Client-side classification](#client-side-classification).

## Classifying business errors

`apperror` classifies failures without any transport types:

```go
import "github.com/dreamsxin/go-kit/v2/apperror"

func (s todoService) Get(ctx context.Context, id int64) (Todo, error) {
	if id <= 0 {
		return Todo{}, apperror.New(
			apperror.KindInvalidArgument,
			"todo.invalid_id",
			"todo id must be a positive integer",
		)
	}
	todo, err := s.store.get(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Todo{}, apperror.New(
			apperror.KindNotFound,
			"todo.not_found",
			"todo not found",
		)
	}
	return todo, err
}
```

Every `apperror.Error` carries a stable machine-readable `code` and a public
`message`; both are safe to expose to clients. Unclassified errors return 500
without leaking internals.

### The `apperror` reference

The package is `github.com/dreamsxin/go-kit/v2/apperror`, part of the core
module with no extra dependency:

| API | Purpose |
| --- | --- |
| `New(kind, code, message)` | create a classified error with a stable code and a public message |
| `Wrap(kind, code, message, cause)` | same, preserving the underlying cause for `errors.Is/As` |
| `WrapCause(kind, code, cause)` | same with no public message, when the cause must stay internal |
| `Error.ErrorKind()` | the transport-neutral `Kind` |
| `Error.ErrorCode()` | the stable machine-readable code |
| `Error.PublicMessage()` | the message safe to show clients |

Every kind also has a same-named constructor — `NotFound(code, message)`,
`DeadlineExceeded(code, message)`, and so on — for shorter call sites.

Kinds and their transport mapping. This table is the single source for the
built-in mapping; the transports derive it from
`server.HTTPStatusForErrorKind` and `grpcserver.CodeForErrorKind`:

| Kind | HTTP status | gRPC code |
| --- | --- | --- |
| `KindInvalidArgument` | 400 | InvalidArgument |
| `KindUnauthenticated` | 401 | Unauthenticated |
| `KindPermissionDenied` | 403 | PermissionDenied |
| `KindNotFound` | 404 | NotFound |
| `KindAlreadyExists` | 409 | AlreadyExists |
| `KindConflict` | 409 | Aborted |
| `KindFailedPrecondition` | 412 | FailedPrecondition |
| `KindResourceExhausted` | 429 | ResourceExhausted |
| `KindCanceled` | 499 | Canceled |
| `KindInternal` | 500 | Internal |
| `KindUnimplemented` | 501 | Unimplemented |
| `KindUnavailable` | 503 | Unavailable |
| `KindDeadlineExceeded` | 504 | DeadlineExceeded |
| unclassified | 500 | Internal (opaque to clients) |

499 is the non-standard "Client Closed Request" status from nginx, also used by
gRPC-gateway. `server.StatusClientClosedRequest` names it. It keeps client
disconnects out of the 5xx rate.

The endpoint rejection errors classify themselves, so both transports derive
the same meaning from them:

| Rejection error | Kind | HTTP status |
| --- | --- | --- |
| `ErrRateLimited` | `KindResourceExhausted` | 429 |
| `ErrCircuitOpen` | `KindUnavailable` | 503 |
| `ErrBulkheadFull` | `KindUnavailable` | 503 |
| `ErrBackpressure` | `KindUnavailable` | 503 |

Rate limiting blames the caller's quota (429); a tripped breaker or a shed load
means the service or its dependency is unavailable (503), the same distinction
Envoy and Resilience4j make.

Unclassified context errors map like their kinds, so a timeout or a disconnect
reads the same whether the endpoint classified it or not:

- `context.DeadlineExceeded` — what `TimeoutMiddleware` surfaces — encodes as
  504, like `KindDeadlineExceeded`.
- `context.Canceled` encodes as 499, like `KindCanceled`.

Explicit classification wins: an `apperror` that wraps a context error keeps
its own kind.

### Retry hints

An error can tell the client how long to wait by implementing
`endpoint.RetryAfterReporter`. `endpoint.NewRetryAfterError(err, delay)` adds
the hint without changing the error's identity, so `errors.Is` and the
classification still work.

The HTTP encoders turn it into a `Retry-After` header (seconds, rounded up);
the gRPC encoder attaches `google.rpc.RetryInfo` to the status. A breaker
rejecting inside its open window reports the time left in that window
automatically; a rejection while half-open (another probe is already in flight)
carries no hint, because the window has already elapsed and there is nothing
honest to report — the caller falls back to its own backoff. A `RateLimiter`
that implements `RetryAfterReporter` has its delay attached to
`ErrRateLimited`.

`RetryMiddleware` reads the same contract in the other direction: when the
failure reports a hint, that hint replaces the local backoff schedule, capped by
`endpoint.MaxRetryAfterHint` so a hostile peer cannot park a caller that has no
deadline of its own.

### Client-side classification

A client call is an endpoint, so the same middleware has to understand its
errors. `client.HTTPStatusError` classifies itself: it implements
`apperror.Kinder` by mapping the response status back to a kind
(`client.KindForStatus`), and `RetryAfterReporter` by parsing the `Retry-After`
response header (both delay-seconds and HTTP-date forms). One consequence is
that `WithRetry` works on a client endpoint with no custom classifier:

```go
call, _ := client.NewJSONClient[UserResp](http.MethodGet, "https://api/users/1")
ep := endpoint.NewBuilder(call).
    WithTimeout(2 * time.Second).
    WithCircuitBreaker(breaker).
    WithRetry(3). // 408/429/5xx retried, other 4xx not, server Retry-After honored
    Build()
```

Translate deliberately before returning an upstream error to your own callers.
An unchanged upstream `KindNotFound` makes your handler answer 404 for a
dependency's missing record:

```go
if _, err := call(ctx, req); err != nil {
    return apperror.WrapCause(apperror.KindUnavailable, "upstream.users", err)
}
```

The gRPC client is symmetric: it wraps a failed call in `grpc.StatusError`,
which maps the code to a kind (`grpc.KindNameForCode`) and reports the
`google.rpc.RetryInfo` delay, while `status.FromError` keeps working.

`DefaultRetryable` also honors the optional `interface{ Retryable() bool }`
contract, which lets a custom client error decide for itself. Precedence is:
context errors and local admission-control rejections are never retried, then
`Retryable()`, then `apperror.KindUnavailable`.

## The stable code, not the status, identifies the failure

A status code is a coarse channel — a few dozen values that cannot carry
business meaning and cannot evolve. The stable application code does that job,
so every transport carries it out of band from the status:

- HTTP: in the body, as `{"code": "user.missing", ...}`.
- gRPC: in a `google.rpc.ErrorInfo` detail, as `reason`, following AIP-193.
  `NewErrorEncoder(WithErrorDomain("users.example.com"))` sets the `domain` that
  owns those codes.

Both client sides read it back, so the code survives a relay instead of
degrading into a status-derived name:

- `client.HTTPStatusError.ErrorCode()` parses the JSON body.
- `grpc.StatusError.ErrorCode()` reads the `ErrorInfo` reason, and
  `ErrorDomain()` its domain.

Because `HTTPStatusError` and `StatusError` satisfy
`transport/http.ErrorCoder`, an unchanged relay reproduces the upstream code:

```go
// upstream answers 404 {"code":"user.missing"}
// this service answers 404 {"code":"user.missing"} too
```

The upstream *message* is deliberately not relayed as a public message: it
belongs to another service's contract. Set your own with `PublicMessage`.

## Automatic status mapping

The transports map kinds to statuses with the table above; no business code
touches protocol types. `server.HTTPStatusForError` exposes the whole rule set
(including `StatusCoder` and unclassified context errors) so custom encoders
reuse it instead of duplicating it.

The default JSON error body is:

```json
{"code": "todo.not_found", "message": "todo not found", "request_id": "..."}
```

The `message` follows one rule in all three built-in encoders: a
`PublicMessager` wins; otherwise a status below 500 may use `err.Error()`; a 500
always answers `"Internal Server Error"`. 500 is where an unclassified error
lands, and an unclassified error is exactly the one whose text was never written
for a client — it may be wrapping a driver message or an upstream body. The error
still reaches the logs in full, and `request_id` ties the two together. A
deliberate 5xx set through `StatusCoder` keeps its message.

`DefaultErrorEncoder` has one explicit escape hatch: below 500, an error that
implements `json.Marshaler` replaces the entire response body and bypasses
`PublicMessage`. This is an application-owned wire contract; the application is
responsible for redaction and the resulting content type. The escape hatch never
opens at 500.

## Validation errors

Requests implementing `endpoint.Validatable` are validated before business
logic runs; field failures collect into `endpoint.ValidationError`, which
encodes as 400 with the stable code `bad_request.validation`:

```go
func (r CreateUserRequest) Validate() error {
	if r.Name == "" {
		return endpoint.NewValidationError("name", "is required")
	}
	return nil
}

ep := endpoint.NewBuilder(createUser).WithValidation().Build()
```

## Custom error formats

The JSON entry points accept a custom error encoder per route; with `kit`,
install one for every route at assembly:

```go
kit.NewHTTP(":8080", kit.WithJSONServerOptions(
	server.ServerErrorEncoder(myErrorEncoder),
))
```

The encoder receives the error and the `http.ResponseWriter`. A typical custom
format keeps classification but changes the envelope:

```go
func myErrorEncoder(_ context.Context, err error, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 500, "message": "internal error"})
		return
	}
	// Reuse the framework's classification instead of duplicating the mapping.
	status := server.HTTPStatusForError(err)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": status, "message": appErr.PublicMessage()})
}
```

The runnable walkthrough is [examples/envelope](../examples/README.md).

## Custom error kinds with custom statuses

The application can define its own `apperror.Kind` values (the kind is a
string). Register the status mapping once with
`JSONErrorEncoderWithKindMapper`; unknown kinds fall back to the built-in
mapping:

```go
svc, err := kit.NewHTTP(":8080", kit.WithJSONServerOptions(
	server.ServerErrorEncoder(server.JSONErrorEncoderWithKindMapper(func(k apperror.Kind) int {
		if k == "payment_failed" {
			return http.StatusPaymentRequired
		}
		return 0 // fall back to the built-in mapping
	})),
))
```

`server.HTTPStatusForErrorKind(kind)` exposes the built-in mapping when a
custom encoder wants to compose with it instead of replacing it.

The mapper resolves *kinds*, not statuses: an error that states its own status
through `transporthttp.StatusCoder` keeps it, exactly as with the default
encoder. That is what stops a mapper from silently rewriting a relayed
`client.HTTPStatusError` or an `endpoint.ValidationError`.

## Rules of thumb

- Business code classifies with `apperror`; transports map statuses; never
  leak internals to clients.
- Client errors (4xx) carry a public message. 500 never does — it is where
  unclassified failures land. A deliberate 501/503/504 carries one if the error
  states it through `PublicMessage`; `err.Error()` is never used at 5xx.
- Retry policy belongs to the caller: only classified, idempotent failures
  should be retried.
