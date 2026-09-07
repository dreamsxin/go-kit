package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// TestDecorateRoutesRecordsTheMatchedRoute proves why the seam exists: decoration
// at registration puts the middleware inside the mux, so the request it sees is
// the one the mux matched and http.Request.Pattern is the route. Wrapping the mux
// instead records every request with an empty route.
func TestDecorateRoutesRecordsTheMatchedRoute(t *testing.T) {
	var seen []httpserver.Observation
	recorder := httpserver.RecorderFunc(func(_ context.Context, obs httpserver.Observation) {
		seen = append(seen, obs)
	})

	mux := http.NewServeMux()
	registrar := httpserver.DecorateRoutes(mux, httpserver.RecordingMiddleware(recorder))
	registrar.Handle("GET /users/{id}", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	registrar.HandleFunc("POST /users", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/users/7", nil))
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/users", nil))

	if len(seen) != 2 {
		t.Fatalf("observations = %d, want 2", len(seen))
	}
	if seen[0].Route != "GET /users/{id}" || seen[0].StatusCode != http.StatusOK {
		t.Errorf("first observation = %+v", seen[0])
	}
	if seen[1].Route != "POST /users" || seen[1].StatusCode != http.StatusCreated {
		t.Errorf("second observation = %+v", seen[1])
	}
}

// TestDecorateRoutesWithoutMiddlewareChangesNothing proves the decorator is not a
// layer for its own sake: with nothing to apply, the caller gets its registrar
// back.
func TestDecorateRoutesWithoutMiddlewareChangesNothing(t *testing.T) {
	mux := http.NewServeMux()
	if got := httpserver.DecorateRoutes(mux, nil); got != httpserver.RouteRegistrar(mux) {
		t.Fatalf("DecorateRoutes(mux, nil) = %v, want the mux itself", got)
	}
}

// TestDecorateRoutesRejectsANilRegistrar proves the misassembly is loud. Dropping
// every registration silently would look like a service with no endpoints.
func TestDecorateRoutesRejectsANilRegistrar(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("DecorateRoutes(nil, ...) did not panic")
		}
	}()
	httpserver.DecorateRoutes(nil, func(next http.Handler) http.Handler { return next })
}
