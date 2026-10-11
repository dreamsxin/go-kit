package kit_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/kit"
)

// TestGroupPrefixesMethodPatterns: a "METHOD /path" pattern registered through
// a group keeps its method and gains the group's prefix.
func TestGroupPrefixesMethodPatterns(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")
	api := component.Group("/api")
	kit.HandleJSONTyped(api, "POST /todos", func(_ context.Context, _ struct{}) (string, error) {
		return "created", nil
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/todos", nil)
	component.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /api/todos = %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/todos", nil)
	component.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/todos = %d, want 405: the joined pattern must keep its method", recorder.Code)
	}
}

// TestGroupPrefixesBarePatterns: a bare "/path" pattern registered through a
// group stays bare, answering every method under the prefix.
func TestGroupPrefixesBarePatterns(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")
	api := component.Group("/api")
	kit.HandleJSONTyped(api, "/status", func(_ context.Context, _ struct{}) (string, error) {
		return "ok", nil
	})

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, "/api/status", nil)
		component.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Errorf("%s /api/status = %d, want 200: a bare pattern joined through a group answers every method", method, recorder.Code)
		}
	}
}

// TestNestedGroupsJoinPrefixes: a group of a group carries both prefixes, in
// the order they were cut.
func TestNestedGroupsJoinPrefixes(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")
	api := component.Group("/api")
	v1 := api.Group("/v1")
	kit.HandleJSONTyped(v1, "GET /todos", func(_ context.Context, _ struct{}) (string, error) {
		return "v1 list", nil
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/todos", nil)
	component.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/todos = %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/todos", nil)
	component.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Errorf("GET /api/todos = %d, want 404: the unprefixed route was never registered", recorder.Code)
	}
}

// TestGroupKeepsEndpointMiddleware: a route joined through a group is the
// component's route in every other respect — the component's endpoint
// middleware still sees it, and the recorders label it with the joined
// pattern.
func TestGroupKeepsEndpointMiddleware(t *testing.T) {
	var sawMiddleware bool
	var seenOperation string
	component := kit.MustNewHTTP("127.0.0.1:0",
		kit.WithEndpointMiddleware(func(next endpoint.Endpoint) endpoint.Endpoint {
			return func(ctx context.Context, request any) (any, error) {
				sawMiddleware = true
				return next(ctx, request)
			}
		}),
		kit.WithRecorder(endpoint.RecorderFunc(func(_ context.Context, obs endpoint.Observation) {
			seenOperation = obs.Operation
		})),
	)
	kit.HandleJSONTyped(component.Group("/api"), "POST /todos", func(_ context.Context, _ struct{}) (string, error) {
		return "created", nil
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/todos", nil)
	component.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /api/todos = %d: %s", recorder.Code, recorder.Body.String())
	}
	if !sawMiddleware {
		t.Error("the component's endpoint middleware did not run for a group-joined route")
	}
	if seenOperation != "POST /api/todos" {
		t.Errorf("recorder saw operation %q, want the joined pattern %q", seenOperation, "POST /api/todos")
	}
}

// TestGroupJoinPreservesPathParameters: a path parameter registered through a
// group keeps its braces and still fills the request struct.
func TestGroupJoinPreservesPathParameters(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")
	type idRequest struct {
		ID int64 `form:"id" json:"id"`
	}
	kit.HandleJSONTyped(component.Group("/api"), "GET /todos/{id}", func(_ context.Context, req idRequest) (idRequest, error) {
		return req, nil
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/todos/7", nil)
	component.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/todos/7 = %d: %s", recorder.Code, recorder.Body.String())
	}
	var got idRequest
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != 7 {
		t.Errorf("id = %d, want 7 through the group-joined pattern", got.ID)
	}
}

// TestGroupJoinPreservesSubtree: a trailing-slash subtree pattern registered
// through a group stays a subtree.
func TestGroupJoinPreservesSubtree(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")
	api := component.Group("/api")
	api.HandleFunc("/assets/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("asset"))
	})

	for _, path := range []string{"/api/assets/a.js", "/api/assets/deep/b.js"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		component.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || recorder.Body.String() != "asset" {
			t.Errorf("GET %s = %d %q, want the subtree to serve it", path, recorder.Code, recorder.Body.String())
		}
	}
}

// TestGroupRawHandlerKeepsComponentContext: a raw handler registered through a
// group still gets the component's HTTP context — the request ID and the
// WithTimeout deadline — because a group only joins patterns.
func TestGroupRawHandlerKeepsComponentContext(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0", kit.WithRequestID())
	component.Group("/api").HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.TrimSpace(r.Header.Get("X-Request-ID"))))
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	request.Header.Set("X-Request-ID", "group-test-id")
	component.ServeHTTP(recorder, request)
	if recorder.Body.String() != "group-test-id" {
		t.Errorf("request id echoed %q, want %q: a raw route through a group keeps the component context", recorder.Body.String(), "group-test-id")
	}
}
