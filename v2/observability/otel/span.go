package oteladapter

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
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
				// The URL of a server-side request carries no scheme —
				// net/http fills only path and query — so the scheme comes
				// from the transport that received it.
				semconv.URLScheme(schemeOf(r)),
				semconv.URLPath(r.URL.Path),
			}
			// A ServeMux hands its matched pattern to the handler it
			// dispatches to, on a copy of the request — the middleware
			// sits outside that. Resolve the route here instead, so the
			// span carries its full name from creation (which is what
			// sampling decisions see). A pattern Handler returns for a
			// request it would redirect — /products onto /products/, a
			// cleaned path — is the target path, not a registration, and a
			// URI path is never a span name; those fall back to the method.
			route := ""
			name := method
			if mux, ok := next.(*http.ServeMux); ok {
				if handler, pattern := mux.Handler(r); pattern != "" && !isMuxRedirect(handler) {
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

	// RoundTrip must not modify the request it is handed: a caller retrying
	// with the same *http.Request would otherwise re-send the first
	// attempt's traceparent — attaching the retry to a dead trace — and a
	// shared request is a data race on the header map. The clone is the
	// upstream otelhttp transport's own rule.
	request = request.Clone(ctx)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(request.Header))

	response, err := t.base.RoundTrip(request)
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

// spanResponseWriter captures the status code a handler answered with,
// defaulting to the implicit 200 of a handler that only writes, and
// forwards the optional interfaces a handler may assert — Flush for
// streams, Hijack for protocol upgrades, ReaderFrom for sendfile — so the
// span middleware is transparent to everything but the span.
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
	if !w.wrote {
		// net/http answers 200 when the handler never says otherwise; the
		// span must record what the client saw, not the zero value.
		w.code = http.StatusOK
		w.wrote = true
	}
	return w.ResponseWriter.Write(p)
}

func (w *spanResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack keeps protocol upgrades working through the middleware.
func (w *spanResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("oteladapter: response writer cannot hijack")
	}
	if !w.wrote {
		w.code = http.StatusSwitchingProtocols
		w.wrote = true
	}
	return h.Hijack()
}

// ReadFrom keeps the io.ReaderFrom fast path (sendfile) through the
// middleware where the underlying writer has it.
func (w *spanResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	if !w.wrote {
		w.code = http.StatusOK
		w.wrote = true
	}
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(r)
	}
	return io.Copy(onlyWriter{w}, r)
}

type onlyWriter struct{ w *spanResponseWriter }

func (o onlyWriter) Write(p []byte) (int, error) { return o.w.Write(p) }

// schemeOf reports the URL scheme of a server-side request: the request's
// URL carries none, so the transport that received it decides.
func schemeOf(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// isMuxRedirect reports whether the ServeMux answered with its own redirect
// — the trailing-slash and cleaned-path cases — whose "pattern" is the
// redirect target rather than a registered route.
func isMuxRedirect(handler http.Handler) bool {
	return strings.Contains(fmt.Sprintf("%T", handler), "redirectHandler")
}
