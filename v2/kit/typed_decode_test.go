package kit_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/kit"
)

type idRequest struct {
	ID       int64  `form:"id"       json:"id"`
	Keyword  string `form:"keyword"  json:"keyword"`
	Username string `json:"username"`
}

// TestTypedRouteDecodesQueryAndPath: a GET route fills the request struct from
// the query string and the path values, which is what makes a typed route on a
// parameterized path possible without dropping to a raw handler.
func TestTypedRouteDecodesQueryAndPath(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")
	kit.HandleJSONTyped(component, "GET /todos/{id}", func(_ context.Context, req idRequest) (idRequest, error) {
		return req, nil
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/todos/7?keyword=urgent", nil)
	component.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET = %d: %s", recorder.Code, recorder.Body.String())
	}
	var got idRequest
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != 7 || got.Keyword != "urgent" {
		t.Errorf("request = %+v, want id 7 from the path and keyword urgent from the query", got)
	}
}

// TestTypedRoutePathWinsOverQuery: the path value names the resource being
// addressed; a query parameter of the same name must not override it.
func TestTypedRoutePathWinsOverQuery(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")
	kit.HandleJSONTyped(component, "GET /todos/{id}", func(_ context.Context, req idRequest) (idRequest, error) {
		return req, nil
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/todos/7?id=9", nil)
	component.ServeHTTP(recorder, request)

	var got idRequest
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != 7 {
		t.Errorf("id = %d, want 7: the path value must win over the query parameter", got.ID)
	}
}

// TestTypedRouteDecodesBodyAndPath: a non-GET route reads the strictly decoded
// JSON body and then the path values into the same struct.
func TestTypedRouteDecodesBodyAndPath(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")
	kit.HandleJSONTyped(component, "PUT /todos/{id}", func(_ context.Context, req idRequest) (idRequest, error) {
		return req, nil
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/todos/7", strings.NewReader(`{"username":"ada"}`))
	request.Header.Set("Content-Type", "application/json")
	component.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT = %d: %s", recorder.Code, recorder.Body.String())
	}
	var got idRequest
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != 7 || got.Username != "ada" {
		t.Errorf("request = %+v, want id 7 from the path and username from the body", got)
	}
}

// TestTypedRouteEmptyBodyIsZeroValue: a non-GET request that carries no body
// decodes to the zero value instead of answering 400, so a button that posts
// nothing reaches the handler and its validation answers in the application's
// own words.
func TestTypedRouteEmptyBodyIsZeroValue(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")
	kit.HandleJSONTyped(component, "POST /todos/{id}/done", func(_ context.Context, req idRequest) (idRequest, error) {
		if req.ID == 0 {
			return idRequest{}, apperror.New(apperror.KindInvalidArgument, "todo.id_required", "todo id is required")
		}
		return req, nil
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/todos/7/done", nil)
	request.Header.Set("Content-Type", "application/json") // declared but empty: still "nothing sent"
	component.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("POST without a body = %d: %s; want 200 with the path value decoded", recorder.Code, recorder.Body.String())
	}
	var got idRequest
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != 7 {
		t.Errorf("id = %d, want 7 from the path on a body-less request", got.ID)
	}
}

// TestTypedRouteStrictnessStays: filling from the URL did not soften the body
// rules. An unknown field is still 400, and a media type the route does not
// speak is still 415.
func TestTypedRouteStrictnessStays(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")
	kit.HandleJSONTyped(component, "POST /todos", func(_ context.Context, req idRequest) (idRequest, error) {
		return req, nil
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/todos", strings.NewReader(`{"unknown":true}`))
	request.Header.Set("Content-Type", "application/json")
	component.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("unknown field = %d, want 400", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/todos", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "text/plain")
	component.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Errorf("text/plain body = %d, want 415", recorder.Code)
	}
}
