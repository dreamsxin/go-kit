package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/transport/http/client"
)

type todoRequest struct {
	ID      int64  `form:"id"       json:"id"`
	Keyword string `form:"keyword"  json:"keyword"`
	Title   string `json:"title"`
}

type todoResponse struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// TestTypedJSONClientPostBodyAndResponse: a non-GET typed call sends the
// struct as JSON (path placeholders filled) and returns Resp without a type
// assertion.
func TestTypedJSONClientPostBodyAndResponse(t *testing.T) {
	var gotBody todoRequest
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(todoResponse{ID: 7, Title: gotBody.Title})
	}))
	defer server.Close()

	call, err := client.NewTypedJSON[todoRequest, todoResponse](http.MethodPost, server.URL+"/todos/{id}", client.DefaultMaxJSONResponseBytes)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := call(context.Background(), todoRequest{ID: 7, Title: "write tests"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != 7 || resp.Title != "write tests" {
		t.Errorf("response = %+v", resp)
	}
	if gotPath != "/todos/7" {
		t.Errorf("path = %q, want the placeholder filled from the struct", gotPath)
	}
	// The whole struct is the body; the server's path decode wins on overlap,
	// so the client need not strip path values from the body.
	if gotBody.Title != "write tests" {
		t.Errorf("body = %+v, want the struct as JSON", gotBody)
	}
}

// TestTypedJSONClientGetQueryAndPath: a GET typed call fills the URL's path
// placeholders and query string from the struct.
func TestTypedJSONClientGetQueryAndPath(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(todoResponse{ID: 7})
	}))
	defer server.Close()

	call, err := client.NewTypedJSON[todoRequest, todoResponse](http.MethodGet, server.URL+"/todos/{id}", client.DefaultMaxJSONResponseBytes)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := call(context.Background(), todoRequest{ID: 7, Keyword: "urgent"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != 7 {
		t.Errorf("response = %+v", resp)
	}
	if !strings.Contains(gotQuery, "keyword=urgent") {
		t.Errorf("query = %q, want keyword=urgent", gotQuery)
	}
	if strings.Contains(gotQuery, "id=") {
		t.Errorf("query = %q; the path value must not be repeated as a query parameter", gotQuery)
	}
}

// TestTypedJSONClientSurfacesHTTPStatusError: a non-2xx answer surfaces as
// the classified error, not a decode failure.
func TestTypedJSONClientSurfacesHTTPStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"todo.not_found","message":"todo not found"}`))
	}))
	defer server.Close()

	call, err := client.NewTypedJSON[todoRequest, todoResponse](http.MethodGet, server.URL+"/todos/9", client.DefaultMaxJSONResponseBytes)
	if err != nil {
		t.Fatal(err)
	}
	_, err = call(context.Background(), todoRequest{ID: 9})
	var statusErr *client.HTTPStatusError
	if err == nil || !asHTTPStatusError(err, &statusErr) {
		t.Fatalf("error = %v, want HTTPStatusError", err)
	}
	if statusErr.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", statusErr.StatusCode)
	}
}

func asHTTPStatusError(err error, target **client.HTTPStatusError) bool {
	for err != nil {
		if e, ok := err.(*client.HTTPStatusError); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// TestTypedJSONClientNonStructBody: a map request is sent as the body with
// the URL left as written.
func TestTypedJSONClientNonStructBody(t *testing.T) {
	var gotBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
	}))
	defer server.Close()

	call, err := client.NewTypedJSON[map[string]string, map[string]string](http.MethodPost, server.URL+"/notes", client.DefaultMaxJSONResponseBytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call(context.Background(), map[string]string{"text": "hi"}); err != nil {
		t.Fatal(err)
	}
	if gotBody["text"] != "hi" {
		t.Errorf("body = %v", gotBody)
	}
}
