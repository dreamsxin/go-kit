package oteladapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

func TestHTTPMetricsCarryRouteAndStatus(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	metrics, err := NewHTTPMetrics(provider.Meter("test"))
	if err != nil {
		t.Fatalf("NewHTTPMetrics: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /users/{id}", httpserver.RecordingMiddleware(metrics)(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
	)))
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/users/7", nil))

	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	histogram, ok := histogramFor(data, "http.server.request.duration")
	if !ok {
		t.Fatal("http.server.request.duration was not recorded")
	}
	if histogram.unit != "s" {
		t.Fatalf("unit = %q, want %q", histogram.unit, "s")
	}
	// Without http.route the series is either unbounded or anonymous; without
	// http.response.status_code a 500 rate cannot be alerted on from metrics.
	if got := histogram.attribute("http.route"); got != "GET /users/{id}" {
		t.Fatalf("http.route = %q, want %q", got, "GET /users/{id}")
	}
	if got := histogram.attribute("http.response.status_code"); got != "500" {
		t.Fatalf("http.response.status_code = %q, want %q", got, "500")
	}
	if got := histogram.attribute("http.request.method"); got != "GET" {
		t.Fatalf("http.request.method = %q, want %q", got, "GET")
	}
	if got := histogram.attribute("url.scheme"); got != "http" {
		t.Fatalf("url.scheme = %q, want %q", got, "http")
	}
	if got := histogram.attribute("error.type"); got != "500" {
		t.Fatalf("error.type = %q, want %q", got, "500")
	}
}

func TestHTTPMetricsOmitRouteForUnmatchedRequests(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	metrics, err := NewHTTPMetrics(provider.Meter("test"))
	if err != nil {
		t.Fatalf("NewHTTPMetrics: %v", err)
	}
	handler := httpserver.RecordingMiddleware(metrics)(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) },
	))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/unrouted/42", nil))

	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	histogram, ok := histogramFor(data, "http.server.request.duration")
	if !ok {
		t.Fatal("http.server.request.duration was not recorded")
	}
	if got, found := histogram.lookup("http.route"); found {
		t.Fatalf("http.route = %q, want absent", got)
	}
	// A 4xx is the client's error, not the server's, so it carries no
	// error.type.
	if got, found := histogram.lookup("error.type"); found {
		t.Fatalf("error.type = %q, want absent", got)
	}
}

type histogramPoint struct {
	unit  string
	point metricdata.HistogramDataPoint[float64]
}

func (h histogramPoint) lookup(key string) (string, bool) {
	value, found := h.point.Attributes.Value(attribute.Key(key))
	if !found {
		return "", false
	}
	return value.Emit(), true
}

func (h histogramPoint) attribute(key string) string {
	value, _ := h.lookup(key)
	return value
}

func histogramFor(data metricdata.ResourceMetrics, name string) (histogramPoint, bool) {
	for _, scope := range data.ScopeMetrics {
		for _, item := range scope.Metrics {
			if item.Name != name {
				continue
			}
			histogram, ok := item.Data.(metricdata.Histogram[float64])
			if !ok || len(histogram.DataPoints) == 0 {
				return histogramPoint{}, false
			}
			return histogramPoint{unit: item.Unit, point: histogram.DataPoints[0]}, true
		}
	}
	return histogramPoint{}, false
}
