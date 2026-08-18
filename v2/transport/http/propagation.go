package http

import (
	"context"
	"net/http"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// ExtractTraceparent is a RequestFunc that extracts the W3C traceparent
// header from an incoming request into the context as an
// endpoint.TraceContext. Use it as a server Before hook so the endpoint
// chain joins the caller's trace instead of starting a new one:
//
//	server.NewServer(ep, server.ServerBefore(transporthttp.ExtractTraceparent))
//
// Invalid or absent headers are ignored; endpoint.TracingMiddleware then
// mints a fresh trace.
func ExtractTraceparent(ctx context.Context, r *http.Request) context.Context {
	tc, ok := endpoint.ParseTraceparent(r.Header.Get("traceparent"))
	if !ok {
		return ctx
	}
	return endpoint.WithTraceContext(ctx, tc)
}

// InjectTraceparent is a RequestFunc that writes the endpoint.TraceContext
// from the context into the traceparent header of an outgoing request. Use
// it as a client Before hook so downstream services continue the same trace:
//
//	client.NewClient(method, tgt, enc, dec, client.Before(transporthttp.InjectTraceparent))
//
// The injected parent span ID is the current operation's span, giving the
// downstream service a connected parent. Requests without a trace context in
// the context are left untouched; pair this hook with
// endpoint.TracingMiddleware to guarantee one exists.
func InjectTraceparent(ctx context.Context, r *http.Request) context.Context {
	tc := endpoint.TraceContextFromContext(ctx)
	if !tc.Valid() {
		return ctx
	}
	r.Header.Set("traceparent", tc.String())
	return ctx
}
