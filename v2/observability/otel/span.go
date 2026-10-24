package oteladapter

import (
	"net/http"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

// HTTPServerMiddleware returns standard http.Handler middleware that starts
// one SERVER span per inbound request. Install it with kit.WithHTTPMiddleware
// around the whole server; the span it starts is the parent the endpoint
// span (TracingMiddleware) and any outbound client span join, so one request
// becomes a tree instead of three unrelated spans.
//
// The span is named `{method}` — `GET` — and gains the matched route
// (`GET /products/{id}`) when the mux set one on the request, per the HTTP
// semantic conventions: a URI path is never a span name or a route. The
// status follows the conventions too: 1xx–3xx and 4xx leave the span status
// unset on a server, 5xx marks it Error with `error.type` carrying the code.
//
// Stable: otel.http-server-span — the middleware starts one SERVER span per
// request, named GET or GET with the matched route, carrying
// http.request.method, url.scheme, url.path and http.response.status_code,
// with 5xx recorded as Error.
// Covered by: TestHTTPServerMiddlewareStartsASERVERSpan,
// TestHTTPServerMiddlewareNamesTheRoute, TestHTTPServerMiddlewareMarks5xxAsError
func HTTPServerMiddleware(tracer trace.Tracer, options ...TraceOption) func(http.Handler) http.Handler {
	if tracer == nil {
		tracer = otel.Tracer(instrumentationName)
	}
	cfg := traceOptions{kind: trace.SpanKindServer}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			method := canonicalMethod(r.Method)
			attrs := []attribute.KeyValue{
				semconv.HTTPRequestMethodKey.String(method),
				semconv.URLScheme(r.URL.Scheme),
				semconv.URLPath(r.URL.Path),
			}
			// A ServeMux hands its matched pattern to the handler it
			// dispatches to, on a copy of the request — the middleware
			// sits outside that. Resolve the route here instead, so the
			// span carries its full name from creation (which is what
			// sampling decisions see).
			route := ""
			name := method
			if mux, ok := next.(*http.ServeMux); ok {
				if _, pattern := mux.Handler(r); pattern != "" {
					// Handler spells the pattern with its method when it
					// declares one; the name takes that whole shape, and
					// the route attribute keeps the path only.
					name = pattern
					route = strings.TrimPrefix(pattern, method+" ")
					attrs = append(attrs, semconv.HTTPRoute(route))
				}
			}
			ctx, span := tracer.Start(r.Context(), name,
				trace.WithSpanKind(cfg.kind),
				trace.WithAttributes(attrs...),
				trace.WithAttributes(cfg.attributes...),
			)
			defer span.End()

			iw := &spanResponseWriter{ResponseWriter: w}
			next.ServeHTTP(iw, r.WithContext(ctx))

			code := iw.code
			span.SetAttributes(semconv.HTTPResponseStatusCode(code))
			if code >= 500 {
				span.SetStatus(codes.Error, "")
				span.SetAttributes(semconv.ErrorTypeKey.String(strconv.Itoa(code)))
			}
		})
	}
}

// NewRoundTripper wraps an http.RoundTripper so every outbound call through
// it starts one CLIENT span and injects the active trace context into the
// request headers through the global propagator — the same propagator
// Setup installs, so a span this transport starts is what the next service
// extracts. Wrap client.DefaultClient's transport, or any *http.Transport,
// and hand the wrapped transport back with client.SetClient.
//
// A transport error and a 4xx or 5xx response mark the span Error, per the
// conventions for a client; the response status code is an attribute either
// way, and `error.type` carries the code on a server-class failure.
//
// Stable: otel.http-client-span — the RoundTripper starts one CLIENT span per
// call, carrying server.address, server.port and url.full, injecting the
// trace context, and marking transport errors and 4xx/5xx as Error.
// Covered by: TestRoundTripperStartsACLIENTSpan, TestRoundTripperMarksErrorsAndInjects
func NewRoundTripper(tracer trace.Tracer, base http.RoundTripper) http.RoundTripper {
	if tracer == nil {
		tracer = otel.Tracer(instrumentationName)
	}
	if base == nil {
		base = http.DefaultTransport
	}
	return roundTripper{tracer: tracer, base: base}
}

type roundTripper struct {
	tracer trace.Tracer
	base   http.RoundTripper
}

func (t roundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	method := canonicalMethod(request.Method)
	attrs := []attribute.KeyValue{
		semconv.HTTPRequestMethodKey.String(method),
		semconv.URLFull(request.URL.String()),
		semconv.ServerAddress(request.URL.Hostname()),
	}
	if port := request.URL.Port(); port != "" {
		if n, err := strconv.Atoi(port); err == nil {
			attrs = append(attrs, semconv.ServerPort(n))
		}
	}
	ctx, span := t.tracer.Start(request.Context(), method,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
	defer span.End()

	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(request.Header))

	response, err := t.base.RoundTrip(request.WithContext(ctx))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "")
		span.SetAttributes(semconv.ErrorTypeKey.String(errorTypeName(err)))
		return response, err
	}
	span.SetAttributes(semconv.HTTPResponseStatusCode(response.StatusCode))
	if response.StatusCode >= 400 {
		span.SetStatus(codes.Error, "")
		span.SetAttributes(semconv.ErrorTypeKey.String(strconv.Itoa(response.StatusCode)))
	}
	return response, err
}

// canonicalMethod maps an arbitrary method to the conventions' bounded
// enumeration, falling back to the literal `_OTHER` a span name needs.
func canonicalMethod(method string) string {
	switch method {
	case http.MethodConnect, http.MethodDelete, http.MethodGet, http.MethodHead,
		http.MethodOptions, http.MethodPatch, http.MethodPost, http.MethodPut,
		http.MethodTrace:
		return method
	default:
		return "_OTHER"
	}
}

// errorTypeName classifies a transport error for error.type: the conventions
// reserve `_OTHER` for everything no narrower rule covers, and a transport
// error has no status code to speak of.
func errorTypeName(err error) string {
	return "_OTHER"
}

// spanResponseWriter captures the status code a handler answered with and
// forwards Flush so a stream through the span middleware stays a stream.
type spanResponseWriter struct {
	http.ResponseWriter
	code  int
	wrote bool
}

func (w *spanResponseWriter) WriteHeader(code int) {
	if !w.wrote {
		w.code = code
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *spanResponseWriter) Write(p []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(p)
}

func (w *spanResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
