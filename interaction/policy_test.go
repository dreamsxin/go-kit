package interaction

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestAuthorizationHookAllowsAndDeniesToolCalls(t *testing.T) {
	rt := NewRuntime(nil, nil, nil, AuthorizationHook{
		Authorizer: AuthorizerFunc(func(ctx context.Context, session Session, call ToolCall) (AuthorizationDecision, error) {
			return AuthorizationDecision{Allowed: call.Name == "allowed", Reason: "blocked tool"}, nil
		}),
	})
	if err := rt.RegisterTool(ToolFunc{ToolName: "allowed", Fn: echoTool}); err != nil {
		t.Fatalf("Register allowed: %v", err)
	}
	if err := rt.RegisterTool(ToolFunc{ToolName: "blocked", Fn: echoTool}); err != nil {
		t.Fatalf("Register blocked: %v", err)
	}
	session, err := rt.StartSession(context.Background(), "agent", nil)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	if _, err := rt.CallTool(context.Background(), ToolCall{SessionID: session.ID, Name: "allowed"}); err != nil {
		t.Fatalf("Call allowed: %v", err)
	}
	_, err = rt.CallTool(context.Background(), ToolCall{SessionID: session.ID, Name: "blocked"})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Call blocked err = %v, want ErrUnauthorized", err)
	}

	events, err := rt.Events.List(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("List events: %v", err)
	}
	got := []EventType{events[0].Type, events[1].Type, events[2].Type}
	want := []EventType{EventSessionStarted, EventToolCall, EventToolResult}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want denied call to be rejected before event emission", got)
	}
}

func TestAuditHookRecordsBeforeAndAfterToolCalls(t *testing.T) {
	var records []AuditRecord
	rt := NewRuntime(nil, nil, nil, AuditHook{
		Sink: AuditSinkFunc(func(ctx context.Context, record AuditRecord) error {
			records = append(records, record)
			return nil
		}),
	})
	if err := rt.RegisterTool(ToolFunc{ToolName: "echo", Fn: echoTool}); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}
	session, err := rt.StartSession(context.Background(), "agent", nil)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	_, err = rt.CallTool(context.Background(), ToolCall{
		SessionID: session.ID,
		Name:      "echo",
		Input:     "hello",
		Metadata:  map[string]string{"trace": "abc"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("audit records length = %d, want 2", len(records))
	}
	if records[0].Phase != "before" || records[0].Tool != "echo" || !records[0].Allowed {
		t.Fatalf("unexpected before record: %+v", records[0])
	}
	if records[1].Phase != "after" || records[1].Tool != "echo" || !records[1].Allowed {
		t.Fatalf("unexpected after record: %+v", records[1])
	}
	if records[0].Metadata["trace"] != "abc" {
		t.Fatalf("metadata = %+v, want trace=abc", records[0].Metadata)
	}
}

func TestAuditHookRecordsToolErrors(t *testing.T) {
	var records []AuditRecord
	rt := NewRuntime(nil, nil, nil, AuditHook{
		Sink: AuditSinkFunc(func(ctx context.Context, record AuditRecord) error {
			records = append(records, record)
			return nil
		}),
	})
	if err := rt.RegisterTool(ToolFunc{
		ToolName: "fail",
		Fn: func(context.Context, ToolCall) (ToolResult, error) {
			return ToolResult{}, errors.New("boom")
		},
	}); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}
	session, err := rt.StartSession(context.Background(), "agent", nil)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	_, _ = rt.CallTool(context.Background(), ToolCall{SessionID: session.ID, Name: "fail"})
	if len(records) != 2 {
		t.Fatalf("audit records length = %d, want 2", len(records))
	}
	if records[1].Allowed || records[1].Error != "boom" {
		t.Fatalf("unexpected error audit record: %+v", records[1])
	}
}

func echoTool(ctx context.Context, call ToolCall) (ToolResult, error) {
	return ToolResult{Output: call.Input, Metadata: map[string]string{"tool": call.Name}}, nil
}
