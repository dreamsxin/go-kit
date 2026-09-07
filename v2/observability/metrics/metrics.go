// Package metrics exposes the numbers endpoint recording already collects, in
// the Prometheus text exposition format, without adding a metrics client to the
// dependency graph.
//
// v2 could always push telemetry — observability/otel does that — but the pull
// model most deployments run needs a scrape endpoint, and every application was
// left to build one. That meant each service named its own series and chose its
// own labels, so two services in one repository disagreed about what "request
// duration" meant.
//
// This package answers a scrape from an endpoint.Metrics collector and nothing
// else. It is deliberately not a metrics library: there are no custom
// collectors, no registry, and no histograms it did not measure. An application
// that wants those implements endpoint.Recorder against its own client — that
// seam has always been the extension point, and it stays the one.
//
// # The same numbers, and where the two models differ
//
// Both this exposition and observability/otel are fed by endpoint.Recorder, so a
// dashboard built on the scraped series and an alert built on the pushed one
// describe the same traffic. Two differences are real and neither is smoothed
// over:
//
//   - Counters here start at zero when the process starts, because
//     endpoint.Metrics is in memory. A scraper handles that — it detects the
//     reset — but a query that subtracts raw values across a restart will not.
//   - Duration is reported as a sum and a count, not as buckets. The collector
//     measures a total, so quantiles are not available from this endpoint; the
//     OpenTelemetry histogram is where a p99 comes from. Emitting invented
//     buckets would make the two disagree while looking like they agree.
package metrics

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// ContentType is the exposition format this package writes. Prometheus and every
// scraper that speaks its text format accept it; OpenMetrics scrapers read it as
// the fallback content type.
const ContentType = "text/plain; version=0.0.4; charset=utf-8"

// DefaultNamespace prefixes every series name. It is "endpoint" rather than
// "http" because what is being measured is an endpoint call, which may have
// arrived over HTTP or gRPC.
const DefaultNamespace = "endpoint"

// Source is what this package can expose: the read side of endpoint.Metrics. An
// application can implement it over its own collector, but the point of the
// narrow interface is that *endpoint.Metrics already satisfies it.
type Source interface {
	Snapshot() endpoint.MetricsSnapshot
	SnapshotFor(operation string) endpoint.MetricsSnapshot
	Operations() []string
}

// Option configures the exposition.
type Option func(*exposition)

// WithNamespace overrides the series name prefix. An empty or blank namespace is
// ignored rather than producing series that start with an underscore.
func WithNamespace(namespace string) Option {
	return func(e *exposition) {
		if trimmed := strings.TrimSpace(namespace); trimmed != "" {
			e.namespace = trimmed
		}
	}
}

type exposition struct {
	namespace string
	source    Source
}

// Handler serves the exposition for source. Mount it wherever probes are
// mounted; it is a plain http.Handler, so an admin mux, a separate listener, or
// the traffic listener all work — and which of those is right is a disclosure
// decision the deployment makes, not this package.
//
// A nil source is a misassembly rather than an empty scrape: reporting zeros
// would look like a service that receives no traffic.
//
// Stable: metrics.exposition-format — the metrics endpoint answers 200 with the Prometheus text exposition format and its content type.
// Covered by: TestHandlerWritesTheExpositionFormat
//
// Stable: metrics.exposition-methods — the metrics endpoint answers GET and HEAD, and 405 with an Allow header for anything else.
// Covered by: TestHandlerRefusesMethodsThatAreNotReads
func Handler(source Source, opts ...Option) http.Handler {
	if source == nil {
		panic("metrics: source cannot be nil")
	}
	e := &exposition{namespace: DefaultNamespace, source: source}
	for _, option := range opts {
		if option != nil {
			option(e)
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead:
		default:
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "metrics: read the endpoint with GET or HEAD", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", ContentType)
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodHead {
			return
		}
		// A scrape that fails halfway has already sent 200, so there is nothing
		// to report but the truncated body. Write errors here are the client
		// hanging up, which is not this server's problem to log.
		_ = e.write(w)
	})
}

// Write renders the exposition to w. It exists for the assembly that serves
// metrics over something other than HTTP, and for tests that want the bytes.
func Write(w io.Writer, source Source, opts ...Option) error {
	if source == nil {
		return fmt.Errorf("metrics: source cannot be nil")
	}
	e := &exposition{namespace: DefaultNamespace, source: source}
	for _, option := range opts {
		if option != nil {
			option(e)
		}
	}
	return e.write(w)
}

// write emits one series family at a time, which is what the text format
// requires: a scraper rejects a file where a metric name reappears after another
// name has been seen.
//
// Stable: metrics.series-are-per-operation — every series carries the operation label recorded for it and no aggregate duplicate is emitted, so a scraper sums rather than double counts.
// Covered by: TestExpositionEmitsNoAggregateDuplicate, TestUnlabelledCallsAreReportedUnderAnEmptyOperation
//
// Stable: metrics.label-cardinality — the only label values are the operations recording was given, so cardinality is bounded by the routes the server declared rather than by request data.
// Covered by: TestLabelValuesComeOnlyFromRecordedOperations, TestLabelValuesAreEscaped
func (e *exposition) write(w io.Writer) error {
	rows := e.rows()

	var buf strings.Builder
	buf.WriteString("# HELP " + e.name("requests_total") + " Endpoint calls recorded, by operation and outcome.\n")
	buf.WriteString("# TYPE " + e.name("requests_total") + " counter\n")
	for _, row := range rows {
		buf.WriteString(fmt.Sprintf("%s{operation=\"%s\",outcome=\"success\"} %d\n",
			e.name("requests_total"), escapeLabel(row.operation), row.snapshot.SuccessCount))
		buf.WriteString(fmt.Sprintf("%s{operation=\"%s\",outcome=\"error\"} %d\n",
			e.name("requests_total"), escapeLabel(row.operation), row.snapshot.ErrorCount))
	}

	buf.WriteString("# HELP " + e.name("request_duration_seconds") + " Time spent in endpoint calls.\n")
	// A summary without quantiles: the collector measures a total and a count,
	// so that is what is reported. Buckets this package did not measure would be
	// invented data.
	buf.WriteString("# TYPE " + e.name("request_duration_seconds") + " summary\n")
	for _, row := range rows {
		buf.WriteString(fmt.Sprintf("%s_sum{operation=\"%s\"} %s\n",
			e.name("request_duration_seconds"), escapeLabel(row.operation), seconds(row.snapshot.TotalDuration)))
		buf.WriteString(fmt.Sprintf("%s_count{operation=\"%s\"} %d\n",
			e.name("request_duration_seconds"), escapeLabel(row.operation), row.snapshot.RequestCount))
	}

	buf.WriteString("# HELP " + e.name("last_request_timestamp_seconds") + " When an operation was last called, in Unix seconds.\n")
	buf.WriteString("# TYPE " + e.name("last_request_timestamp_seconds") + " gauge\n")
	for _, row := range rows {
		if row.snapshot.LastRequestTime.IsZero() {
			continue
		}
		buf.WriteString(fmt.Sprintf("%s{operation=\"%s\"} %s\n",
			e.name("last_request_timestamp_seconds"), escapeLabel(row.operation),
			timestamp(row.snapshot.LastRequestTime)))
	}

	_, err := io.WriteString(w, buf.String())
	return err
}

type row struct {
	operation string
	snapshot  endpoint.MetricsSnapshot
}

// rows collects one series row per recorded operation, plus the remainder that
// was recorded without an operation label.
//
// No aggregate row is emitted: a scraper sums the labelled series itself, and a
// pre-summed series would be counted twice by every query that does.
func (e *exposition) rows() []row {
	operations := e.source.Operations()
	sort.Strings(operations)

	rows := make([]row, 0, len(operations)+1)
	var labelled endpoint.MetricsSnapshot
	for _, operation := range operations {
		snapshot := e.source.SnapshotFor(operation)
		labelled.RequestCount += snapshot.RequestCount
		labelled.SuccessCount += snapshot.SuccessCount
		labelled.ErrorCount += snapshot.ErrorCount
		labelled.TotalDuration += snapshot.TotalDuration
		rows = append(rows, row{operation: operation, snapshot: snapshot})
	}

	// endpoint.MetricsMiddleware records without an operation, and those calls
	// only reach the total. Dropping them would make a scrape disagree with the
	// collector; inventing a name for them would make it disagree with the code.
	total := e.source.Snapshot()
	remainder := endpoint.MetricsSnapshot{
		RequestCount:    total.RequestCount - labelled.RequestCount,
		SuccessCount:    total.SuccessCount - labelled.SuccessCount,
		ErrorCount:      total.ErrorCount - labelled.ErrorCount,
		TotalDuration:   total.TotalDuration - labelled.TotalDuration,
		LastRequestTime: total.LastRequestTime,
	}
	if remainder.RequestCount > 0 {
		rows = append(rows, row{operation: "", snapshot: remainder})
	}
	return rows
}

func (e *exposition) name(suffix string) string {
	return e.namespace + "_" + suffix
}

// escapeLabel escapes what the text format reserves. An operation label comes
// from a route pattern, but a pattern with a quote in it would otherwise produce
// a scrape the collector rejects outright — every series in the response, not
// just this one.
func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return strings.ReplaceAll(value, "\n", `\n`)
}

func seconds(duration time.Duration) string {
	return formatFloat(duration.Seconds())
}

func timestamp(at time.Time) string {
	return formatFloat(float64(at.UnixNano()) / float64(time.Second))
}

// formatFloat renders a float the way the text format expects: enough digits to
// be useful, no exponent for ordinary values.
func formatFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", value), "0"), ".")
}
