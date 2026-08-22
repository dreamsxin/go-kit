# Testing

English | [简体中文](testing_zh.md)

Business logic is a plain function of `(context, Request) -> (Response, error)`,
so most tests need no server at all. HTTP behavior is covered with
`httptest.NewServer`, which accepts a `kit.Service` directly.

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

`kit.Service` implements `http.Handler`, so `httptest.NewServer` serves it
without touching a port:

```go
func TestHTTP_Greet(t *testing.T) {
	svc := kit.MustNew(":0", kit.WithRequestID())
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

`MustNew` is the test constructor; it panics on invalid configuration, which
is exactly what a test wants.

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

## Reference patterns

The example tests are the canonical reference: `examples/quickstart`,
`examples/todosvc` (service, store, and HTTP layers), and `examples/auth`
(middleware and status codes) each demonstrate one layer of the request path.
