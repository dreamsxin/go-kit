package kit_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/kit"
	"github.com/dreamsxin/go-kit/v2/observability/metrics"
)

// TestSeriesCountIsBoundedByRoutesNotByRequests proves the bound that keeps a
// metrics endpoint from taking down the system scraping it: the operation label is
// the matched route pattern, so a thousand distinct URLs under one pattern are one
// series, and a caller cannot create a new one by inventing a path.
//
// This is the property to protect on every change to how operations are labelled.
// Cardinality that grows with request data does not fail here — it fails in
// production, in the storage the scraper writes to.
func TestSeriesCountIsBoundedByRoutesNotByRequests(t *testing.T) {
	collector := &endpoint.Metrics{}
	component := kit.MustNewHTTP("127.0.0.1:0", kit.WithMetrics(collector))
	component.Handle("GET /metrics", metrics.Handler(collector))
	kit.HandleJSONTyped(component, "POST /users/{id}/notes", func(context.Context, map[string]string) (map[string]string, error) {
		return map[string]string{"ok": "true"}, nil
	})

	for i := 0; i < 50; i++ {
		recorder := httptest.NewRecorder()
		path := fmt.Sprintf("/users/%d/notes", i)
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"text":"hi"}`))
		request.Header.Set("Content-Type", "application/json")
		component.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("POST %s = %d: %s", path, recorder.Code, recorder.Body.String())
		}
	}

	recorder := httptest.NewRecorder()
	component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := recorder.Body.String()

	if !strings.Contains(body, `endpoint_requests_total{operation="POST /users/{id}/notes",outcome="success"} 50`) {
		t.Fatalf("the pattern series is missing or wrong:\n%s", body)
	}
	// One route, one series per outcome — not one per id.
	if got := strings.Count(body, "endpoint_requests_total{"); got != 2 {
		t.Fatalf("requests_total series = %d, want 2 for a single route:\n%s", got, body)
	}
	for i := 0; i < 50; i++ {
		if id := fmt.Sprintf("/users/%d/notes", i); strings.Contains(body, id) {
			t.Fatalf("the exposition contains the request path %s: cardinality is unbounded", id)
		}
	}
}

// TestUnroutedRequestsAddNoSeries proves the other half of the bound: a request
// that matched nothing is not recorded at all, so scanning for URLs cannot inflate
// the series count either.
func TestUnroutedRequestsAddNoSeries(t *testing.T) {
	collector := &endpoint.Metrics{}
	component := kit.MustNewHTTP("127.0.0.1:0", kit.WithMetrics(collector))
	component.Handle("GET /metrics", metrics.Handler(collector))

	for _, path := range []string{"/admin", "/.env", "/wp-login.php", "/../../etc/passwd"} {
		recorder := httptest.NewRecorder()
		component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	}

	if got := collector.Operations(); len(got) != 0 {
		t.Fatalf("operations = %v, want none: unrouted requests must not become series", got)
	}
	recorder := httptest.NewRecorder()
	component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if body := recorder.Body.String(); strings.Contains(body, "wp-login") {
		t.Fatalf("a probe path reached a label:\n%s", body)
	}
}
