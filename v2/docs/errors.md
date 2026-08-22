# Error Handling

English | [简体中文](errors_zh.md)

go-kit v2 separates error classification (business) from error encoding
(transport). This page covers the whole flow: classification, automatic status
mapping, validation errors, and custom wire formats.

## Classifying business errors

`apperror` classifies failures without any transport types:

```go
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

## Automatic status mapping

The transports map kinds to statuses: HTTP 400/401/403/404/409/429/500 and the
matching gRPC codes. See the table in [core concepts](concepts.md#error-classification).

The default JSON error body is:

```json
{"code": "todo.not_found", "message": "todo not found", "request_id": "..."}
```

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
kit.New(":8080", kit.WithJSONServerOptions(
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

## Rules of thumb

- Business code classifies with `apperror`; transports map statuses; never
  leak internals to clients.
- Client errors (4xx) carry a public message; server errors (5xx) stay opaque.
- Retry policy belongs to the caller: only classified, idempotent failures
  should be retried.
