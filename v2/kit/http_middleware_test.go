package kit

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWithHTTPMiddlewareAppliesToAllRoutesInOrder(t *testing.T) {
	var order []string
	middleware := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				order = append(order, name+":before")
				w.Header().Add("X-Middleware", name)
				next.ServeHTTP(w, request)
				order = append(order, name+":after")
			})
		}
	}
	service, err := New(":0", WithHTTPMiddleware(middleware("first"), middleware("second")))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	service.HandleFunc("GET /raw", func(w http.ResponseWriter, _ *http.Request) {
		order = append(order, "handler")
		w.WriteHeader(http.StatusNoContent)
	})

	for _, path := range []string{"/health", "/raw"} {
		order = nil
		recorder := httptest.NewRecorder()
		service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK && recorder.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d", path, recorder.Code)
		}
		if got := recorder.Header().Values("X-Middleware"); strings.Join(got, ",") != "first,second" {
			t.Fatalf("%s middleware headers = %v", path, got)
		}
		if path == "/raw" {
			if got := strings.Join(order, ","); got != "first:before,second:before,handler,second:after,first:after" {
				t.Fatalf("order = %s", got)
			}
		}
	}
}

func TestWithHTTPMiddlewareRejectsNil(t *testing.T) {
	if _, err := New(":0", WithHTTPMiddleware(nil)); err == nil {
		t.Fatal("expected nil HTTP middleware error")
	}
}
