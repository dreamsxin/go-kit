package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/kit"
)

// errorKind returns the apperror kind of err, or "" when unclassified.
func errorKind(err error) apperror.Kind {
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		return appErr.ErrorKind()
	}
	return ""
}

func newTodoServer(t *testing.T) *httptest.Server {
	t.Helper()
	store, err := openTodoStore(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.close() })

	svc := kit.MustNewHTTP(":0", kit.WithRequestID(), kit.WithTimeout(5*time.Second))
	registerRoutes(svc, todoService{store: store, now: time.Now})
	return httptest.NewServer(svc)
}

func TestServiceCRUDFlow(t *testing.T) {
	store, err := openTodoStore(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.close()
	svc := todoService{store: store, now: time.Now}
	ctx := context.Background()

	created, err := svc.Create(ctx, "first")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 || created.Title != "first" {
		t.Fatalf("created todo: %+v", created)
	}

	fetched, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if fetched.Title != "first" || fetched.Done {
		t.Fatalf("fetched todo: %+v", fetched)
	}

	done, err := svc.MarkDone(ctx, created.ID)
	if err != nil {
		t.Fatalf("mark done: %v", err)
	}
	if !done.Done {
		t.Fatalf("todo should be done: %+v", done)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Todos) != 1 {
		t.Fatalf("list length: got %d, want 1", len(list.Todos))
	}

	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Get(ctx, created.ID); errorKind(err) != apperror.KindNotFound {
		t.Fatalf("get after delete: want not_found, got %v", err)
	}
}

func TestCreateValidation(t *testing.T) {
	store, err := openTodoStore(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.close()
	svc := todoService{store: store, now: time.Now}

	if _, err := svc.Create(context.Background(), ""); errorKind(err) != apperror.KindInvalidArgument {
		t.Fatalf("empty title: want invalid_argument, got %v", err)
	}
}

func TestHTTPCreateListDoneDelete(t *testing.T) {
	srv := newTodoServer(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/todos", "application/json", bytes.NewReader([]byte(`{"title":"write tests"}`)))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create status: got %d, want 200", resp.StatusCode)
	}
	var created Todo
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Title != "write tests" || created.ID == 0 {
		t.Fatalf("created: %+v", created)
	}

	// List contains the created todo.
	listResp, err := http.Get(srv.URL + "/todos")
	if err != nil {
		t.Fatal(err)
	}
	defer listResp.Body.Close()
	var list todoList
	if err := json.NewDecoder(listResp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Todos) != 1 || list.Todos[0].Title != "write tests" {
		t.Fatalf("list: %+v", list)
	}

	// Mark done.
	doneURL := fmt.Sprintf("%s/todos/%d/done", srv.URL, created.ID)
	doneResp, err := http.Post(doneURL, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer doneResp.Body.Close()
	var done Todo
	if err := json.NewDecoder(doneResp.Body).Decode(&done); err != nil {
		t.Fatal(err)
	}
	if !done.Done {
		t.Fatalf("done todo: %+v", done)
	}

	// Delete returns 204.
	delResp, err := http.NewRequest(http.MethodDelete,
		fmt.Sprintf("%s/todos/%d", srv.URL, created.ID), nil)
	if err != nil {
		t.Fatal(err)
	}
	delResult, err := http.DefaultClient.Do(delResp)
	if err != nil {
		t.Fatal(err)
	}
	defer delResult.Body.Close()
	if delResult.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status: got %d, want 204", delResult.StatusCode)
	}
}

func TestHTTPUnknownTodoIs404(t *testing.T) {
	srv := newTodoServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/todos/999")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", resp.StatusCode)
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "todo.not_found" {
		t.Fatalf("code: got %q, want todo.not_found", body.Code)
	}
}
