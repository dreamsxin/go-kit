package kit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/kit"
	"github.com/dreamsxin/go-kit/v2/observability/metrics"
)

// The scrape surface is mounted by the application, not by kit: kit does not
// import observability/metrics, and the dependency gate keeps it that way. What
// these tests pin down is that mounting it with Handle behaves the way an
// operator needs — the route reports the service, and reading it does not change
// what it reports.

func metricsComponent(t *testing.T, path string) (*kit.HTTP, *endpoint.Metrics) {
	t.Helper()
	collector := &endpoint.Metrics{}
	component := kit.MustNewHTTP("127.0.0.1:0", kit.WithMetrics(collector))
	component.Handle("GET "+path, metrics.Handler(collector))
	return component, collector
}

// TestMountedMetricsEndpointServesWhatWasRecorded proves the wiring: a route call
// shows up in the exposition under its own pattern, which is what makes a scrape
// usable per route rather than only in aggregate.
func TestMountedMetricsEndpointServesWhatWasRecorded(t *testing.T) {
	component, _ := metricsComponent(t, "/metrics")
	kit.HandleJSONTyped(component, "POST /users", func(context.Context, map[string]string) (map[string]string, error) {
		return map[string]string{"ok": "true"}, nil
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"ada"}`))
	request.Header.Set("Content-Type", "application/json")
	component.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /users = %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /metrics = %d", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `endpoint_requests_total{operation="POST /users",outcome="success"} 1`) {
		t.Fatalf("the recorded route is missing from the exposition:\n%s", body)
	}
}

// TestScrapingDoesNotCountAsARequest proves the endpoint reports on the service
// rather than on itself. A raw route registered with Handle is deliberately
// outside the endpoint chain, so recording never sees it — a scrape that
// incremented the counters would make traffic look proportional to the scrape
// interval.
func TestScrapingDoesNotCountAsARequest(t *testing.T) {
	component, collector := metricsComponent(t, "/metrics")

	for i := 0; i < 3; i++ {
		recorder := httptest.NewRecorder()
		component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("scrape %d = %d", i, recorder.Code)
		}
	}
	if got := collector.Snapshot().RequestCount; got != 0 {
		t.Fatalf("scraping recorded %d request(s), want 0", got)
	}
}

// TestNoMetricsEndpointUnlessMounted proves nothing is exposed by default:
// putting route names and traffic shape on a listener is the application's
// decision, and kit makes it for nobody.
func TestNoMetricsEndpointUnlessMounted(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0", kit.WithMetrics(&endpoint.Metrics{}))
	recorder := httptest.NewRecorder()
	component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("GET /metrics = %d, want 404 when nothing was mounted", recorder.Code)
	}
}
