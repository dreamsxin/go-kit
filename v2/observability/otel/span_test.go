package oteladapter_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	oteladapter "github.com/dreamsxin/go-kit/v2/observability/otel"
)

func newRecordingTracer(t *testing.T) (trace.Tracer, *tracetest.SpanRecorder) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	recorder := tracetest.NewSpanRecorder()
	tp.RegisterSpanProcessor(recorder)
	return tp.Tracer("oteladapter_test"), recorder
}

func byName(recorder *tracetest.SpanRecorder, name string) sdktrace.ReadOnlySpan {
	for _, stub := range recorder.Ended() {
		if stub.Name() == name {
			return stub
		}
	}
	return nil
}

func attr(stub sdktrace.ReadOnlySpan, key string) string {
	for _, kv := range stub.Attributes() {
		if string(kv.Key) == key {
			return kv.Value.Emit()
		}
	}
	return ""
}

// TestHTTPServerMiddlewareStartsASERVERSpan pins otel.http-server-span: the
// middleware starts one SERVER span carrying the conventions' attributes, and
// a 200 leaves the span status unset.
func TestHTTPServerMiddlewareStartsASERVERSpan(t *testing.T) {
	tracer, recorder := newRecordingTracer(t)
	middleware := oteladapter.HTTPServerMiddleware(tracer)
	served := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	request := httptest.NewRequest(http.MethodGet, "https://example.com/products?on_sale=1", nil)
	served.ServeHTTP(httptest.NewRecorder(), request)

	stub := byName(recorder, "GET")
	if stub == nil {
		t.Fatalf("spans = %v, want a GET span", recorder.Ended())
	}
	if stub.SpanKind() != trace.SpanKindServer {
		t.Errorf("kind = %v, want SERVER", stub.SpanKind())
	}
	if got := attr(stub, "http.request.method"); got != "GET" {
		t.Errorf("http.request.method = %v", got)
	}
	if got := attr(stub, "url.path"); got != "/products" {
		t.Errorf("url.path = %v", got)
	}
	if got := attr(stub, "url.scheme"); got != "https" {
		t.Errorf("url.scheme = %v", got)
	}
	if got := attr(stub, "http.response.status_code"); got != "200" {
		t.Errorf("http.response.status_code = %v", got)
	}
	if stub.Status().Code != codes.Unset {
		t.Errorf("status = %v, want Unset for a 200 on a server span", stub.Status())
	}
}

// TestHTTPServerMiddlewareNamesTheRoute covers the rename: once the mux has
// matched, the span is named after the route and carries it as an attribute.
// A URI path never becomes either.
func TestHTTPServerMiddlewareNamesTheRoute(t *testing.T) {
	tracer, recorder := newRecordingTracer(t)
	mux := http.NewServeMux()
	mux.Handle("GET /products/{id}", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	served := oteladapter.HTTPServerMiddleware(tracer)(mux)

	served.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/products/17", nil))

	stub := byName(recorder, "GET /products/{id}")
	if stub == nil {
		for _, sp := range recorder.Ended() {
			t.Logf("recorded span name: %q route=%v", sp.Name(), attr(sp, "http.route"))
		}
		t.Fatalf("spans = %v, want the route-named span", len(recorder.Ended()))
	}
	if got := attr(stub, "http.route"); got != "/products/{id}" {
		t.Errorf("http.route = %v", got)
	}
}

// TestHTTPServerMiddlewareMarks5xxAsError covers the status rule: 5xx is
// Error on a server span and error.type names the code.
func TestHTTPServerMiddlewareMarks5xxAsError(t *testing.T) {
	tracer, recorder := newRecordingTracer(t)
	served := oteladapter.HTTPServerMiddleware(tracer)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))

	served.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

	stub := byName(recorder, "GET")
	if stub == nil || stub.Status().Code != codes.Error {
		t.Fatalf("a 502 must mark the span Error, got %v", stub)
	}
	if got := attr(stub, "error.type"); got != "502" {
		t.Errorf("error.type = %v, want the status code", got)
	}
}

// TestRoundTripperStartsACLIENTSpan pins otel.http-client-span: one CLIENT
// span per outbound call with the conventions' required attributes, ended
// after the response arrives.
func TestRoundTripperStartsACLIENTSpan(t *testing.T) {
	tracer, recorder := newRecordingTracer(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	transport := oteladapter.NewRoundTripper(tracer, server.Client().Transport)
	request, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/items", nil)
	if _, err := transport.RoundTrip(request); err != nil {
		t.Fatalf("round trip: %v", err)
	}

	stub := byName(recorder, "GET")
	if stub == nil {
		t.Fatalf("spans = %v, want a client span", recorder.Ended())
	}
	if stub.SpanKind() != trace.SpanKindClient {
		t.Errorf("kind = %v, want CLIENT", stub.SpanKind())
	}
	if got := attr(stub, "server.address"); got != "127.0.0.1" {
		t.Errorf("server.address = %v", got)
	}
	if got := attr(stub, "url.full"); !strings.HasPrefix(got, server.URL+"/items") {
		t.Errorf("url.full = %v", got)
	}
	if got := attr(stub, "http.response.status_code"); got != "204" {
		t.Errorf("http.response.status_code = %v", got)
	}
	if stub.Status().Code != codes.Unset {
		t.Errorf("a 204 leaves a client span Unset, got %v", stub.Status())
	}
}

// TestRoundTripperMarksErrorsAndInjects covers the other half of
// otel.http-client-span: a 4xx marks the span Error with error.type, and the
// request headers carry the active trace context, so the next service
// extracts the span this transport started.
func TestRoundTripperMarksErrorsAndInjects(t *testing.T) {
	tracer, recorder := newRecordingTracer(t)
	var seenTraceparent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenTraceparent = r.Header.Get("traceparent")
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	transport := oteladapter.NewRoundTripper(tracer, server.Client().Transport)
	request, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d", response.StatusCode)
	}

	stub := byName(recorder, "GET")
	if stub.Status().Code != codes.Error {
		t.Fatalf("a 403 marks a client span Error, got %v", stub.Status())
	}
	if got := attr(stub, "error.type"); got != "403" {
		t.Errorf("error.type = %v", got)
	}
	if !strings.HasPrefix(seenTraceparent, "00-") {
		t.Fatalf("traceparent = %q, want the injected context", seenTraceparent)
	}
	// The injected header is the span this transport started, so the tree
	// continues across the hop.
	if !strings.Contains(seenTraceparent, stub.SpanContext().SpanID().String()) {
		t.Errorf("traceparent %q does not name span %s", seenTraceparent, stub.SpanContext().SpanID())
	}
}

// TestServerAndClientSpansFormATree is the property the transport spans
// exist for: server → endpoint → client share one trace, the client span
// parented in the server span through the request context.
func TestServerAndClientSpansFormATree(t *testing.T) {
	tracer, recorder := newRecordingTracer(t)

	var clientTransport = oteladapter.NewRoundTripper(tracer, http.DefaultTransport)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://example.com/relays", nil)
		_, _ = clientTransport.RoundTrip(request)
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	served := oteladapter.HTTPServerMiddleware(tracer)(handler)
	served.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/forward", nil))

	spans := recorder.Ended()
	var serverSpan, clientSpan sdktrace.ReadOnlySpan
	for i := range spans {
		switch spans[i].SpanKind() {
		case trace.SpanKindServer:
			serverSpan = spans[i]
		case trace.SpanKindClient:
			clientSpan = spans[i]
		}
	}
	if serverSpan == nil || clientSpan == nil {
		t.Fatalf("spans = %v, want one SERVER and one CLIENT", spans)
	}
	if clientSpan.Parent().SpanID() != serverSpan.SpanContext().SpanID() {
		t.Fatalf("client span parent = %v, want the server span %s",
			clientSpan.Parent().SpanID(), serverSpan.SpanContext().SpanID())
	}
	if clientSpan.SpanContext().TraceID() != serverSpan.SpanContext().TraceID() {
		t.Fatal("the two spans do not share a trace")
	}
}
