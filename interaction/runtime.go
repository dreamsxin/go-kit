package interaction

import "context"

// Runtime coordinates sessions, events, tools, and hooks.
type Runtime struct {
	Sessions SessionStore
	Events   EventSink
	Tools    ToolRegistry
	Hooks    []Hook
}

func NewRuntime(sessions SessionStore, events EventSink, tools ToolRegistry, hooks ...Hook) *Runtime {
	if sessions == nil {
		sessions = NewMemorySessionStore()
	}
	if events == nil {
		events = NewMemoryEventSink()
	}
	if tools == nil {
		tools = NewMemoryToolRegistry()
	}
	return &Runtime{
		Sessions: sessions,
		Events:   events,
		Tools:    tools,
		Hooks:    append([]Hook(nil), hooks...),
	}
}

func (r *Runtime) StartSession(ctx context.Context, subject string, metadata map[string]string) (Session, error) {
	session, err := r.Sessions.Create(ctx, subject, metadata)
	if err != nil {
		return Session{}, err
	}
	if err := r.Events.Emit(ctx, Event{SessionID: session.ID, Type: EventSessionStarted}); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (r *Runtime) EndSession(ctx context.Context, id SessionID) (Session, error) {
	session, err := r.Sessions.Close(ctx, id)
	if err != nil {
		return Session{}, err
	}
	if err := r.Events.Emit(ctx, Event{SessionID: id, Type: EventSessionEnded}); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (r *Runtime) RegisterTool(tool Tool) error {
	return r.Tools.Register(tool)
}

func (r *Runtime) ListTools() []ToolDescriptor {
	lister, ok := r.Tools.(ToolLister)
	if !ok {
		return nil
	}
	return lister.List()
}

func (r *Runtime) CallTool(ctx context.Context, call ToolCall) (ToolResult, error) {
	session, err := r.Sessions.Get(ctx, call.SessionID)
	if err != nil {
		return ToolResult{}, err
	}
	if session.Closed() {
		return ToolResult{}, ErrSessionClosed
	}

	for _, hook := range r.Hooks {
		if err := hook.BeforeToolCall(ctx, session, call); err != nil {
			return ToolResult{}, err
		}
	}

	_ = r.Events.Emit(ctx, Event{
		SessionID: call.SessionID,
		Type:      EventToolCall,
		Name:      call.Name,
		Payload:   call.Input,
		Metadata:  call.Metadata,
	})

	result, err := r.Tools.Call(ctx, call)
	_ = r.Events.Emit(ctx, Event{
		SessionID: call.SessionID,
		Type:      EventToolResult,
		Name:      call.Name,
		Payload:   result.Output,
		Metadata:  result.Metadata,
	})
	if err != nil {
		_ = r.Events.Emit(ctx, Event{
			SessionID: call.SessionID,
			Type:      EventError,
			Name:      call.Name,
			Payload:   err.Error(),
		})
	}

	for i := len(r.Hooks) - 1; i >= 0; i-- {
		if hookErr := r.Hooks[i].AfterToolCall(ctx, session, call, result, err); hookErr != nil && err == nil {
			err = hookErr
		}
	}
	return result, err
}

// HookFuncs adapts functions into a Hook.
type HookFuncs struct {
	Before func(context.Context, Session, ToolCall) error
	After  func(context.Context, Session, ToolCall, ToolResult, error) error
}

func (h HookFuncs) BeforeToolCall(ctx context.Context, session Session, call ToolCall) error {
	if h.Before == nil {
		return nil
	}
	return h.Before(ctx, session, call)
}

func (h HookFuncs) AfterToolCall(ctx context.Context, session Session, call ToolCall, result ToolResult, err error) error {
	if h.After == nil {
		return nil
	}
	return h.After(ctx, session, call, result, err)
}
