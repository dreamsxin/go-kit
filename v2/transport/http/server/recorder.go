package server

import (
	"context"
	"net/http"
	"time"
)

// Observation is one HTTP exchange handed to a Recorder. It carries protocol
// facts only — the transport layer knows the route, the method, and the status
// code, and does not know what the business outcome was. Business outcomes
// reach a metrics backend through endpoint.Recorder instead.
type Observation struct {
	// Method is the HTTP request method.
	Method string
	// Route is the matched route pattern, taken from http.Request.Pattern.
	// It is empty when the request matched no route, which is what keeps an
	// unrouted URL from becoming an unbounded metric dimension.
	Route string
	// Scheme is the request URL scheme: "https" when the request arrived over
	// TLS, otherwise "http".
	Scheme string
	// StatusCode is the response status code.
	StatusCode int
	// ResponseBytes is the number of body bytes written.
	ResponseBytes int64
	// Duration is how long the handler took, measured around it.
	Duration time.Duration
}

// Recorder receives one Observation per HTTP request. Implement it to report
// HTTP server metrics to a backend; observability/otel implements it against
// the OpenTelemetry HTTP semantic conventions.
//
// ObserveHTTP runs on the request path and must not block.
type Recorder interface {
	ObserveHTTP(ctx context.Context, obs Observation)
}

// RecorderFunc adapts a function to Recorder.
type RecorderFunc func(ctx context.Context, obs Observation)

// ObserveHTTP implements Recorder.
func (f RecorderFunc) ObserveHTTP(ctx context.Context, obs Observation) { f(ctx, obs) }

// RecordingMiddleware returns standard http.Handler middleware that reports one
// Observation per request to each recorder, in the order they were passed.
//
// Install it per route, inside the mux: Route comes from
// http.Request.Pattern, which only the handler a ServeMux dispatched to can
// read. Middleware wrapped around the mux instead would record every request
// with an empty route. kit.WithHTTPRecorder installs it in the right place.
//
// It panics when a recorder is nil so misassembly fails at startup rather than
// on the first request.
func RecordingMiddleware(recorders ...Recorder) func(http.Handler) http.Handler {
	for _, recorder := range recorders {
		if recorder == nil {
			panic("http server: recorder cannot be nil")
		}
	}
	observers := append([]Recorder(nil), recorders...)
	return func(next http.Handler) http.Handler {
		if len(observers) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			iw := &InterceptingWriter{ResponseWriter: w, code: http.StatusOK}
			next.ServeHTTP(iw.reimplementInterfaces(), r)

			obs := Observation{
				Method:        r.Method,
				Route:         r.Pattern,
				Scheme:        requestScheme(r),
				StatusCode:    iw.GetCode(),
				ResponseBytes: iw.GetWritten(),
				Duration:      time.Since(start),
			}
			for _, recorder := range observers {
				recorder.ObserveHTTP(r.Context(), obs)
			}
		})
	}
}

// requestScheme reports the scheme the request arrived over. A server request
// has an empty URL.Scheme, so TLS is the only thing that distinguishes the two.
func requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
