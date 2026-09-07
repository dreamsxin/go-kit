package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/observability/metrics"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// serveWithRecording runs one request through a mux whose routes are wrapped in
// the bridge, which is where the route pattern is readable.
func serveWithRecording(t *testing.T, collector *endpoint.Metrics, method, target string, status int) {
	t.Helper()
	recording := httpserver.RecordingMiddleware(metrics.HTTPRecorder(collector))
	mux := http.NewServeMux()
	mux.Handle("GET /users/{id}", recording(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })))
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, target, nil))
}

// TestHTTPRecorderRecordsUnderTheRoutePattern proves the bridge labels by route,
// so a service that only wires the HTTP layer gets the same bounded series as one
// with an endpoint chain.
func TestHTTPRecorderRecordsUnderTheRoutePattern(t *testing.T) {
	collector := &endpoint.Metrics{}
	for _, id := range []string{"/users/1", "/users/2", "/users/999"} {
		serveWithRecording(t, collector, http.MethodGet, id, http.StatusOK)
	}

	if got := collector.Operations(); len(got) != 1 || got[0] != "GET /users/{id}" {
		t.Fatalf("operations = %v, want the route pattern alone", got)
	}
	if got := collector.SnapshotFor("GET /users/{id}").SuccessCount; got != 3 {
		t.Fatalf("success count = %d, want 3", got)
	}

	var exposition strings.Builder
	if err := metrics.Write(&exposition, collector); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if strings.Contains(exposition.String(), "/users/999") {
		t.Fatalf("a request path reached a label:\n%s", exposition.String())
	}
}

// TestHTTPRecorderCountsServerErrorsOnly proves the outcome the transport can
// honestly report: 5xx is this service failing, 4xx is the caller being told no.
// Counting rejections as errors makes every error-rate alert fire on client
// behaviour.
func TestHTTPRecorderCountsServerErrorsOnly(t *testing.T) {
	collector := &endpoint.Metrics{}
	serveWithRecording(t, collector, http.MethodGet, "/users/1", http.StatusOK)
	serveWithRecording(t, collector, http.MethodGet, "/users/2", http.StatusNotFound)
	serveWithRecording(t, collector, http.MethodGet, "/users/3", http.StatusUnprocessableEntity)
	serveWithRecording(t, collector, http.MethodGet, "/users/4", http.StatusInternalServerError)
	serveWithRecording(t, collector, http.MethodGet, "/users/5", http.StatusBadGateway)

	snapshot := collector.SnapshotFor("GET /users/{id}")
	if snapshot.RequestCount != 5 {
		t.Fatalf("request count = %d, want 5", snapshot.RequestCount)
	}
	if snapshot.ErrorCount != 2 {
		t.Errorf("error count = %d, want the two 5xx responses", snapshot.ErrorCount)
	}
	if snapshot.SuccessCount != 3 {
		t.Errorf("success count = %d, want 200 and both 4xx", snapshot.SuccessCount)
	}
}

// TestHTTPRecorderIgnoresUnroutedRequests proves a request that matched nothing is
// not recorded at all. Anything else turns a vulnerability scan into traffic the
// service appears to have served.
func TestHTTPRecorderIgnoresUnroutedRequests(t *testing.T) {
	collector := &endpoint.Metrics{}
	recording := httpserver.RecordingMiddleware(metrics.HTTPRecorder(collector))
	// No mux: the handler is called directly, so http.Request.Pattern is empty
	// exactly as it is for an unmatched request.
	handler := recording(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/.env", nil))

	if got := collector.Snapshot().RequestCount; got != 0 {
		t.Fatalf("recorded %d unrouted request(s), want 0", got)
	}
	if got := collector.Operations(); len(got) != 0 {
		t.Fatalf("operations = %v, want none", got)
	}
}

// TestHTTPRecorderRejectsANilCollector proves the misassembly is loud rather than
// a recorder that silently drops everything.
func TestHTTPRecorderRejectsANilCollector(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("HTTPRecorder(nil) did not panic")
		}
	}()
	metrics.HTTPRecorder(nil)
}
