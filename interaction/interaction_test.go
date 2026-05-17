package interaction

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestRuntimeSessionLifecycleAndEvents(t *testing.T) {
	rt := NewRuntime(nil, nil, nil)

	session, err := rt.StartSession(context.Background(), "agent-1", map[string]string{"trace": "abc"})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if session.ID == "" || session.Subject != "agent-1" {
		t.Fatalf("unexpected session: %+v", session)
	}

	closed, err := rt.EndSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("EndSession: %v", err)
	}
	if !closed.Closed() {
		t.Fatalf("session should be closed: %+v", closed)
	}

	events, err := rt.Events.List(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("List events: %v", err)
	}
	got := []EventType{events[0].Type, events[1].Type}
	want := []EventType{EventSessionStarted, EventSessionEnded}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestRuntimeToolCallRunsHooksAndRecordsEvents(t *testing.T) {
	var hookOrder []string
	rt := NewRuntime(nil, nil, nil, HookFuncs{
		Before: func(ctx context.Context, session Session, call ToolCall) error {
			hookOrder = append(hookOrder, "before:"+call.Name)
			return nil
		},
		After: func(ctx context.Context, session Session, call ToolCall, result ToolResult, err error) error {
			hookOrder = append(hookOrder, "after:"+call.Name)
			return nil
		},
	})
	if err := rt.RegisterTool(ToolFunc{
		ToolName: "echo",
		Fn: func(ctx context.Context, call ToolCall) (ToolResult, error) {
			return ToolResult{Output: call.Input}, nil
		},
	}); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}

	session, err := rt.StartSession(context.Background(), "agent", nil)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	result, err := rt.CallTool(context.Background(), ToolCall{
		SessionID: session.ID,
		Name:      "echo",
		Input:     "hello",
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.Output != "hello" {
		t.Fatalf("Output = %v, want hello", result.Output)
	}
	if !reflect.DeepEqual(hookOrder, []string{"before:echo", "after:echo"}) {
		t.Fatalf("hook order = %v", hookOrder)
	}

	events, err := rt.Events.List(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("List events: %v", err)
	}
	got := []EventType{events[0].Type, events[1].Type, events[2].Type}
	want := []EventType{EventSessionStarted, EventToolCall, EventToolResult}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestRuntimeRejectsClosedSessionToolCall(t *testing.T) {
	rt := NewRuntime(nil, nil, nil)
	session, err := rt.StartSession(context.Background(), "agent", nil)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if _, err := rt.EndSession(context.Background(), session.ID); err != nil {
		t.Fatalf("EndSession: %v", err)
	}

	_, err = rt.CallTool(context.Background(), ToolCall{SessionID: session.ID, Name: "missing"})
	if !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("CallTool err = %v, want ErrSessionClosed", err)
	}
}

func TestToolRegistryValidation(t *testing.T) {
	registry := NewMemoryToolRegistry()
	if err := registry.Register(nil); !errors.Is(err, ErrNilTool) {
		t.Fatalf("Register nil err = %v, want ErrNilTool", err)
	}
	if err := registry.Register(ToolFunc{}); !errors.Is(err, ErrEmptyToolName) {
		t.Fatalf("Register empty err = %v, want ErrEmptyToolName", err)
	}
	tool := ToolFunc{ToolName: "echo", Fn: func(context.Context, ToolCall) (ToolResult, error) {
		return ToolResult{}, nil
	}}
	if err := registry.Register(tool); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := registry.Register(tool); !errors.Is(err, ErrToolExists) {
		t.Fatalf("Register duplicate err = %v, want ErrToolExists", err)
	}
	if _, err := registry.Call(context.Background(), ToolCall{Name: "missing"}); !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("Call missing err = %v, want ErrToolNotFound", err)
	}
}

func TestMemoryStoresDefensivelyCopyMetadata(t *testing.T) {
	store := NewMemorySessionStore()
	meta := map[string]string{"k": "v"}
	session, err := store.Create(context.Background(), "subject", meta)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	meta["k"] = "changed"

	got, err := store.Get(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Metadata["k"] != "v" {
		t.Fatalf("metadata was not copied: %+v", got.Metadata)
	}
	got.Metadata["k"] = "mutated"

	again, err := store.Get(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("Get again: %v", err)
	}
	if again.Metadata["k"] != "v" {
		t.Fatalf("metadata leaked mutation: %+v", again.Metadata)
	}
}
