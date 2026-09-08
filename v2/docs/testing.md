# Testing

English | [简体中文](testing_zh.md)

Business logic is a plain function of `(context, Request) -> (Response, error)`,
so most tests need no server at all. Use the smallest test boundary that proves
the behavior.

## Test Selection

| What changed | First test |
| --- | --- |
| service rules or error kinds | call the service directly |
| endpoint middleware | call the built endpoint directly |
| HTTP decoding, status, headers | `httptest.NewServer` with `kit.HTTP` |
| generated project or SDK contract | `go test ./tools -run 'TestMicrogen'` |
| concurrency or lifecycle | focused `go test -race` |

Keep full generated-project and process smoke tests for integration or release
verification; they are intentionally slower.

## Unit-testing business logic

```go
func TestGreet_EmptyName(t *testing.T) {
	_, err := greet(context.Background(), GreetRequest{})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}
```

Classified errors are asserted through `apperror`:

```go
var appErr *apperror.Error
if !errors.As(err, &appErr) || appErr.ErrorKind() != apperror.KindInvalidArgument {
	t.Fatalf("expected invalid_argument, got %v", err)
}
```

## Testing the HTTP surface

`kit.HTTP` implements `http.Handler`, so `httptest.NewServer` serves it
without touching a port:

```go
func TestHTTP_Greet(t *testing.T) {
	svc := kit.MustNewHTTP(":0", kit.WithRequestID())
	kit.HandleJSONTyped(svc, "/greet", greet)

	srv := httptest.NewServer(svc)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/greet", "application/json",
		strings.NewReader(`{"name":"kit"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	// assert status, body, and headers
}
```

`MustNewHTTP` is the test constructor; it panics on invalid configuration,
which is exactly what a test wants.

## Testing middleware chains

Build the endpoint and call it directly:

```go
ep := endpoint.NewBuilder(base).WithValidation().Build()
if _, err := ep(context.Background(), invalidRequest); err == nil {
	t.Fatal("validation should reject the request")
}
```

Rejection errors are asserted with `errors.Is`:

```go
if !errors.Is(err, endpoint.ErrRateLimited) {
	t.Fatalf("expected rate limit rejection, got %v", err)
}
```

## Time is an input, not a wait

A test that sleeps is slow when it passes and flaky when the machine is loaded.
Where this framework decides *what time it is*, it reads a `endpoint.Clock`, and a
nil clock means the wall clock — so a service that does not care writes nothing:

```go
clock := endpoint.NewManualClock(time.Unix(0, 0))

ep := endpoint.NewBuilder(callDependency).
	WithRetry(3, endpoint.WithRetryClock(clock)).
	Build()

done := make(chan error, 1)
go func() { _, err := ep(ctx, request); done <- err }()

for clock.Pending() == 0 { // the code under test has reached its wait
	runtime.Gosched()
}
clock.Advance(time.Hour) // an hour of backoff elapses instantly
```

`ManualClock` is exported beside the contract, not hidden in a test-only package:
the point is that your own code can accept the same clock this framework's
components accept. `Advance` and `Set` deliver on every timer that becomes due,
`Pending` reports how many waits are outstanding, and it is safe to advance from
the test goroutine while the code under test reads it from others.

The seams that exist today:

- `endpoint.WithRetryClock` — the wait between retry attempts.
- `endpoint.Metrics.Clock` — the `LastRequestTime` a snapshot reports.
- `httpsecurity.CSRFConfig.Clock` — when a CSRF token is minted and when its TTL
  is checked. It is declared structurally there, because that package depends on
  nothing else in this framework; any value with a `Now` method fits, including
  `endpoint.ManualClock`.

What the clock deliberately does **not** decide is how long real work took.
`Observation.Duration` and every logged request duration are measured, because a
clock that could shorten them would make a recorder report something untrue about
the system. Assert on what a duration is derived from, not on the duration of a
call you did not actually make slow.

## Integration tests

An integration test starts the thing a deployment starts. Two boundaries are
worth the cost.

A generated project already carries a scaffold. `microgen -tests` writes one
`test/<service>_test.go` per service that constructs the service and calls every
method with a zero-valued request, once directly and once through
`LoggingMiddleware`. It proves the wiring compiles and returns; it asserts
nothing about your rules. That file is generator-owned and is rewritten on
regeneration, so real assertions belong in a file of your own:

```bash
go test ./...          # from the generated project root
```

Above that, run the built binary and drive it over the network. Wait for
readiness rather than sleeping: `/readyz` returns 503 until both listeners are
serving and 503 again as soon as shutdown begins, so polling it is the honest
gate. `/health` behaves the same way; `/livez` answers 200 while the process is
alive and says nothing about readiness. There is no `/healthz`.

Lifecycle and concurrency claims need the race detector, not a longer test:

```bash
go test -race -run 'TestShutdown|TestDrain' -count=1 ./...
```

## Reference patterns

The example tests are the canonical reference: `examples/quickstart`,
`examples/todosvc` (service, store, and HTTP layers), and `examples/auth`
(middleware and status codes) each demonstrate one layer of the request path.
