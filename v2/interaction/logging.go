package interaction

import (
	"context"
	"log/slog"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// The attribute names a tool call is reported under. They are the ones
// observability/slog's LoggingMiddleware writes for an endpoint call —
// duration, success, error, trace_id, request_id — so the tool call and the
// request that triggered it join on the same identifiers instead of being two
// unrelated logs.
const (
	logAttrTool      = "tool"
	logAttrSession   = "session"
	logAttrDuration  = "duration"
	logAttrSuccess   = "success"
	logAttrError     = "error"
	logAttrTraceID   = "trace_id"
	logAttrRequestID = "request_id"
)

// WithLogger reports every tool call to logger and returns the runtime for
// chaining. A nil logger reports nothing, which is the default: a library that
// logs before being asked to is noise in somebody else's output.
//
// Pass the logger the request path already uses. The correlation identifiers
// come from the call's context — the trace context a transport extracted and
// the request ID it minted — so an MCP tool call is attributable to the HTTP
// request that carried it.
func (r *Runtime) WithLogger(logger *slog.Logger) *Runtime {
	r.Logger = logger
	return r
}

// logToolCall reports one completed or rejected tool call.
//
// A failure logs at Error because nothing else reports it: a failed tool call
// travels back to the model as a result, not as a transport error, so a
// service with no log here has no signal that its tools are failing.
func (r *Runtime) logToolCall(ctx context.Context, call ToolCall, duration time.Duration, err error, rejected bool) {
	if r.Logger == nil {
		return
	}
	attrs := []slog.Attr{
		slog.String(logAttrTool, call.Name),
		slog.String(logAttrSession, string(call.SessionID)),
		slog.Duration(logAttrDuration, duration),
		slog.Bool(logAttrSuccess, err == nil),
	}
	if traceID := endpoint.TraceIDFromContext(ctx); traceID != "" {
		attrs = append(attrs, slog.String(logAttrTraceID, string(traceID)))
	}
	if requestID := endpoint.RequestIDFromContext(ctx); requestID != "" {
		attrs = append(attrs, slog.String(logAttrRequestID, requestID))
	}
	if err != nil {
		attrs = append(attrs, slog.String(logAttrError, err.Error()))
	}

	level, message := slog.LevelInfo, "tool call succeeded"
	switch {
	case rejected:
		// The tool never ran: a hook refused the call.
		level, message = slog.LevelError, "tool call rejected"
	case err != nil:
		level, message = slog.LevelError, "tool call failed"
	}
	r.Logger.LogAttrs(ctx, level, message, attrs...)
}
