package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	transporthttp "github.com/dreamsxin/go-kit/v2/transport/http"
)

const upstreamHeader = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

func TestExtractTraceparent(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("traceparent", upstreamHeader)

	ctx := transporthttp.ExtractTraceparent(context.Background(), r)
	tc := endpoint.TraceContextFromContext(ctx)
	if !tc.Valid() {
		t.Fatalf("trace context not extracted: %+v", tc)
	}
	if tc.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("trace ID: got %q", tc.TraceID)
	}
	if tc.Flags != "01" {
		t.Errorf("flags: got %q", tc.Flags)
	}
}

func TestExtractTraceparentIgnoresInvalidHeaders(t *testing.T) {
	for _, header := range []string{"", "garbage", "00-00-00-00"} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("traceparent", header)
		ctx := transporthttp.ExtractTraceparent(context.Background(), r)
		if endpoint.TraceContextFromContext(ctx).Valid() {
			t.Errorf("invalid header %q produced a trace context", header)
		}
	}
}

func TestInjectTraceparent(t *testing.T) {
	tc := endpoint.TraceContext{
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:  "00f067aa0ba902b7",
		Flags:   "01",
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	transporthttp.InjectTraceparent(endpoint.WithTraceContext(context.Background(), tc), r)

	if got := r.Header.Get("traceparent"); got != upstreamHeader {
		t.Errorf("traceparent header: want %q, got %q", upstreamHeader, got)
	}
}

func TestInjectTraceparentWithoutContextIsNoop(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	transporthttp.InjectTraceparent(context.Background(), r)

	if got := r.Header.Get("traceparent"); got != "" {
		t.Errorf("traceparent header should be untouched, got %q", got)
	}
}

// TestTraceparentPropagatesAcrossServices exercises the full correlation
// path: an upstream request carries a traceparent header, the server
// extracts it, TracingMiddleware joins the trace, and the outbound client
// call injects a traceparent with the same trace ID for the downstream
// service.
func TestTraceparentPropagatesAcrossServices(t *testing.T) {
	var downstreamTraceID string

	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tc, ok := endpoint.ParseTraceparent(r.Header.Get("traceparent"))
		if !ok {
			t.Error("downstream received no valid traceparent")
			return
		}
		downstreamTraceID = tc.TraceID
	}))
	defer downstream.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := transporthttp.ExtractTraceparent(r.Context(), r)

		call := endpoint.TracingMiddleware()(func(ctx context.Context, _ any) (any, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, downstream.URL, nil)
			if err != nil {
				return nil, err
			}
			transporthttp.InjectTraceparent(ctx, req)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			return resp.StatusCode, nil
		})

		if _, err := call(ctx, nil); err != nil {
			t.Errorf("downstream call failed: %v", err)
		}
	}))
	defer upstream.Close()

	req, err := http.NewRequest(http.MethodGet, upstream.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("traceparent", upstreamHeader)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if downstreamTraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("downstream trace ID: want the upstream trace ID, got %q", downstreamTraceID)
	}
}
