package interaction

import (
	"context"
	"errors"
	"testing"
	"time"
)

// --- Session ---

func TestSession_Closed(t *testing.T) {
	s := Session{ID: "s1", Subject: "user"}
	if s.Closed() {
		t.Fatal("new session should not be closed")
	}

	s.ClosedAt = time.Now()
	if !s.Closed() {
		t.Fatal("session with ClosedAt set should be closed")
	}
}

// --- ToolFunc ---

func TestToolFunc_Name(t *testing.T) {
	f := ToolFunc{ToolName: "echo"}
	if f.Name() != "echo" {
		t.Fatalf("expected name 'echo', got %q", f.Name())
	}
}

func TestToolFunc_Descriptor(t *testing.T) {
	f := ToolFunc{
		ToolName:    "echo",
		Description: "Echoes input",
		Schema:      map[string]string{"type": "object"},
	}
	d := f.Descriptor()
	if d.Name != "echo" {
		t.Fatalf("expected name 'echo', got %q", d.Name)
	}
	if d.Description != "Echoes input" {
		t.Fatalf("expected description 'Echoes input', got %q", d.Description)
	}
	if d.InputSchema == nil {
		t.Fatal("expected non-nil InputSchema")
	}
}

func TestToolFunc_Call_Success(t *testing.T) {
	f := ToolFunc{
		ToolName: "echo",
		Fn: func(_ context.Context, call ToolCall) (ToolResult, error) {
			return ToolResult{Output: call.Input}, nil
		},
	}
	result, err := f.Call(context.Background(), ToolCall{Input: "hello"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Output != "hello" {
		t.Fatalf("expected output 'hello', got %v", result.Output)
	}
}

func TestToolFunc_Call_NilFn(t *testing.T) {
	f := ToolFunc{ToolName: "noop"}
	_, err := f.Call(context.Background(), ToolCall{})
	if !errors.Is(err, ErrNilToolFunc) {
		t.Fatalf("expected ErrNilToolFunc, got %v", err)
	}
}

func TestToolFunc_Call_PropagatesError(t *testing.T) {
	toolErr := errors.New("tool failure")
	f := ToolFunc{
		ToolName: "fail",
		Fn: func(_ context.Context, _ ToolCall) (ToolResult, error) {
			return ToolResult{}, toolErr
		},
	}
	_, err := f.Call(context.Background(), ToolCall{})
	if !errors.Is(err, toolErr) {
		t.Fatalf("expected tool error, got %v", err)
	}
}

// --- ToolFunc implements ToolDescriber ---

func TestToolFunc_ImplementsToolDescriber(t *testing.T) {
	f := ToolFunc{ToolName: "echo", Description: "Echo"}
	var _ ToolDescriber = f // compile-time check
}

// --- Event constants ---

func TestEventTypes(t *testing.T) {
	tests := []struct {
		got  EventType
		want string
	}{
		{EventSessionStarted, "session.started"},
		{EventSessionEnded, "session.ended"},
		{EventToolCall, "tool.call"},
		{EventToolResult, "tool.result"},
		{EventError, "error"},
		{EventMessage, "message"},
	}
	for _, tt := range tests {
		if string(tt.got) != tt.want {
			t.Fatalf("expected %q, got %q", tt.want, string(tt.got))
		}
	}
}

// --- MemorySessionStore ---

func TestMemorySessionStore_CreateAndGet(t *testing.T) {
	store := NewMemorySessionStore()
	ctx := context.Background()

	session, err := store.Create(ctx, "user-1", map[string]string{"role": "admin"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if session.ID == "" {
		t.Fatal("session ID should not be empty")
	}
	if session.Subject != "user-1" {
		t.Fatalf("expected subject 'user-1', got %q", session.Subject)
	}
	if session.Metadata["role"] != "admin" {
		t.Fatalf("expected metadata role=admin, got %q", session.Metadata["role"])
	}

	got, err := store.Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != session.ID {
		t.Fatalf("expected ID %q, got %q", session.ID, got.ID)
	}
}

func TestMemorySessionStore_Get_NotFound(t *testing.T) {
	store := NewMemorySessionStore()
	_, err := store.Get(context.Background(), "nonexistent")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestMemorySessionStore_Close(t *testing.T) {
	store := NewMemorySessionStore()
	ctx := context.Background()

	session, _ := store.Create(ctx, "user", nil)
	closed, err := store.Close(ctx, session.ID)
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !closed.Closed() {
		t.Fatal("session should be closed")
	}

	// Double close is idempotent
	closed2, err := store.Close(ctx, session.ID)
	if err != nil {
		t.Fatalf("Double Close: %v", err)
	}
	if !closed2.Closed() {
		t.Fatal("double closed session should still be closed")
	}
}

func TestMemorySessionStore_Close_NotFound(t *testing.T) {
	store := NewMemorySessionStore()
	_, err := store.Close(context.Background(), "nonexistent")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestMemorySessionStore_MetadataCloned(t *testing.T) {
	store := NewMemorySessionStore()
	original := map[string]string{"key": "val"}
	session, _ := store.Create(context.Background(), "user", original)

	original["key"] = "mutated"
	got, _ := store.Get(context.Background(), session.ID)
	if got.Metadata["key"] != "val" {
		t.Fatal("session metadata should be cloned on Create")
	}
}

// --- MemoryEventSink ---

func TestMemoryEventSink_EmitAndList(t *testing.T) {
	sink := NewMemoryEventSink()
	ctx := context.Background()

	err := sink.Emit(ctx, Event{SessionID: "s1", Type: EventMessage, Name: "test"})
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}

	events, err := sink.List(ctx, "s1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventMessage {
		t.Fatalf("expected EventMessage, got %s", events[0].Type)
	}
	if events[0].At.IsZero() {
		t.Fatal("event At should be auto-set")
	}
}

func TestMemoryEventSink_List_Empty(t *testing.T) {
	sink := NewMemoryEventSink()
	events, err := sink.List(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

func TestMemoryEventSink_MetadataCloned(t *testing.T) {
	sink := NewMemoryEventSink()
	meta := map[string]string{"key": "val"}
	sink.Emit(context.Background(), Event{SessionID: "s1", Type: EventMessage, Metadata: meta})

	meta["key"] = "mutated"
	events, _ := sink.List(context.Background(), "s1")
	if events[0].Metadata["key"] != "val" {
		t.Fatal("event metadata should be cloned on Emit")
	}
}

// --- MemoryToolRegistry ---

func TestMemoryToolRegistry_RegisterAndGet(t *testing.T) {
	reg := NewMemoryToolRegistry()
	tool := ToolFunc{
		ToolName: "echo",
		Fn:       func(_ context.Context, call ToolCall) (ToolResult, error) { return ToolResult{Output: call.Input}, nil },
	}
	if err := reg.Register(tool); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, ok := reg.Get("echo")
	if !ok {
		t.Fatal("expected tool to be found")
	}
	if got.Name() != "echo" {
		t.Fatalf("expected name 'echo', got %q", got.Name())
	}
}

func TestMemoryToolRegistry_Register_NilTool(t *testing.T) {
	reg := NewMemoryToolRegistry()
	err := reg.Register(nil)
	if !errors.Is(err, ErrNilTool) {
		t.Fatalf("expected ErrNilTool, got %v", err)
	}
}

func TestMemoryToolRegistry_Register_EmptyName(t *testing.T) {
	reg := NewMemoryToolRegistry()
	err := reg.Register(ToolFunc{})
	if !errors.Is(err, ErrEmptyToolName) {
		t.Fatalf("expected ErrEmptyToolName, got %v", err)
	}
}

func TestMemoryToolRegistry_Register_Duplicate(t *testing.T) {
	reg := NewMemoryToolRegistry()
	reg.Register(ToolFunc{ToolName: "echo", Fn: func(_ context.Context, _ ToolCall) (ToolResult, error) { return ToolResult{}, nil }})
	err := reg.Register(ToolFunc{ToolName: "echo", Fn: func(_ context.Context, _ ToolCall) (ToolResult, error) { return ToolResult{}, nil }})
	if !errors.Is(err, ErrToolExists) {
		t.Fatalf("expected ErrToolExists, got %v", err)
	}
}

func TestMemoryToolRegistry_Call_NotFound(t *testing.T) {
	reg := NewMemoryToolRegistry()
	_, err := reg.Call(context.Background(), ToolCall{Name: "nonexistent"})
	if !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("expected ErrToolNotFound, got %v", err)
	}
}

func TestMemoryToolRegistry_List_SortedByName(t *testing.T) {
	reg := NewMemoryToolRegistry()
	mkTool := func(name string) ToolFunc {
		return ToolFunc{ToolName: name, Description: name + " desc", Fn: func(_ context.Context, _ ToolCall) (ToolResult, error) { return ToolResult{}, nil }}
	}
	reg.Register(mkTool("zebra"))
	reg.Register(mkTool("alpha"))
	reg.Register(mkTool("middle"))

	descs := reg.List()
	if len(descs) != 3 {
		t.Fatalf("expected 3 descriptors, got %d", len(descs))
	}
	if descs[0].Name != "alpha" {
		t.Fatalf("expected first name 'alpha', got %q", descs[0].Name)
	}
	if descs[1].Name != "middle" {
		t.Fatalf("expected second name 'middle', got %q", descs[1].Name)
	}
	if descs[2].Name != "zebra" {
		t.Fatalf("expected third name 'zebra', got %q", descs[2].Name)
	}
}

func TestMemoryToolRegistry_List_ToolWithoutDescriber(t *testing.T) {
	reg := NewMemoryToolRegistry()
	// Use a tool that implements Tool but NOT ToolDescriber
	reg.Register(&plainTool{name: "simple"})

	descs := reg.List()
	if len(descs) != 1 {
		t.Fatalf("expected 1 descriptor, got %d", len(descs))
	}
	if descs[0].Name != "simple" {
		t.Fatalf("expected name 'simple', got %q", descs[0].Name)
	}
	if descs[0].Description != "" {
		t.Fatalf("expected empty description for plain tool, got %q", descs[0].Description)
	}
}

// plainTool implements Tool but NOT ToolDescriber.
type plainTool struct{ name string }

func (p *plainTool) Name() string { return p.name }
func (p *plainTool) Call(_ context.Context, _ ToolCall) (ToolResult, error) {
	return ToolResult{}, nil
}
