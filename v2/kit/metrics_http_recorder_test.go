package kit_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/kit"
	"github.com/dreamsxin/go-kit/v2/observability/metrics"
)

// TestHTTPRecorderFeedsTheExposition proves the assembly a service without an
// endpoint chain needs: kit.WithHTTPRecorder installs recording where the route
// pattern is readable, metrics.HTTPRecorder translates it into the collector, and
// the same collector answers the scrape. Raw handlers registered with Handle are
// not in the endpoint chain, so this is the only path that gives them series.
func TestHTTPRecorderFeedsTheExposition(t *testing.T) {
	collector := &endpoint.Metrics{}
	component := kit.MustNewHTTP("127.0.0.1:0",
		kit.WithHTTPRecorder(metrics.HTTPRecorder(collector)),
	)
	component.HandleFunc("GET /orders/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	component.HandleFunc("POST /orders", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	component.Handle("GET /metrics", metrics.Handler(collector))

	for _, id := range []string{"7", "8", "9"} {
		recorder := httptest.NewRecorder()
		component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/orders/"+id, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET /orders/%s = %d", id, recorder.Code)
		}
	}
	recorder := httptest.NewRecorder()
	component.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/orders", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("POST /orders = %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := recorder.Body.String()

	for _, want := range []string{
		`endpoint_requests_total{operation="GET /orders/{id}",outcome="success"} 3`,
		`endpoint_requests_total{operation="POST /orders",outcome="error"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("exposition is missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "/orders/7") {
		t.Errorf("a request path reached a label:\n%s", body)
	}
	// Probe and metrics routes are mounted outside recording, so orchestrator
	// traffic does not dominate every rate this measures.
	if strings.Contains(body, `operation="/metrics"`) || strings.Contains(body, "livez") {
		t.Errorf("infrastructure routes were recorded:\n%s", body)
	}
}
