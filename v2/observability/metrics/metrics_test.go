package metrics_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/observability/metrics"
)

func recorded(t *testing.T) *endpoint.Metrics {
	t.Helper()
	collector := &endpoint.Metrics{}
	ctx := context.Background()
	collector.Observe(ctx, endpoint.Observation{Operation: "GET /users", Duration: 250 * time.Millisecond})
	collector.Observe(ctx, endpoint.Observation{Operation: "GET /users", Duration: 250 * time.Millisecond})
	collector.Observe(ctx, endpoint.Observation{Operation: "POST /users", Duration: time.Second, Err: errors.New("conflict")})
	return collector
}

func scrape(t *testing.T, source metrics.Source, opts ...metrics.Option) (*httptest.ResponseRecorder, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	metrics.Handler(source, opts...).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return recorder, recorder.Body.String()
}

// TestHandlerWritesTheExpositionFormat proves a scraper gets what it expects: the
// declared content type, one TYPE line per family, and the counts the collector
// actually holds.
func TestHandlerWritesTheExpositionFormat(t *testing.T) {
	recorder, body := scrape(t, recorded(t))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != metrics.ContentType {
		t.Errorf("Content-Type = %q, want %q", got, metrics.ContentType)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store: a cached scrape is a lie about now", got)
	}

	for _, want := range []string{
		"# TYPE endpoint_requests_total counter",
		`endpoint_requests_total{operation="GET /users",outcome="success"} 2`,
		`endpoint_requests_total{operation="GET /users",outcome="error"} 0`,
		`endpoint_requests_total{operation="POST /users",outcome="error"} 1`,
		"# TYPE endpoint_request_duration_seconds summary",
		`endpoint_request_duration_seconds_sum{operation="GET /users"} 0.5`,
		`endpoint_request_duration_seconds_count{operation="GET /users"} 2`,
		`endpoint_request_duration_seconds_sum{operation="POST /users"} 1`,
		"# TYPE endpoint_last_request_timestamp_seconds gauge",
		`endpoint_last_request_timestamp_seconds{operation="GET /users"}`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("exposition is missing %q:\n%s", want, body)
		}
	}

	// The text format requires each family to be contiguous: a scraper rejects a
	// document where a metric name reappears after another name was seen.
	requests := strings.Index(body, "endpoint_requests_total{")
	duration := strings.Index(body, "endpoint_request_duration_seconds_sum{")
	stamp := strings.Index(body, "endpoint_last_request_timestamp_seconds{")
	if !(requests < duration && duration < stamp) {
		t.Errorf("families are interleaved: %d, %d, %d\n%s", requests, duration, stamp, body)
	}
}

// TestHandlerRefusesMethodsThatAreNotReads proves a scrape endpoint is a read: a
// POST to it is a mistake worth reporting rather than a silent 200.
func TestHandlerRefusesMethodsThatAreNotReads(t *testing.T) {
	handler := metrics.Handler(recorded(t))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/metrics", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", recorder.Code)
	}
	if allow := recorder.Header().Get("Allow"); allow != "GET, HEAD" {
		t.Errorf("Allow = %q, want GET, HEAD", allow)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodHead, "/metrics", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d, want 200", recorder.Code)
	}
	if recorder.Body.Len() != 0 {
		t.Errorf("HEAD returned a body of %d bytes", recorder.Body.Len())
	}
}

// TestExpositionEmitsNoAggregateDuplicate proves the numbers can be summed. A
// pre-summed series alongside the labelled ones would be counted twice by every
// query that aggregates, which is the default thing to do with a counter.
func TestExpositionEmitsNoAggregateDuplicate(t *testing.T) {
	_, body := scrape(t, recorded(t))
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "endpoint_requests_total ") ||
			strings.HasPrefix(line, "endpoint_request_duration_seconds_count ") {
			t.Fatalf("an unlabelled aggregate series was emitted: %q", line)
		}
	}
	if strings.Count(body, "endpoint_requests_total{") != 4 {
		t.Fatalf("want two operations times two outcomes:\n%s", body)
	}
}

// TestUnlabelledCallsAreReportedUnderAnEmptyOperation proves a scrape agrees with
// the collector. endpoint.MetricsMiddleware records without an operation, and
// those calls only reach the total; dropping them would make the endpoint report
// less traffic than the service handled.
func TestUnlabelledCallsAreReportedUnderAnEmptyOperation(t *testing.T) {
	collector := &endpoint.Metrics{}
	ctx := context.Background()
	collector.Observe(ctx, endpoint.Observation{Operation: "GET /users", Duration: time.Second})
	collector.Observe(ctx, endpoint.Observation{Duration: 2 * time.Second})

	_, body := scrape(t, collector)
	if !strings.Contains(body, `endpoint_requests_total{operation="",outcome="success"} 1`) {
		t.Fatalf("unlabelled calls are missing from the exposition:\n%s", body)
	}
	if !strings.Contains(body, `endpoint_request_duration_seconds_sum{operation=""} 2`) {
		t.Fatalf("unlabelled duration is missing or wrong:\n%s", body)
	}
}

// TestLabelValuesComeOnlyFromRecordedOperations proves the bound on cardinality:
// the series that exist are the operations recording was given, so a scrape cannot
// grow a new series because a caller invented a path.
func TestLabelValuesComeOnlyFromRecordedOperations(t *testing.T) {
	collector := recorded(t)
	_, body := scrape(t, collector)

	operations := map[string]bool{}
	for _, line := range strings.Split(body, "\n") {
		start := strings.Index(line, `operation="`)
		if start < 0 {
			continue
		}
		rest := line[start+len(`operation="`):]
		end := strings.Index(rest, `"`)
		if end < 0 {
			continue
		}
		operations[rest[:end]] = true
	}
	for operation := range operations {
		if operation == "" {
			continue
		}
		if snapshot := collector.SnapshotFor(operation); snapshot.RequestCount == 0 {
			t.Errorf("label value %q is in the exposition but not in the collector", operation)
		}
	}
	if len(operations) != 2 {
		t.Fatalf("label values = %v, want exactly the two recorded operations", operations)
	}
}

// TestLabelValuesAreEscaped proves one bad operation name cannot break the whole
// scrape: an unescaped quote makes a collector reject the entire response, not
// just the offending series.
func TestLabelValuesAreEscaped(t *testing.T) {
	collector := &endpoint.Metrics{}
	collector.Observe(context.Background(), endpoint.Observation{
		Operation: `GET /say"hi"\n`,
		Duration:  time.Millisecond,
	})

	_, body := scrape(t, collector)
	if !strings.Contains(body, `operation="GET /say\"hi\"\\n"`) {
		t.Fatalf("label value was not escaped:\n%s", body)
	}
}

// TestWithNamespaceRenamesEverySeries proves the prefix is configurable for a
// deployment whose conventions differ, and that a blank one is ignored rather than
// producing series starting with an underscore.
func TestWithNamespaceRenamesEverySeries(t *testing.T) {
	_, body := scrape(t, recorded(t), metrics.WithNamespace("usersvc"))
	if !strings.Contains(body, "usersvc_requests_total{") {
		t.Fatalf("namespace was not applied:\n%s", body)
	}
	if strings.Contains(body, "endpoint_requests_total{") {
		t.Fatalf("the default namespace is still present:\n%s", body)
	}

	_, body = scrape(t, recorded(t), metrics.WithNamespace("   "))
	if !strings.Contains(body, "endpoint_requests_total{") {
		t.Fatalf("a blank namespace was applied:\n%s", body)
	}
}

// TestHandlerRejectsANilSource proves the misassembly is loud. An endpoint over
// nothing would report zeros, which reads as a service with no traffic.
func TestHandlerRejectsANilSource(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("Handler(nil) did not panic")
		}
	}()
	metrics.Handler(nil)
}

// TestWriteReportsANilSource proves the non-HTTP path reports the same mistake as
// an error rather than a panic, because it is not on a request path.
func TestWriteReportsANilSource(t *testing.T) {
	var sink strings.Builder
	if err := metrics.Write(&sink, nil); err == nil {
		t.Fatal("Write(nil) = nil, want an error")
	}
}
