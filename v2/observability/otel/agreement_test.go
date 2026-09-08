package oteladapter

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/sdk/metric"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	kitmetrics "github.com/dreamsxin/go-kit/v2/observability/metrics"
)

// TestMetricsAgreeWithTheExposition proves the two ways out of v2 report the same
// thing. Both the OpenTelemetry adapter and the scrape surface are fed by
// endpoint.RecordingMiddleware, so a dashboard built on the pushed series and an
// alert built on the scraped one describe the same traffic — and if that ever
// stops being true, this fails rather than a graph quietly disagreeing with a
// pager.
//
// The promise itself is declared on NewMetrics, in non-test source, because a
// marker in a _test.go file is one the behaviour gate never reviews.
func TestMetricsAgreeWithTheExposition(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	pushed, err := NewMetrics(provider.Meter("test"))
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}
	scraped := &endpoint.Metrics{}

	// One recording middleware, two recorders: this is the shape the promise is
	// about. Nothing here writes to one backend and not the other.
	ok := endpoint.RecordingMiddleware("GET /users", scraped, pushed)(
		func(context.Context, any) (any, error) { return "ok", nil })
	failing := endpoint.RecordingMiddleware("POST /users", scraped, pushed)(
		func(context.Context, any) (any, error) { return nil, errors.New("conflict") })

	for i := 0; i < 3; i++ {
		if _, err := ok(context.Background(), nil); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if _, err := failing(context.Background(), nil); err == nil {
		t.Fatal("the failing endpoint returned no error")
	}

	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	var exposition strings.Builder
	if err := kitmetrics.Write(&exposition, scraped); err != nil {
		t.Fatalf("Write: %v", err)
	}

	pushedCount := metricDataCount(data, "go_kit.endpoint.requests")
	scrapedCount := expositionSum(t, exposition.String(), "endpoint_requests_total{")
	if pushedCount != int64(scrapedCount) {
		t.Errorf("pushed %d requests, exposed %d", pushedCount, int64(scrapedCount))
	}
	if pushedCount != 4 {
		t.Errorf("pushed %d requests, want the 4 that were made", pushedCount)
	}

	// The duration is the same measurement in both paths, so the totals have to
	// match to within float rounding — the exposition prints microseconds.
	pushedSeconds := histogramSum(data, "go_kit.endpoint.duration")
	scrapedSeconds := expositionSum(t, exposition.String(), "endpoint_request_duration_seconds_sum{")
	if difference := pushedSeconds - scrapedSeconds; difference > 1e-5 || difference < -1e-5 {
		t.Errorf("pushed %v seconds, exposed %v seconds", pushedSeconds, scrapedSeconds)
	}

	// The operation labels have to agree too: same names, or one of the two is
	// grouping by something the other does not have.
	pushedOperations := attributeValues(data, "go_kit.endpoint.requests", "operation")
	for _, operation := range []string{"GET /users", "POST /users"} {
		if !pushedOperations[operation] {
			t.Errorf("pushed series is missing operation %q", operation)
		}
		if !strings.Contains(exposition.String(), `operation="`+operation+`"`) {
			t.Errorf("exposition is missing operation %q", operation)
		}
	}
}

// expositionSum adds up every sample whose line starts with prefix, which is how a
// scraper aggregates a labelled family.
func expositionSum(t *testing.T, exposition, prefix string) float64 {
	t.Helper()
	var total float64
	for _, line := range strings.Split(exposition, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		space := strings.LastIndex(line, " ")
		if space < 0 {
			t.Fatalf("malformed sample line %q", line)
		}
		value, err := strconv.ParseFloat(line[space+1:], 64)
		if err != nil {
			t.Fatalf("sample %q: %v", line, err)
		}
		total += value
	}
	return total
}

func histogramSum(data metricdata.ResourceMetrics, name string) float64 {
	for _, scope := range data.ScopeMetrics {
		for _, item := range scope.Metrics {
			if item.Name != name {
				continue
			}
			histogram, ok := item.Data.(metricdata.Histogram[float64])
			if !ok {
				continue
			}
			var total float64
			for _, point := range histogram.DataPoints {
				total += point.Sum
			}
			return total
		}
	}
	return 0
}

func attributeValues(data metricdata.ResourceMetrics, name, key string) map[string]bool {
	values := map[string]bool{}
	for _, scope := range data.ScopeMetrics {
		for _, item := range scope.Metrics {
			if item.Name != name {
				continue
			}
			sum, ok := item.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, point := range sum.DataPoints {
				iterator := point.Attributes.Iter()
				for iterator.Next() {
					attribute := iterator.Attribute()
					if string(attribute.Key) == key {
						values[attribute.Value.AsString()] = true
					}
				}
			}
		}
	}
	return values
}
