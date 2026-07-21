package interaction

import (
	"context"
	"errors"
	"testing"
)

// --- NewRuntime defaults ---

func TestNewRuntime_Defaults(t *testing.T) {
	rt := NewRuntime()
	if rt.Sessions == nil {
		t.Fatal("Sessions should default to MemorySessionStore")
	}
	if rt.Events == nil {
		t.Fatal("Events should default to MemoryEventSink")
	}
	if rt.Tools == nil {
		t.Fatal("Tools should default to MemoryToolRegistry")
	}
	if rt.Hooks != nil {
		t.Fatal("Hooks should default to nil")
	}
	if rt.Resources != nil {
		t.Fatal("Resources should default to nil")
	}
	if rt.Prompts != nil {
		t.Fatal("Prompts should default to nil")
	}
}

// --- With* chaining methods ---

func TestRuntime_WithSessions(t *testing.T) {
	rt := NewRuntime()
	store := NewMemorySessionStore()
	got := rt.WithSessions(store)
	if got != rt {
		t.Fatal("WithSessions should return the same runtime for chaining")
	}
	if rt.Sessions != store {
		t.Fatal("WithSessions should replace the SessionStore")
	}
}

func TestRuntime_WithEvents(t *testing.T) {
	rt := NewRuntime()
	sink := NewMemoryEventSink()
	got := rt.WithEvents(sink)
	if got != rt {
		t.Fatal("WithEvents should return the same runtime for chaining")
	}
	if rt.Events != sink {
		t.Fatal("WithEvents should replace the EventSink")
	}
}

func TestRuntime_WithTools(t *testing.T) {
	rt := NewRuntime()
	reg := NewMemoryToolRegistry()
	got := rt.WithTools(reg)
	if got != rt {
		t.Fatal("WithTools should return the same runtime for chaining")
	}
	if rt.Tools != reg {
		t.Fatal("WithTools should replace the ToolRegistry")
	}
}

func TestRuntime_WithHooks_Append(t *testing.T) {
	rt := NewRuntime()
	h1 := HookFuncs{}
	h2 := HookFuncs{}
	rt.WithHooks(h1)
	if len(rt.Hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(rt.Hooks))
	}
	rt.WithHooks(h2)
	if len(rt.Hooks) != 2 {
		t.Fatalf("expected 2 hooks (append), got %d", len(rt.Hooks))
	}
}

func TestRuntime_WithResources(t *testing.T) {
	rt := NewRuntime()
	prov := NewMemoryResourceProvider()
	got := rt.WithResources(prov)
	if got != rt {
		t.Fatal("WithResources should return the same runtime for chaining")
	}
	if rt.Resources != prov {
		t.Fatal("WithResources should set the ResourceProvider")
	}
}

func TestRuntime_WithPrompts(t *testing.T) {
	rt := NewRuntime()
	prov := NewMemoryPromptProvider()
	got := rt.WithPrompts(prov)
	if got != rt {
		t.Fatal("WithPrompts should return the same runtime for chaining")
	}
	if rt.Prompts != prov {
		t.Fatal("WithPrompts should set the PromptProvider")
	}
}

func TestRuntime_Chaining(t *testing.T) {
	rt := NewRuntime().
		WithHooks(HookFuncs{}).
		WithResources(NewMemoryResourceProvider()).
		WithPrompts(NewMemoryPromptProvider())
	if len(rt.Hooks) != 1 {
		t.Fatal("chaining should accumulate hooks")
	}
	if rt.Resources == nil {
		t.Fatal("chaining should set resources")
	}
	if rt.Prompts == nil {
		t.Fatal("chaining should set prompts")
	}
}

// --- RegisterResource ---

func TestRuntime_RegisterResource_LazyInit(t *testing.T) {
	rt := NewRuntime()
	if rt.Resources != nil {
		t.Fatal("Resources should start nil")
	}
	err := rt.RegisterResource(
		Resource{URI: "file:///test", Name: "test"},
		[]ResourceContent{{URI: "file:///test", Text: "hello"}},
	)
	if err != nil {
		t.Fatalf("RegisterResource: %v", err)
	}
	if rt.Resources == nil {
		t.Fatal("RegisterResource should lazy-init Resources")
	}

	// Verify the resource was actually registered
	resources, err := rt.ListResources(context.Background())
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
}

func TestRuntime_RegisterResource_WrongType(t *testing.T) {
	// Set Resources to a non-MemoryResourceProvider implementation
	rt := NewRuntime()
	rt.Resources = &mockResourceProvider{}
	err := rt.RegisterResource(
		Resource{URI: "file:///test", Name: "test"},
		nil,
	)
	if err == nil {
		t.Fatal("expected error when Resources is not *MemoryResourceProvider")
	}
	if !contains(err.Error(), "not a *MemoryResourceProvider") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- RegisterPrompt ---

func TestRuntime_RegisterPrompt_LazyInit(t *testing.T) {
	rt := NewRuntime()
	if rt.Prompts != nil {
		t.Fatal("Prompts should start nil")
	}
	err := rt.RegisterPrompt(
		Prompt{Name: "greet"},
		func(args map[string]string) (PromptResult, error) {
			return PromptResult{Description: "hi"}, nil
		},
	)
	if err != nil {
		t.Fatalf("RegisterPrompt: %v", err)
	}
	if rt.Prompts == nil {
		t.Fatal("RegisterPrompt should lazy-init Prompts")
	}

	prompts, err := rt.ListPrompts(context.Background())
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	if len(prompts) != 1 {
		t.Fatalf("expected 1 prompt, got %d", len(prompts))
	}
}

func TestRuntime_RegisterPrompt_WrongType(t *testing.T) {
	rt := NewRuntime()
	rt.Prompts = &mockPromptProvider{}
	err := rt.RegisterPrompt(Prompt{Name: "test"}, nil)
	if err == nil {
		t.Fatal("expected error when Prompts is not *MemoryPromptProvider")
	}
	if !contains(err.Error(), "not a *MemoryPromptProvider") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- StartSession / EndSession ---

func TestRuntime_StartSession(t *testing.T) {
	rt := NewRuntime()
	ctx := context.Background()

	session, err := rt.StartSession(ctx, "user-1", map[string]string{"role": "admin"})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if session.Subject != "user-1" {
		t.Fatalf("expected subject 'user-1', got %q", session.Subject)
	}
	if session.ID == "" {
		t.Fatal("session ID should not be empty")
	}

	// Verify session.started event was emitted
	events, err := rt.Events.List(ctx, session.ID)
	if err != nil {
		t.Fatalf("Events.List: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventSessionStarted {
		t.Fatalf("expected EventSessionStarted, got %s", events[0].Type)
	}
}

func TestRuntime_EndSession(t *testing.T) {
	rt := NewRuntime()
	ctx := context.Background()

	session, err := rt.StartSession(ctx, "user-1", nil)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	closed, err := rt.EndSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("EndSession: %v", err)
	}
	if !closed.Closed() {
		t.Fatal("session should be closed after EndSession")
	}

	// Verify session.ended event was emitted (total 2 events: started + ended)
	events, err := rt.Events.List(ctx, session.ID)
	if err != nil {
		t.Fatalf("Events.List: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[1].Type != EventSessionEnded {
		t.Fatalf("expected EventSessionEnded, got %s", events[1].Type)
	}
}

func TestRuntime_ReleaseSessionDeletesMemoryRecord(t *testing.T) {
	rt := NewRuntime()
	session, err := rt.StartSession(context.Background(), "subject", nil)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if err := rt.ReleaseSession(context.Background(), session.ID); err != nil {
		t.Fatalf("ReleaseSession: %v", err)
	}
	if _, err := rt.Sessions.Get(context.Background(), session.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("Get after ReleaseSession = %v, want ErrSessionNotFound", err)
	}
	events, err := rt.Events.List(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("List events: %v", err)
	}
	if len(events) != 2 || events[1].Type != EventSessionEnded {
		t.Fatalf("events = %#v, want start and end", events)
	}
}

func TestRuntime_StartSession_CancelledContext(t *testing.T) {
	rt := NewRuntime()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := rt.StartSession(ctx, "user-1", nil)
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

// --- RegisterTool / ListTools ---

func TestRuntime_RegisterTool_And_ListTools(t *testing.T) {
	rt := NewRuntime()
	tool := ToolFunc{
		ToolName:    "echo",
		Description: "Echoes input",
		Fn: func(_ context.Context, call ToolCall) (ToolResult, error) {
			return ToolResult{Output: call.Input}, nil
		},
	}
	if err := rt.RegisterTool(tool); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}

	descs := rt.ListTools()
	if len(descs) != 1 {
		t.Fatalf("expected 1 tool descriptor, got %d", len(descs))
	}
	if descs[0].Name != "echo" {
		t.Fatalf("expected tool name 'echo', got %q", descs[0].Name)
	}
	if descs[0].Description != "Echoes input" {
		t.Fatalf("expected description 'Echoes input', got %q", descs[0].Description)
	}
}

func TestRuntime_ListTools_NoToolLister(t *testing.T) {
	rt := NewRuntime()
	// Replace Tools with a registry that doesn't implement ToolLister
	rt.Tools = &minimalRegistry{}
	descs := rt.ListTools()
	if descs != nil {
		t.Fatalf("expected nil when Tools doesn't implement ToolLister, got %v", descs)
	}
}

// --- CallTool ---

func TestRuntime_CallTool_FullLifecycle(t *testing.T) {
	rt := NewRuntime()
	ctx := context.Background()

	tool := ToolFunc{
		ToolName: "echo",
		Fn: func(_ context.Context, call ToolCall) (ToolResult, error) {
			return ToolResult{Output: call.Input}, nil
		},
	}
	if err := rt.RegisterTool(tool); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}

	session, err := rt.StartSession(ctx, "tester", nil)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	result, err := rt.CallTool(ctx, ToolCall{
		SessionID: session.ID,
		Name:      "echo",
		Input:     "hello",
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.Output != "hello" {
		t.Fatalf("expected output 'hello', got %v", result.Output)
	}

	// Verify events: session.started + tool.call + tool.result = 3
	events, err := rt.Events.List(ctx, session.ID)
	if err != nil {
		t.Fatalf("Events.List: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[1].Type != EventToolCall {
		t.Fatalf("expected EventToolCall, got %s", events[1].Type)
	}
	if events[2].Type != EventToolResult {
		t.Fatalf("expected EventToolResult, got %s", events[2].Type)
	}
}

func TestRuntime_CallTool_SessionClosed(t *testing.T) {
	rt := NewRuntime()
	ctx := context.Background()

	session, err := rt.StartSession(ctx, "tester", nil)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	_, err = rt.EndSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("EndSession: %v", err)
	}

	_, err = rt.CallTool(ctx, ToolCall{SessionID: session.ID, Name: "x"})
	if !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("expected ErrSessionClosed, got %v", err)
	}
}

func TestRuntime_CallTool_SessionNotFound(t *testing.T) {
	rt := NewRuntime()
	_, err := rt.CallTool(context.Background(), ToolCall{SessionID: "nonexistent", Name: "x"})
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestRuntime_CallTool_HookBeforeDeny(t *testing.T) {
	rt := NewRuntime()
	ctx := context.Background()

	denyErr := errors.New("denied")
	rt.WithHooks(HookFuncs{
		Before: func(_ context.Context, _ Session, _ ToolCall) error {
			return denyErr
		},
	})

	tool := ToolFunc{ToolName: "echo", Fn: func(_ context.Context, _ ToolCall) (ToolResult, error) {
		t.Fatal("tool should not be called when hook denies")
		return ToolResult{}, nil
	}}
	rt.RegisterTool(tool)

	session, _ := rt.StartSession(ctx, "tester", nil)
	_, err := rt.CallTool(ctx, ToolCall{SessionID: session.ID, Name: "echo"})
	if !errors.Is(err, denyErr) {
		t.Fatalf("expected deny error, got %v", err)
	}
}

func TestRuntime_CallTool_HookAfterError(t *testing.T) {
	rt := NewRuntime()
	ctx := context.Background()

	afterErr := errors.New("after-error")
	rt.WithHooks(HookFuncs{
		After: func(_ context.Context, _ Session, _ ToolCall, _ ToolResult, _ error) error {
			return afterErr
		},
	})

	tool := ToolFunc{ToolName: "echo", Fn: func(_ context.Context, _ ToolCall) (ToolResult, error) {
		return ToolResult{Output: "ok"}, nil
	}}
	rt.RegisterTool(tool)

	session, _ := rt.StartSession(ctx, "tester", nil)
	_, err := rt.CallTool(ctx, ToolCall{SessionID: session.ID, Name: "echo"})
	if !errors.Is(err, afterErr) {
		t.Fatalf("expected after-error, got %v", err)
	}
}

func TestRuntime_CallTool_ToolError_EmitsErrorEvent(t *testing.T) {
	rt := NewRuntime()
	ctx := context.Background()

	toolErr := errors.New("tool failure")
	tool := ToolFunc{ToolName: "fail", Fn: func(_ context.Context, _ ToolCall) (ToolResult, error) {
		return ToolResult{}, toolErr
	}}
	rt.RegisterTool(tool)

	session, _ := rt.StartSession(ctx, "tester", nil)
	_, err := rt.CallTool(ctx, ToolCall{SessionID: session.ID, Name: "fail"})
	if !errors.Is(err, toolErr) {
		t.Fatalf("expected tool error, got %v", err)
	}

	// Events: started + tool.call + tool.result + error = 4
	events, _ := rt.Events.List(ctx, session.ID)
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}
	if events[3].Type != EventError {
		t.Fatalf("expected EventError, got %s", events[3].Type)
	}
}

// --- emit with OnEmitError ---

func TestRuntime_Emit_OnEmitError(t *testing.T) {
	rt := NewRuntime()
	var captured error
	rt.OnEmitError = func(err error) {
		captured = err
	}

	// Use a failing event sink
	rt.Events = &failingEventSink{err: errors.New("emit failed")}
	rt.emit(context.Background(), Event{Type: EventMessage})
	if captured == nil {
		t.Fatal("OnEmitError should have been called")
	}
	if captured.Error() != "emit failed" {
		t.Fatalf("unexpected error: %v", captured)
	}
}

func TestRuntime_Emit_OnEmitError_Nil(t *testing.T) {
	rt := NewRuntime()
	// OnEmitError is nil — should not panic
	rt.Events = &failingEventSink{err: errors.New("emit failed")}
	rt.emit(context.Background(), Event{Type: EventMessage})
	// No panic = pass
}

func TestRuntime_Emit_Success(t *testing.T) {
	rt := NewRuntime()
	var called bool
	rt.OnEmitError = func(err error) {
		called = true
	}
	rt.emit(context.Background(), Event{Type: EventMessage, SessionID: "s1"})
	if called {
		t.Fatal("OnEmitError should NOT be called on success")
	}
}

// --- ListResources / ReadResource / ListResourceTemplates ---

func TestRuntime_ListResources_NilProvider(t *testing.T) {
	rt := NewRuntime()
	resources, err := rt.ListResources(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resources != nil {
		t.Fatalf("expected nil, got %v", resources)
	}
}

func TestRuntime_ReadResource_NilProvider(t *testing.T) {
	rt := NewRuntime()
	_, err := rt.ReadResource(context.Background(), "file:///test")
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("expected ErrResourceNotFound, got %v", err)
	}
}

func TestRuntime_ListResourceTemplates_NilProvider(t *testing.T) {
	rt := NewRuntime()
	templates, err := rt.ListResourceTemplates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if templates != nil {
		t.Fatal("expected nil templates with nil provider")
	}
}

func TestRuntime_ListResourceTemplates_NoTemplateLister(t *testing.T) {
	rt := NewRuntime()
	// Use a provider that doesn't implement ResourceTemplateLister
	rt.Resources = &mockResourceProvider{}
	templates, err := rt.ListResourceTemplates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if templates != nil {
		t.Fatal("expected nil templates when provider doesn't implement ResourceTemplateLister")
	}
}

func TestRuntime_ListResourceTemplates_WithProvider(t *testing.T) {
	rt := NewRuntime()
	prov := NewMemoryResourceProvider()
	prov.SetTemplates([]ResourceTemplate{
		{URITemplate: "file:///{name}", Name: "dynamic"},
	})
	rt.Resources = prov

	templates, err := rt.ListResourceTemplates(context.Background())
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	if len(templates) != 1 {
		t.Fatalf("expected 1 template, got %d", len(templates))
	}
	if templates[0].Name != "dynamic" {
		t.Fatalf("expected template name 'dynamic', got %q", templates[0].Name)
	}
}

// --- ListPrompts / GetPrompt / CompletePromptArgument ---

func TestRuntime_ListPrompts_NilProvider(t *testing.T) {
	rt := NewRuntime()
	prompts, err := rt.ListPrompts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompts != nil {
		t.Fatal("expected nil prompts with nil provider")
	}
}

func TestRuntime_GetPrompt_NilProvider(t *testing.T) {
	rt := NewRuntime()
	_, err := rt.GetPrompt(context.Background(), "test", nil)
	if !errors.Is(err, ErrPromptNotFound) {
		t.Fatalf("expected ErrPromptNotFound, got %v", err)
	}
}

func TestRuntime_CompletePromptArgument_NilProvider(t *testing.T) {
	rt := NewRuntime()
	_, err := rt.CompletePromptArgument(context.Background(), "p", "a", "v")
	if !errors.Is(err, ErrCompletionUnsupported) {
		t.Fatalf("expected ErrCompletionUnsupported, got %v", err)
	}
}

func TestRuntime_CompletePromptArgument_NoCompleter(t *testing.T) {
	rt := NewRuntime()
	// Use a provider that doesn't implement PromptCompleter
	rt.Prompts = &mockPromptProvider{}
	_, err := rt.CompletePromptArgument(context.Background(), "p", "a", "v")
	if !errors.Is(err, ErrCompletionUnsupported) {
		t.Fatalf("expected ErrCompletionUnsupported, got %v", err)
	}
}

func TestRuntime_CompletePromptArgument_WithCompleter(t *testing.T) {
	rt := NewRuntime()
	prov := NewMemoryPromptProvider()
	prov.Register(Prompt{Name: "greet"}, func(args map[string]string) (PromptResult, error) {
		return PromptResult{}, nil
	})
	rt.Prompts = prov

	result, err := rt.CompletePromptArgument(context.Background(), "greet", "name", "he")
	if err != nil {
		t.Fatalf("CompletePromptArgument: %v", err)
	}
	// Default implementation returns empty suggestions
	if len(result.Values) != 0 {
		t.Fatalf("expected empty values, got %v", result.Values)
	}
}

// --- HookFuncs nil-safety ---

func TestHookFuncs_NilBefore(t *testing.T) {
	h := HookFuncs{}
	err := h.BeforeToolCall(context.Background(), Session{}, ToolCall{})
	if err != nil {
		t.Fatalf("nil Before should return nil, got %v", err)
	}
}

func TestHookFuncs_NilAfter(t *testing.T) {
	h := HookFuncs{}
	err := h.AfterToolCall(context.Background(), Session{}, ToolCall{}, ToolResult{}, nil)
	if err != nil {
		t.Fatalf("nil After should return nil, got %v", err)
	}
}

func TestHookFuncs_WithBefore(t *testing.T) {
	called := false
	h := HookFuncs{
		Before: func(_ context.Context, _ Session, _ ToolCall) error {
			called = true
			return nil
		},
	}
	h.BeforeToolCall(context.Background(), Session{}, ToolCall{})
	if !called {
		t.Fatal("Before should have been called")
	}
}

func TestHookFuncs_WithAfter(t *testing.T) {
	called := false
	h := HookFuncs{
		After: func(_ context.Context, _ Session, _ ToolCall, _ ToolResult, _ error) error {
			called = true
			return nil
		},
	}
	h.AfterToolCall(context.Background(), Session{}, ToolCall{}, ToolResult{}, nil)
	if !called {
		t.Fatal("After should have been called")
	}
}

// --- CallTool with multiple hooks (reverse order AfterToolCall) ---

func TestRuntime_CallTool_HooksReverseOrder(t *testing.T) {
	rt := NewRuntime()
	ctx := context.Background()

	var order []string
	h1 := HookFuncs{
		Before: func(_ context.Context, _ Session, _ ToolCall) error {
			order = append(order, "before-1")
			return nil
		},
		After: func(_ context.Context, _ Session, _ ToolCall, _ ToolResult, _ error) error {
			order = append(order, "after-1")
			return nil
		},
	}
	h2 := HookFuncs{
		Before: func(_ context.Context, _ Session, _ ToolCall) error {
			order = append(order, "before-2")
			return nil
		},
		After: func(_ context.Context, _ Session, _ ToolCall, _ ToolResult, _ error) error {
			order = append(order, "after-2")
			return nil
		},
	}
	rt.WithHooks(h1, h2)

	tool := ToolFunc{ToolName: "noop", Fn: func(_ context.Context, _ ToolCall) (ToolResult, error) {
		return ToolResult{}, nil
	}}
	rt.RegisterTool(tool)
	session, _ := rt.StartSession(ctx, "tester", nil)
	rt.CallTool(ctx, ToolCall{SessionID: session.ID, Name: "noop"})

	expected := []string{"before-1", "before-2", "after-2", "after-1"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d hook calls, got %d: %v", len(expected), len(order), order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Fatalf("order[%d] = %q, want %q (full: %v)", i, order[i], v, order)
		}
	}
}

// --- Helpers ---

type failingEventSink struct {
	err error
}

func (s *failingEventSink) Emit(_ context.Context, _ Event) error {
	return s.err
}

func (s *failingEventSink) List(_ context.Context, _ SessionID) ([]Event, error) {
	return nil, nil
}

// mockResourceProvider implements ResourceProvider but NOT ResourceTemplateLister.
type mockResourceProvider struct{}

func (m *mockResourceProvider) ListResources(_ context.Context) ([]Resource, error) {
	return nil, nil
}

func (m *mockResourceProvider) ReadResource(_ context.Context, _ string) ([]ResourceContent, error) {
	return nil, ErrResourceNotFound
}

// mockPromptProvider implements PromptProvider but NOT PromptCompleter.
type mockPromptProvider struct{}

func (m *mockPromptProvider) ListPrompts(_ context.Context) ([]Prompt, error) {
	return nil, nil
}

func (m *mockPromptProvider) GetPrompt(_ context.Context, _ string, _ map[string]string) (PromptResult, error) {
	return PromptResult{}, ErrPromptNotFound
}

// minimalRegistry implements ToolRegistry but NOT ToolLister.
type minimalRegistry struct{}

func (m *minimalRegistry) Register(_ Tool) error     { return nil }
func (m *minimalRegistry) Get(_ string) (Tool, bool) { return nil, false }
func (m *minimalRegistry) Call(_ context.Context, _ ToolCall) (ToolResult, error) {
	return ToolResult{}, ErrToolNotFound
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
