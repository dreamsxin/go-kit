package oteladapter

import (
	"context"
	"errors"
	"strconv"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/semconv/v1.40.0/httpconv"

	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// HTTPMetrics records HTTP server requests under the OpenTelemetry HTTP
// semantic conventions: one http.server.request.duration histogram in seconds,
// carrying http.request.method, url.scheme, http.route, and
// http.response.status_code.
//
// http.response.status_code is what makes response status alertable from
// metrics — a rise in 5xx is a query over this one series, with no log
// pipeline in between. http.route is the matched pattern rather than the URL
// path, which is what keeps the dimension bounded.
type HTTPMetrics struct {
	duration httpconv.ServerRequestDuration
	attrs    []attribute.KeyValue
}

var _ httpserver.Recorder = (*HTTPMetrics)(nil)

// NewHTTPMetrics creates the HTTP server instruments from the
// application-owned meter. Install the result with kit.WithHTTPRecorder, or
// with httpserver.RecordingMiddleware for a hand-assembled server.
func NewHTTPMetrics(meter metric.Meter, options ...MetricsOption) (*HTTPMetrics, error) {
	if meter == nil {
		return nil, errors.New("oteladapter: meter is nil")
	}
	cfg := metricsOptions{}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	duration, err := httpconv.NewServerRequestDuration(meter)
	if err != nil {
		return nil, err
	}
	return &HTTPMetrics{
		duration: duration,
		attrs:    append([]attribute.KeyValue(nil), cfg.attributes...),
	}, nil
}

// ObserveHTTP implements httpserver.Recorder.
func (m *HTTPMetrics) ObserveHTTP(ctx context.Context, obs httpserver.Observation) {
	if m == nil {
		return
	}
	attrs := append([]attribute.KeyValue(nil), m.attrs...)
	attrs = append(attrs, m.duration.AttrResponseStatusCode(obs.StatusCode))
	if obs.Route != "" {
		// An unmatched request has no route. Recording one anyway would either
		// invent a label or feed the URL path into an unbounded dimension.
		attrs = append(attrs, m.duration.AttrRoute(obs.Route))
	}
	if obs.StatusCode >= 500 {
		// The conventions ask for error.type on a server error, and the status
		// code is the only classification a transport has.
		attrs = append(attrs, m.duration.AttrErrorType(httpconv.ErrorTypeAttr(strconv.Itoa(obs.StatusCode))))
	}
	m.duration.Record(ctx, obs.Duration.Seconds(), requestMethod(obs.Method), scheme(obs.Scheme), attrs...)
}

// requestMethod maps a method to the conventions' bounded enumeration, so a
// client sending an arbitrary method cannot create a new time series.
func requestMethod(method string) httpconv.RequestMethodAttr {
	switch method {
	case "CONNECT":
		return httpconv.RequestMethodConnect
	case "DELETE":
		return httpconv.RequestMethodDelete
	case "GET":
		return httpconv.RequestMethodGet
	case "HEAD":
		return httpconv.RequestMethodHead
	case "OPTIONS":
		return httpconv.RequestMethodOptions
	case "PATCH":
		return httpconv.RequestMethodPatch
	case "POST":
		return httpconv.RequestMethodPost
	case "PUT":
		return httpconv.RequestMethodPut
	case "QUERY":
		return httpconv.RequestMethodQuery
	case "TRACE":
		return httpconv.RequestMethodTrace
	default:
		return httpconv.RequestMethodOther
	}
}

func scheme(s string) string {
	if s == "" {
		return "http"
	}
	return s
}
