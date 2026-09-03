package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// requestIDResponseHeader is where kit.WithRequestID publishes the ID before
// the handler runs, which is the only place this middleware can read it: HTTP
// middleware wraps the mux, so it runs outside the context the component
// prepares per request.
const requestIDResponseHeader = "X-Request-ID"

// AccessLogMiddleware returns standard http.Handler middleware that records one
// access line per request: method, path, status, response bytes, and duration,
// plus request_id when kit.WithRequestID is enabled and trace_id when the
// request carries a valid W3C traceparent header. Install it with
// kit.WithHTTPMiddleware. A nil logger falls back to slog.Default().
//
// Access logging is a transport-layer concern: this middleware sees protocol
// facts (status codes, bytes), not business outcomes. Business-level logging
// belongs in endpoint middleware such as slogadapter.LoggingMiddleware, which
// observes decoded requests and business errors.
func AccessLogMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			iw := &InterceptingWriter{ResponseWriter: w, code: http.StatusOK}
			next.ServeHTTP(iw.reimplementInterfaces(), r)

			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", iw.GetCode()),
				slog.Int64("bytes", iw.GetWritten()),
				slog.Duration("duration", time.Since(start)),
			}
			if id := requestIDForAccessLog(r, w); id != "" {
				attrs = append(attrs, slog.String("request_id", id))
			}
			if tc, ok := endpoint.ParseTraceparent(r.Header.Get("traceparent")); ok {
				attrs = append(attrs, slog.String("trace_id", tc.TraceID))
			}
			logger.LogAttrs(r.Context(), slog.LevelInfo, "http access", attrs...)
		})
	}
}

// requestIDForAccessLog prefers the context, so the middleware also works when
// installed inside a chain that already populated it, and falls back to the
// response header the component wrote.
func requestIDForAccessLog(r *http.Request, w http.ResponseWriter) string {
	if id := endpoint.RequestIDFromContext(r.Context()); id != "" {
		return id
	}
	return w.Header().Get(requestIDResponseHeader)
}
