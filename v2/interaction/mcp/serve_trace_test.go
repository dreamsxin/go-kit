package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// TestWithIncomingTraceJoinsInboundTraceparent pins
// mcp.server-extracts-traceparent: the built-in server continues the caller's
// trace, so the tool calls dispatched underneath log that trace_id.
func TestWithIncomingTraceJoinsInboundTraceparent(t *testing.T) {
	const header = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	var (
		gotTrace endpoint.TraceID
		gotSpan  endpoint.SpanID
	)
	handler := withIncomingTrace(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTrace = endpoint.TraceIDFromContext(r.Context())
		gotSpan = endpoint.SpanIDFromContext(r.Context())
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("traceparent", header)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if gotTrace != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace_id = %q, want the inbound trace", gotTrace)
	}
	if gotSpan == "" || gotSpan == "00f067aa0ba902b7" {
		t.Fatalf("span_id = %q, want a fresh child span rather than the remote parent", gotSpan)
	}
}

// TestWithIncomingTraceMintsWhenAbsent covers the other half of the promise:
// no header is not an uncorrelated request — a fresh trace is minted.
func TestWithIncomingTraceMintsWhenAbsent(t *testing.T) {
	var seen endpoint.TraceContext
	handler := withIncomingTrace(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = endpoint.TraceContextFromContext(r.Context())
		w.WriteHeader(http.StatusAccepted)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/mcp", nil))

	if !seen.Valid() {
		t.Fatalf("trace context = %+v, want a freshly minted valid context", seen)
	}
}
