package kit_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/kit"
)

// TestGroupPrefixesMethodPatterns: a "METHOD /path" pattern registered through
// a group keeps its method and gains the group's prefix.
func TestGroupPrefixesMethodPatterns(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")
	api := component.Group("/api")
	kit.HandleJSON(api, "POST /todos", func(_ context.Context, _ struct{}) (string, error) {
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
	kit.HandleJSON(api, "/status", func(_ context.Context, _ struct{}) (string, error) {
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
	kit.HandleJSON(v1, "GET /todos", func(_ context.Context, _ struct{}) (string, error) {
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
	kit.HandleJSON(component.Group("/api"), "POST /todos", func(_ context.Context, _ struct{}) (string, error) {
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
	kit.HandleJSON(component.Group("/api"), "GET /todos/{id}", func(_ context.Context, req idRequest) (idRequest, error) {
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

// TestGroupEndpointMiddlewareAppliesToGroupRoutes: the group's endpoint
// middleware governs routes registered through the group, and the
// component's chain still wraps it.
func TestGroupEndpointMiddlewareAppliesToGroupRoutes(t *testing.T) {
	var order []string
	component := kit.MustNewHTTP("127.0.0.1:0",
		kit.WithEndpointMiddleware(func(next endpoint.Endpoint) endpoint.Endpoint {
			return func(ctx context.Context, request any) (any, error) {
				order = append(order, "component")
				return next(ctx, request)
			}
		}),
	)
	api := component.Group("/api")
	api.UseEndpointMiddleware(func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			order = append(order, "group")
			return next(ctx, request)
		}
	})
	kit.HandleJSON(api, "POST /todos", func(_ context.Context, _ struct{}) (string, error) {
		return "created", nil
	})
	kit.HandleJSON(component, "POST /open", func(_ context.Context, _ struct{}) (string, error) {
		return "open", nil
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/todos", nil)
	component.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("group route = %d: %s", recorder.Code, recorder.Body.String())
	}
	if strings.Join(order, ">") != "component>group" {
		t.Errorf("chain = %v, want component outside group", order)
	}

	order = nil
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/open", nil)
	component.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("component route = %d", recorder.Code)
	}
	if len(order) != 1 || order[0] != "component" {
		t.Errorf("component route chain = %v, want only the component middleware", order)
	}
}

// TestGroupEndpointMiddlewareRunsInsideComponentChain: a group endpoint
// middleware that rejects runs inside the component's recorders, so the
// rejection is still observed with the joined pattern.
func TestGroupEndpointMiddlewareRunsInsideComponentChain(t *testing.T) {
	var recorded bool
	component := kit.MustNewHTTP("127.0.0.1:0",
		kit.WithRecorder(endpoint.RecorderFunc(func(_ context.Context, obs endpoint.Observation) {
			recorded = true
		})),
	)
	api := component.Group("/api")
	api.UseEndpointMiddleware(func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			return nil, apperror.New(apperror.KindPermissionDenied, "test.denied", "denied")
		}
	})
	kit.HandleJSON(api, "POST /todos", func(_ context.Context, _ struct{}) (string, error) {
		return "created", nil
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/todos", nil)
	component.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("rejected route = %d: %s", recorder.Code, recorder.Body.String())
	}
	if !recorded {
		t.Error("the recorder did not observe the group middleware's rejection")
	}
}

// TestGroupHTTPMiddlewareWrapsGroupHandlers: the group's HTTP middleware
// wraps the handlers registered through the group, inside the component's
// HTTP middleware, and never touches routes outside the group.
func TestGroupHTTPMiddlewareWrapsGroupHandlers(t *testing.T) {
	var order []string
	component := kit.MustNewHTTP("127.0.0.1:0",
		kit.WithHTTPMiddleware(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "component")
				next.ServeHTTP(w, r)
			})
		}),
	)
	api := component.Group("/api")
	api.UseHTTPMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "group")
			next.ServeHTTP(w, r)
		})
	})
	api.HandleFunc("GET /things", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("things"))
	})
	component.HandleFunc("GET /open", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("open"))
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/things", nil)
	component.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("group route = %d", recorder.Code)
	}
	if strings.Join(order, ">") != "component>group" {
		t.Errorf("chain = %v, want component outside group", order)
	}

	order = nil
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/open", nil)
	component.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("component route = %d", recorder.Code)
	}
	if len(order) != 1 || order[0] != "component" {
		t.Errorf("component route chain = %v, want only the component middleware", order)
	}
}

// TestNestedGroupInheritsMiddleware: a child group's routes carry the
// parent's middleware without re-declaring it.
func TestNestedGroupInheritsMiddleware(t *testing.T) {
	var hits int
	component := kit.MustNewHTTP("127.0.0.1:0")
	api := component.Group("/api")
	api.UseEndpointMiddleware(func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (any, error) {
			hits++
			return next(ctx, request)
		}
	})
	v1 := api.Group("/v1")
	kit.HandleJSON(v1, "GET /todos", func(_ context.Context, _ struct{}) (string, error) {
		return "v1 list", nil
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/todos", nil)
	component.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("nested route = %d: %s", recorder.Code, recorder.Body.String())
	}
	if hits != 1 {
		t.Errorf("parent middleware ran %d times, want 1", hits)
	}
}
