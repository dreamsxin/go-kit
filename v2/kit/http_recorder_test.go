package kit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// TestWithHTTPRecorderReportsRouteAndStatus is the property that makes response
// status alertable from metrics: the recorder sees the matched pattern and the
// status code, for a JSON route and a raw route alike, with no per-route wiring.
func TestWithHTTPRecorderReportsRouteAndStatus(t *testing.T) {
	recorder := &recordingHTTPRecorder{}
	service, err := NewHTTP(":0", WithHTTPRecorder(recorder))
	if err != nil {
		t.Fatalf("NewHTTP: %v", err)
	}
	HandleJSONTyped(service, "POST /users/{id}", func(context.Context, map[string]any) (map[string]any, error) {
		return map[string]any{"ok": true}, nil
	})
	service.HandleFunc("GET /raw", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	service.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/users/7", nil))
	service.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/raw", nil))

	observations := recorder.snapshot()
	if len(observations) != 2 {
		t.Fatalf("observations = %d, want 2", len(observations))
	}
	if observations[0].Route != "POST /users/{id}" {
		t.Fatalf("JSON route = %q, want %q", observations[0].Route, "POST /users/{id}")
	}
	if observations[0].Method != http.MethodPost || observations[0].Scheme != "http" {
		t.Fatalf("JSON observation = %#v", observations[0])
	}
	if observations[1].Route != "GET /raw" || observations[1].StatusCode != http.StatusTeapot {
		t.Fatalf("raw observation = %#v", observations[1])
	}
}

func TestWithHTTPRecorderRejectsNilRecorder(t *testing.T) {
	if _, err := NewHTTP(":0", WithHTTPRecorder(nil)); err == nil {
		t.Fatal("NewHTTP with a nil HTTP recorder returned no error")
	}
}

// TestWithHTTPRecorderSkipsProbeRoutes: orchestrator traffic is not application
// traffic. A liveness check every second would dominate every rate the recorder
// measures.
func TestWithHTTPRecorderSkipsProbeRoutes(t *testing.T) {
	recorder := &recordingHTTPRecorder{}
	service, err := NewHTTP(":0", WithHTTPRecorder(recorder))
	if err != nil {
		t.Fatalf("NewHTTP: %v", err)
	}
	response := httptest.NewRecorder()
	service.ServeHTTP(response, httptest.NewRequest(http.MethodGet, DefaultProbePaths().Liveness, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("probe status = %d, want 200", response.Code)
	}
	if observations := recorder.snapshot(); len(observations) != 0 {
		t.Fatalf("observations = %#v, want none", observations)
	}
}


type recordingHTTPRecorder struct {
	mu           sync.Mutex
	observations []httpserver.Observation
}

func (r *recordingHTTPRecorder) ObserveHTTP(_ context.Context, obs httpserver.Observation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.observations = append(r.observations, obs)
}

func (r *recordingHTTPRecorder) snapshot() []httpserver.Observation {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]httpserver.Observation(nil), r.observations...)
}
