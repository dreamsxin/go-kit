package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// AccessLogMiddleware returns standard http.Handler middleware that records one
// access line per request: method, path, status, response bytes, and
// duration, plus the incoming trace ID when the request carries a valid W3C
// traceparent header. Install it with kit.WithHTTPMiddleware. A nil logger
// falls back to slog.Default().
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
			if tc, ok := endpoint.ParseTraceparent(r.Header.Get("traceparent")); ok {
				attrs = append(attrs, slog.String("trace_id", tc.TraceID))
			}
			logger.LogAttrs(r.Context(), slog.LevelInfo, "http access", attrs...)
		})
	}
}
