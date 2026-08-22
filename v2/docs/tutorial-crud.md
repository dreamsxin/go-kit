# Tutorial: A CRUD Service

English | [简体中文](tutorial-crud_zh.md)

This tutorial builds a database-backed todo service from zero: SQLite storage,
typed JSON routes, classified errors, and graceful shutdown. The complete code
is the runnable [examples/todosvc](../examples/README.md) example.

## 1. The goal

A service with five routes:

| Method | Path | Action |
| --- | --- | --- |
| `POST` | `/todos` | create a todo |
| `GET` | `/todos` | list todos |
| `GET` | `/todos/{id}` | get one todo |
| `POST` | `/todos/{id}/done` | mark a todo done |
| `DELETE` | `/todos/{id}` | delete a todo |

## 2. The storage layer

The repository owns every SQL statement and stays context-aware, so request
cancellation reaches the database:

```go
type todoStore struct{ db *sql.DB }

func openTodoStore(ctx context.Context, dsn string) (*todoStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	store := &todoStore{db: db}
	if err := store.init(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}
```

`init` creates the table when it is missing. The driver is
`modernc.org/sqlite`, so the service builds without CGO.

## 3. The service layer

The service validates input and maps storage failures to `apperror` kinds;
the transport never sees SQL:

```go
func (svc todoService) Create(ctx context.Context, title string) (Todo, error) {
	if title == "" {
		return Todo{}, apperror.New(
			apperror.KindInvalidArgument, "todo.title_required", "title is required",
		)
	}
	return svc.store.create(ctx, title, svc.now())
}
```

A missing row becomes `KindNotFound`, which the transport encodes as 404.

## 4. The transport layer

Body-shaped requests use the typed JSON path (strict decoding, middleware,
envelope). Path parameters use raw handlers, because a JSON body cannot carry
them:

```go
kit.HandleJSONTyped(svc, "POST /todos", func(ctx context.Context, req createTodoRequest) (Todo, error) {
	return todos.Create(ctx, req.Title)
})

svc.HandleFunc("GET /todos/{id}", func(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	todo, err := todos.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err) // apperror kind -> HTTP status
		return
	}
	writeJSON(w, http.StatusOK, todo)
})
```

## 5. Assembly and shutdown

The store opens before the service starts and closes during graceful shutdown
through a `kit.Lifecycle` component:

```go
store, err := openTodoStore(ctx, *dbDSN)
if err != nil {
	log.Fatalf("open database: %v", err)
}
svc, err := kit.New(*httpAddr,
	kit.WithRequestID(),
	kit.WithTimeout(5*time.Second),
	kit.WithLifecycle(&storeLifecycle{store: store}),
)
```

## 6. Run it

```bash
go run ./examples/todosvc

curl -X POST http://localhost:8080/todos -d '{"title":"write the tutorial"}'
curl http://localhost:8080/todos
curl -X POST http://localhost:8080/todos/1/done
curl -i -X DELETE http://localhost:8080/todos/1
curl -i http://localhost:8080/todos/999   # 404 with a stable code
```

## Where to go next

- [error handling](errors.md) for custom error envelopes
- [lifecycle](lifecycle.md) for background jobs beside the HTTP server
- [tutorial: generating a service](tutorial-microgen.md) to generate this
  project shape from an IDL instead
