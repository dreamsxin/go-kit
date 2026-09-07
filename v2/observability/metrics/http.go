package metrics

import (
	"context"
	"fmt"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// The HTTP bridge.
//
// Recording happens at two layers. endpoint.RecordingMiddleware sees business
// outcomes and knows the operation; the HTTP transport sees status codes and knows
// the matched route. A service that only wires the HTTP layer — the generated
// projects do, because they have no endpoint chain around every handler — had no
// way to feed the collector this package exposes.
//
// HTTPRecorder is that bridge, and it is a translation rather than a second
// measurement: the same request produces one observation, which is what keeps a
// scrape from double counting.

// HTTPRecorder adapts an endpoint.Metrics collector to the HTTP transport's
// recorder, so httpserver.RecordingMiddleware can feed the numbers this package
// exposes.
//
//	collector := &endpoint.Metrics{}
//	recording := httpserver.RecordingMiddleware(metrics.HTTPRecorder(collector))
//	mux.Handle("GET /users/{id}", recording(usersHandler))
//	mux.Handle("GET /metrics", metrics.Handler(collector))
//
// Install it per route, inside the mux: the route pattern is only readable by the
// handler a ServeMux dispatched to, and middleware wrapped around the mux records
// every request with an empty route.
//
// A nil collector is a misassembly rather than a silent no-op.
func HTTPRecorder(collector *endpoint.Metrics) httpserver.Recorder {
	if collector == nil {
		panic("metrics: collector cannot be nil")
	}
	return httpserver.RecorderFunc(func(ctx context.Context, obs httpserver.Observation) {
		operation, outcome, ok := translate(obs)
		if !ok {
			return
		}
		collector.Observe(ctx, endpoint.Observation{
			Operation: operation,
			Duration:  obs.Duration,
			Err:       outcome,
		})
	})
}

// translate maps one HTTP exchange onto the collector's vocabulary, and reports
// whether it should be recorded at all.
//
// Stable: metrics.http-bridge-ignores-unrouted — an HTTP observation with no matched route is not recorded, so a request that matched nothing cannot add a series.
// Covered by: TestHTTPRecorderIgnoresUnroutedRequests
//
// Stable: metrics.http-bridge-error-class — a 5xx response is recorded as an error and a 4xx is not, because a rejected request is not a broken server.
// Covered by: TestHTTPRecorderCountsServerErrorsOnly
func translate(obs httpserver.Observation) (operation string, outcome error, record bool) {
	if obs.Route == "" {
		// The route is empty exactly when the request matched nothing. Recording
		// it would either invent a label from the URL — unbounded — or file it
		// under the empty operation, where a scan for /.env would show up as
		// traffic the service served.
		return "", nil, false
	}
	if obs.StatusCode >= 500 {
		// The status is the only outcome the transport knows. 4xx is the caller
		// being told no, which is a working server; 5xx is this service failing,
		// which is what an error rate should measure.
		return obs.Route, fmt.Errorf("http %d", obs.StatusCode), true
	}
	return obs.Route, nil, true
}
