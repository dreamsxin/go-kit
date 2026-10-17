package interaction

import (
	"context"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// ContinueTrace continues the W3C correlation for one server-side operation:
// a valid inbound traceparent keeps the caller's trace ID as the parent and
// mints this node's span ID and a request ID; an absent or malformed header
// starts a fresh trace. A server boundary that owns no endpoint chain — the
// built-in MCP HTTP server is one — calls it on the inbound context, so the
// tool calls dispatched underneath log the caller's trace_id instead of an
// unattributable empty field.
//
// Stable: mcp.server-extracts-traceparent — the built-in MCP server continues
// the inbound W3C traceparent before dispatch, so a tool call's log joins the
// caller's trace, and an unreadable header starts a fresh trace rather than
// failing the request.
// Covered by: TestLogToolCallCarriesCorrelation, TestContinueTraceMintsWhenAbsent,
// TestContinueTraceIgnoresMalformed
func ContinueTrace(ctx context.Context, traceparent string) context.Context {
	if tc, ok := endpoint.ParseTraceparent(traceparent); ok {
		ctx = endpoint.WithIncomingTraceContext(ctx, tc)
	}
	return endpoint.ContinueIncomingTrace(ctx)
}
