package interaction

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// TestToolCallLogCarriesTheRequestCorrelation is the property: a tool call is
// reported through the caller's logger with the identifiers the request path
// uses, so the two logs join instead of sitting in separate worlds.
func TestToolCallLogCarriesTheRequestCorrelation(t *testing.T) {
	logs := &strings.Builder{}
	runtime := NewRuntime().WithLogger(slog.New(slog.NewJSONHandler(logs, nil)))
	if err := runtime.RegisterTool(ToolFunc{
		ToolName: "search",
		Fn: func(context.Context, ToolCall) (ToolResult, error) {
			return ToolResult{Output: "ok"}, nil
		},
	}); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}

	ctx := endpoint.WithRequestID(
		endpoint.WithTraceID(context.Background(), "4bf92f3577b34da6a3ce929d0e0e4736"),
		"9f1c2d3e4a5b6c7d",
	)
	session, err := runtime.StartSession(ctx, "operator", nil)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if _, err := runtime.CallTool(ctx, ToolCall{SessionID: session.ID, Name: "search"}); err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	record := lastLogRecord(t, logs.String())
	if record["msg"] != "tool call succeeded" {
		t.Fatalf("msg = %v, want %q", record["msg"], "tool call succeeded")
	}
	if record["tool"] != "search" {
		t.Fatalf("tool = %v, want %q", record["tool"], "search")
	}
	if record["trace_id"] != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace_id = %v", record["trace_id"])
	}
	if record["request_id"] != "9f1c2d3e4a5b6c7d" {
		t.Fatalf("request_id = %v", record["request_id"])
	}
	if record["success"] != true {
		t.Fatalf("success = %v, want true", record["success"])
	}
	if _, ok := record["duration"]; !ok {
		t.Fatal("duration was not reported")
	}
	if record["session"] != string(session.ID) {
		t.Fatalf("session = %v, want %q", record["session"], session.ID)
	}
}

func TestFailedToolCallIsReportedAtError(t *testing.T) {
	logs := &strings.Builder{}
	runtime := NewRuntime().WithLogger(slog.New(slog.NewJSONHandler(logs, nil)))
	if err := runtime.RegisterTool(ToolFunc{
		ToolName: "search",
		Fn: func(context.Context, ToolCall) (ToolResult, error) {
			return ToolResult{}, errors.New("index unavailable")
		},
	}); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}
	session, err := runtime.StartSession(context.Background(), "operator", nil)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if _, err := runtime.CallTool(context.Background(), ToolCall{SessionID: session.ID, Name: "search"}); err == nil {
		t.Fatal("CallTool returned no error")
	}

	record := lastLogRecord(t, logs.String())
	if record["msg"] != "tool call failed" || record["level"] != "ERROR" {
		t.Fatalf("record = %v, want a failed call at ERROR", record)
	}
	if record["error"] != "index unavailable" {
		t.Fatalf("error = %v, want %q", record["error"], "index unavailable")
	}
	if record["success"] != false {
		t.Fatalf("success = %v, want false", record["success"])
	}
}

// TestRejectedToolCallIsReported: a call a hook refused never reaches the tool,
// so nothing else would ever mention it.
func TestRejectedToolCallIsReported(t *testing.T) {
	logs := &strings.Builder{}
	runtime := NewRuntime().
		WithLogger(slog.New(slog.NewJSONHandler(logs, nil))).
		WithHooks(HookFuncs{
			Before: func(context.Context, Session, ToolCall) error {
				return ErrUnauthorized
			},
		})
	if err := runtime.RegisterTool(ToolFunc{
		ToolName: "search",
		Fn: func(context.Context, ToolCall) (ToolResult, error) {
			t.Fatal("the tool ran although the call was rejected")
			return ToolResult{}, nil
		},
	}); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}
	session, err := runtime.StartSession(context.Background(), "operator", nil)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if _, err := runtime.CallTool(context.Background(), ToolCall{SessionID: session.ID, Name: "search"}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("CallTool error = %v, want %v", err, ErrUnauthorized)
	}

	record := lastLogRecord(t, logs.String())
	if record["msg"] != "tool call rejected" {
		t.Fatalf("msg = %v, want %q", record["msg"], "tool call rejected")
	}
}

func TestToolCallsAreNotLoggedWithoutALogger(t *testing.T) {
	runtime := NewRuntime()
	if err := runtime.RegisterTool(ToolFunc{
		ToolName: "search",
		Fn: func(context.Context, ToolCall) (ToolResult, error) {
			return ToolResult{Output: "ok"}, nil
		},
	}); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}
	session, err := runtime.StartSession(context.Background(), "operator", nil)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	// The property is only that this does not panic on a nil logger; a runtime
	// nobody asked to log must stay silent.
	if _, err := runtime.CallTool(context.Background(), ToolCall{SessionID: session.ID, Name: "search"}); err != nil {
		t.Fatalf("CallTool: %v", err)
	}
}

func lastLogRecord(t *testing.T, output string) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatal("no log records were written")
	}
	record := map[string]any{}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &record); err != nil {
		t.Fatalf("decode log record %q: %v", lines[len(lines)-1], err)
	}
	return record
}
