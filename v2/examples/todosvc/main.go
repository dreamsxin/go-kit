// Package main demonstrates a complete database-backed CRUD service on the
// Service -> Endpoint -> Transport path: a SQLite repository, business logic
// that classifies failures with apperror, typed JSON endpoints for body
// requests, and raw handlers for path-parameter requests.
//
// The driver is modernc.org/sqlite, so the example builds and runs without
// CGO. The default database is in-memory; pass -db.dsn to persist.
//
// Concepts shown:
//   - todoStore owns every SQL statement and stays context-aware
//   - the service layer validates input and maps storage failures to
//     apperror kinds (invalid_argument, not_found)
//   - kit.HandleJSONTyped serves body-shaped requests through the endpoint
//     middleware chain; raw handlers cover path parameters
//   - kit.Lifecycle closes the database during graceful shutdown
//
// Run:
//
//	go run ./examples/todosvc
//
// Test with curl:
//
//	# Create a todo
//	curl -X POST http://localhost:8080/todos \
//	     -H "Content-Type: application/json" \
//	     -d '{"title":"write the CRUD example"}'
//
//	# List todos
//	curl http://localhost:8080/todos
//
//	# Get, complete, and delete one todo (replace 1 with the real id)
//	curl http://localhost:8080/todos/1
//	curl -X POST http://localhost:8080/todos/1/done
//	curl -i -X DELETE http://localhost:8080/todos/1
//
//	# Unknown id: 404 with a stable code
//	curl -i http://localhost:8080/todos/999
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/kit"

	_ "modernc.org/sqlite"
)

// ── Domain types (no framework or database dependency) ────────────────────────

// Todo is the stored domain object.
type Todo struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Done      bool   `json:"done"`
	CreatedAt string `json:"created_at"`
}

type createTodoRequest struct {
	Title string `json:"title"`
}

type todoList struct {
	Todos []Todo `json:"todos"`
}

// ── Storage layer ─────────────────────────────────────────────────────────────

// todoStore owns every SQL statement. All queries honor the request context
// so client cancellation reaches the database driver.
type todoStore struct {
	db *sql.DB
}

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

func (s *todoStore) init(ctx context.Context) error {
	const create = `CREATE TABLE IF NOT EXISTS todos (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		title      TEXT NOT NULL,
		done       INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL
	)`
	_, err := s.db.ExecContext(ctx, create)
	return err
}

func (s *todoStore) close() error { return s.db.Close() }

func (s *todoStore) create(ctx context.Context, title string, createdAt time.Time) (Todo, error) {
	const q = `INSERT INTO todos (title, created_at) VALUES (?, ?) RETURNING id`
	var id int64
	if err := s.db.QueryRowContext(ctx, q, title, createdAt.UTC().Format(time.RFC3339)).Scan(&id); err != nil {
		return Todo{}, err
	}
	return Todo{ID: id, Title: title, CreatedAt: createdAt.UTC().Format(time.RFC3339)}, nil
}

func (s *todoStore) get(ctx context.Context, id int64) (Todo, error) {
	const q = `SELECT id, title, done, created_at FROM todos WHERE id = ?`
	return s.scanOne(s.db.QueryRowContext(ctx, q, id))
}

func (s *todoStore) list(ctx context.Context) ([]Todo, error) {
	const q = `SELECT id, title, done, created_at FROM todos ORDER BY id`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var todos []Todo
	for rows.Next() {
		var t Todo
		var done int
		if err := rows.Scan(&t.ID, &t.Title, &done, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.Done = done != 0
		todos = append(todos, t)
	}
	return todos, rows.Err()
}

func (s *todoStore) markDone(ctx context.Context, id int64) (Todo, error) {
	const q = `UPDATE todos SET done = 1 WHERE id = ?`
	res, err := s.db.ExecContext(ctx, q, id)
	if err != nil {
		return Todo{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Todo{}, sql.ErrNoRows
	}
	return s.get(ctx, id)
}

func (s *todoStore) delete(ctx context.Context, id int64) error {
	const q = `DELETE FROM todos WHERE id = ?`
	res, err := s.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *todoStore) scanOne(row *sql.Row) (Todo, error) {
	var t Todo
	var done int
	if err := row.Scan(&t.ID, &t.Title, &done, &t.CreatedAt); err != nil {
		return Todo{}, err
	}
	t.Done = done != 0
	return t, nil
}

// ── Service layer: validation and error classification ───────────────────────

// todoService turns storage results into classified application errors, so
// every transport maps them to the right status code without knowing SQL.
type todoService struct {
	store *todoStore
	now   func() time.Time
}

func (svc todoService) Create(ctx context.Context, title string) (Todo, error) {
	if title == "" {
		return Todo{}, apperror.New(apperror.KindInvalidArgument, "todo.title_required", "title is required")
	}
	return svc.store.create(ctx, title, svc.now())
}

func (svc todoService) Get(ctx context.Context, id int64) (Todo, error) {
	todo, err := svc.store.get(ctx, id)
	return todo, classify(err)
}

func (svc todoService) List(ctx context.Context) (todoList, error) {
	todos, err := svc.store.list(ctx)
	if err != nil {
		return todoList{}, err
	}
	return todoList{Todos: todos}, nil
}

func (svc todoService) MarkDone(ctx context.Context, id int64) (Todo, error) {
	todo, err := svc.store.markDone(ctx, id)
	return todo, classify(err)
}

func (svc todoService) Delete(ctx context.Context, id int64) error {
	return classify(svc.store.delete(ctx, id))
}

func classify(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return apperror.New(apperror.KindNotFound, "todo.not_found", "todo not found")
	}
	return err
}

// ── HTTP layer ────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeServiceError maps classified application errors to HTTP statuses.
func writeServiceError(w http.ResponseWriter, err error) {
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		log.Printf("unclassified error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"code":    "internal",
			"message": "internal error",
		})
		return
	}
	status := http.StatusInternalServerError
	switch appErr.ErrorKind() {
	case apperror.KindInvalidArgument:
		status = http.StatusBadRequest
	case apperror.KindNotFound:
		status = http.StatusNotFound
	}
	writeJSON(w, status, map[string]string{
		"code":    appErr.ErrorCode(),
		"message": appErr.PublicMessage(),
	})
}

// storeLifecycle teaches kit to close the database during graceful shutdown.
type storeLifecycle struct {
	store *todoStore
	errs  chan error
}

func (l *storeLifecycle) Start() error                   { return nil }
func (l *storeLifecycle) Errors() <-chan error           { return l.errs }
func (l *storeLifecycle) Shutdown(context.Context) error { return l.store.close() }

func registerRoutes(svc *kit.HTTP, todos todoService) {
	// Body-shaped requests use the typed JSON path: service middleware
	// (request ID, endpoint middleware) and strict decoding apply. The
	// ServeMux method pattern keeps GET on the raw handler below.
	kit.HandleJSONTyped(svc, "POST /todos", func(ctx context.Context, req createTodoRequest) (Todo, error) {
		return todos.Create(ctx, req.Title)
	})

	svc.HandleFunc("GET /todos", func(w http.ResponseWriter, r *http.Request) {
		list, err := todos.List(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	})

	svc.HandleFunc("GET /todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		todo, err := todos.Get(r.Context(), id)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, todo)
	})

	svc.HandleFunc("DELETE /todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		if err := todos.Delete(r.Context(), id); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	svc.HandleFunc("POST /todos/{id}/done", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		todo, err := todos.MarkDone(r.Context(), id)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, todo)
	})
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "todo.invalid_id", "message": "todo id must be a positive integer"})
		return 0, false
	}
	return id, true
}

// ── Wire-up ───────────────────────────────────────────────────────────────────

func main() {
	httpAddr := flag.String("http.addr", ":8080", "HTTP listen address")
	dbDSN := flag.String("db.dsn", ":memory:", "SQLite DSN; :memory: keeps data for one process run")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := openTodoStore(ctx, *dbDSN)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	todos := todoService{store: store, now: time.Now}

	svc, err := kit.NewHTTP(*httpAddr,
		kit.WithRequestID(),
		kit.WithTimeout(5*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}

	registerRoutes(svc, todos)

	log.Println("todosvc example listening on", *httpAddr)

	host, err := kit.NewHost(kit.WithLifecycle(&storeLifecycle{store: store}, svc))
	if err != nil {
		log.Fatal(err)
	}
	if err := host.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
